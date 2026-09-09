package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
	"github.com/actuponit/telegram-jira-integration/internal/usecase"
)

// allowedChatID is the chat every handler in these tests is allowlisted
// for; botID is the identity the handler treats as its own.
const (
	allowedChatID int64 = 42
	botID         int64 = 7
)

// --- fakes ---

type fakeSender struct {
	sent       []tgbotapi.MessageConfig
	requested  []tgbotapi.Chattable
	err        error
	requestErr error
	done       chan struct{}
}

func (f *fakeSender) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	f.requested = append(f.requested, c)
	if f.requestErr != nil {
		return nil, f.requestErr
	}
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (f *fakeSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if msg, ok := c.(tgbotapi.MessageConfig); ok {
		f.sent = append(f.sent, msg)
	}
	if f.done != nil {
		close(f.done)
	}
	return tgbotapi.Message{}, f.err
}

type fakeFiles struct {
	urls map[string]string
	data map[string][]byte
	err  error
}

func (f *fakeFiles) FileURL(fileID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.urls[fileID], nil
}

func (f *fakeFiles) Download(_ context.Context, url string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.data[url], nil
}

type fakeDrafter struct {
	draft domain.Draft
	err   error
}

type blockingDrafter struct {
	started chan<- context.Context
	release <-chan struct{}
}

func (d blockingDrafter) Draft(ctx context.Context, _ []ports.Message) (domain.DraftSet, error) {
	d.started <- ctx
	<-d.release
	return domain.DraftSet{Candidates: []domain.Candidate{{Draft: domain.Draft{Title: "Export crashes"}, Status: domain.CandidateReady}}}, nil
}

func (d blockingDrafter) DraftFromAnswer(ctx context.Context, summary, answer string) (domain.Draft, error) {
	return domain.Draft{}, nil
}

func (f fakeDrafter) Draft(ctx context.Context, messages []ports.Message) (domain.DraftSet, error) {
	if f.err != nil {
		return domain.DraftSet{}, f.err
	}
	return domain.DraftSet{Candidates: []domain.Candidate{{Draft: f.draft, Status: domain.CandidateReady}}}, nil
}

func (f fakeDrafter) DraftFromAnswer(ctx context.Context, summary, answer string) (domain.Draft, error) {
	return f.draft, f.err
}

type fakeTracker struct {
	ticket domain.Ticket
	err    error

	statusTicket domain.Ticket
	statusErr    error
	gotStatusKey *string
}

func (f fakeTracker) CreateIssue(ctx context.Context, draft domain.Draft, assignee domain.Assignee, attachment *ports.Attachment) (domain.Ticket, error) {
	return f.ticket, f.err
}

func (f fakeTracker) GetIssueStatus(ctx context.Context, key string) (domain.Ticket, error) {
	if f.gotStatusKey != nil {
		*f.gotStatusKey = key
	}
	return f.statusTicket, f.statusErr
}

type fakeResolver struct {
	assignee domain.Assignee
	ok       bool
}

func (f fakeResolver) Resolve(ctx context.Context, handle string) (domain.Assignee, bool) {
	return f.assignee, f.ok
}

// --- parseToTicketCommand ---

func TestParseToTicketCommand(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		wantAssignee string
		wantOK       bool
	}{
		{"plain command", "/to_ticket", "", true},
		{"command with bot suffix", "/to_ticket@MyBot", "", true},
		{"command with assignee", "/to_ticket @alice", "@alice", true},
		{"command with bot suffix and assignee", "/to_ticket@MyBot @alice", "@alice", true},
		{"not a command", "hello there", "", false},
		{"empty text", "", "", false},
		{"different command", "/status", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assignee, ok := parseToTicketCommand(tc.text)
			if ok != tc.wantOK || assignee != tc.wantAssignee {
				t.Fatalf("parseToTicketCommand(%q) = (%q, %v), want (%q, %v)", tc.text, assignee, ok, tc.wantAssignee, tc.wantOK)
			}
		})
	}
}

// --- gatherContext ---

func TestGatherContext_SingleMessage(t *testing.T) {
	source := &tgbotapi.Message{
		MessageID: 1,
		From:      &tgbotapi.User{UserName: "alice"},
		Text:      "it's broken",
		Date:      100,
	}

	got := gatherContext(source)

	want := []ports.Message{{SenderName: "alice", Text: "it's broken", Timestamp: 100}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("gatherContext() = %+v, want %+v", got, want)
	}
}

func TestGatherContext_ReplyChainOrderedOldestFirst(t *testing.T) {
	grandparent := &tgbotapi.Message{From: &tgbotapi.User{UserName: "carol"}, Text: "first report", Date: 100}
	parent := &tgbotapi.Message{From: &tgbotapi.User{UserName: "bob"}, Text: "still happening", Date: 200, ReplyToMessage: grandparent}
	source := &tgbotapi.Message{From: &tgbotapi.User{UserName: "alice"}, Text: "it's broken", Date: 300, ReplyToMessage: parent}

	got := gatherContext(source)

	if len(got) != 3 {
		t.Fatalf("len(gatherContext()) = %d, want 3", len(got))
	}
	if got[0].SenderName != "carol" || got[1].SenderName != "bob" || got[2].SenderName != "alice" {
		t.Fatalf("gatherContext() not ordered oldest-first: %+v", got)
	}
}

func TestGatherContext_FallsBackToFirstNameAndCaption(t *testing.T) {
	source := &tgbotapi.Message{
		From:    &tgbotapi.User{FirstName: "Dave"},
		Caption: "look at this",
	}

	got := gatherContext(source)

	if got[0].SenderName != "Dave" {
		t.Fatalf("SenderName = %q, want Dave", got[0].SenderName)
	}
	if got[0].Text != "look at this" {
		t.Fatalf("Text = %q, want the caption", got[0].Text)
	}
}

// --- extractImage / extractVideoURL ---

func TestExtractImage_NoPhoto(t *testing.T) {
	attachment, err := extractImage(context.Background(), &tgbotapi.Message{}, &fakeFiles{})
	if err != nil {
		t.Fatalf("extractImage: %v", err)
	}
	if attachment != nil {
		t.Fatalf("expected nil attachment, got %+v", attachment)
	}
}

func TestExtractImage_DownloadsLargestPhoto(t *testing.T) {
	source := &tgbotapi.Message{
		Photo: []tgbotapi.PhotoSize{
			{FileID: "small"},
			{FileID: "large"},
		},
	}
	files := &fakeFiles{
		urls: map[string]string{"large": "https://telegram.example/file/large.jpg"},
		data: map[string][]byte{"https://telegram.example/file/large.jpg": []byte("fake-bytes")},
	}

	attachment, err := extractImage(context.Background(), source, files)
	if err != nil {
		t.Fatalf("extractImage: %v", err)
	}
	if attachment == nil {
		t.Fatal("expected an attachment, got nil")
	}
	if attachment.Filename != "large.jpg" {
		t.Fatalf("Filename = %q, want large.jpg", attachment.Filename)
	}
	if string(attachment.Data) != "fake-bytes" {
		t.Fatalf("Data = %q, want fake-bytes", attachment.Data)
	}
}

func TestExtractImage_DownloadErrorSurfaced(t *testing.T) {
	source := &tgbotapi.Message{Photo: []tgbotapi.PhotoSize{{FileID: "large"}}}
	files := &fakeFiles{err: errors.New("boom")}

	if _, err := extractImage(context.Background(), source, files); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestExtractVideoURL_NoVideo(t *testing.T) {
	url, err := extractVideoURL(context.Background(), &tgbotapi.Message{}, &fakeFiles{})
	if err != nil {
		t.Fatalf("extractVideoURL: %v", err)
	}
	if url != "" {
		t.Fatalf("url = %q, want empty", url)
	}
}

func TestExtractVideoURL_NeverDownloadsBytes(t *testing.T) {
	source := &tgbotapi.Message{Video: &tgbotapi.Video{FileID: "vid"}}
	files := &fakeFiles{urls: map[string]string{"vid": "https://telegram.example/file/vid.mp4"}}

	url, err := extractVideoURL(context.Background(), source, files)
	if err != nil {
		t.Fatalf("extractVideoURL: %v", err)
	}
	if url != "https://telegram.example/file/vid.mp4" {
		t.Fatalf("url = %q, want the video file URL", url)
	}
	if files.data != nil {
		t.Fatal("video bytes should never be downloaded")
	}
}

// --- ParseTicketKey ---

func TestParseTicketKey(t *testing.T) {
	cases := []struct {
		text    string
		wantKey string
		wantOK  bool
	}{
		{"Created PROJ-482: Export crashes — https://jira.example/browse/PROJ-482", "PROJ-482", true},
		{"Created MA-1: Bug — url", "MA-1", true},
		{"not a confirmation message", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		key, ok := ParseTicketKey(tc.text)
		if key != tc.wantKey || ok != tc.wantOK {
			t.Errorf("ParseTicketKey(%q) = (%q, %v), want (%q, %v)", tc.text, key, ok, tc.wantKey, tc.wantOK)
		}
	}
}

// --- classifyCreateTicketError ---

func TestClassifyCreateTicketError(t *testing.T) {
	geminiErr := fmt.Errorf("%w: %w", usecase.ErrDraftFailed, errors.New("timeout"))
	if stage, _ := classifyCreateTicketError(geminiErr); stage != "gemini" {
		t.Fatalf("stage = %q, want gemini", stage)
	}

	jiraErr := fmt.Errorf("%w: %w", usecase.ErrCreateIssueFailed, errors.New("issuetype is required"))
	if stage, msg := classifyCreateTicketError(jiraErr); stage != "jira" || !bytes.Contains([]byte(msg), []byte("issuetype is required")) {
		t.Fatalf("stage/msg = %q/%q, want jira with Jira's error message surfaced", stage, msg)
	}
}

// --- ServeHTTP ---

func newUpdateBody(t *testing.T, update tgbotapi.Update) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	return bytes.NewReader(data)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
}

func TestServeHTTP_RejectsMissingOrWrongSecretToken(t *testing.T) {
	send := &fakeSender{}
	h := newHandler(send, &fakeFiles{}, "correct-secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, fakeTracker{}, fakeResolver{}, testLogger())

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("{}")))
	req.Header.Set(secretHeaderName, "wrong-secret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(send.sent) != 0 {
		t.Fatal("no reply should be sent for an unverified request")
	}
}

func TestServeHTTP_AcknowledgesBeforeTicketWorkWithDetachedContext(t *testing.T) {
	started := make(chan context.Context)
	release := make(chan struct{})
	send := &fakeSender{done: make(chan struct{})}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), blockingDrafter{started: started, release: release}, fakeTracker{ticket: domain.Ticket{Key: "MA-1"}}, fakeResolver{}, testLogger())

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: allowedChatID}, Text: "/to_ticket",
		ReplyToMessage: &tgbotapi.Message{From: &tgbotapi.User{UserName: "alice"}, Text: "it broke"},
	}}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/webhook", newUpdateBody(t, update)).WithContext(requestContext)
	req.Header.Set(secretHeaderName, "secret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	cancelRequest()

	select {
	case processingContext := <-started:
		if err := processingContext.Err(); err != nil {
			t.Fatalf("ticket context inherited cancellation from webhook request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ticket work did not start")
	}

	close(release)
	select {
	case <-send.done:
	case <-time.After(time.Second):
		t.Fatal("ticket work did not finish")
	}
}

func TestServeHTTP_NoReplyTargetSendsUsageHint(t *testing.T) {
	send := &fakeSender{}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, fakeTracker{}, fakeResolver{}, testLogger())

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: allowedChatID},
		Text:      "/to_ticket",
	}}
	h.processTicket(update.Message)
	if len(send.sent) != 1 || send.sent[0].Text != usageHint {
		t.Fatalf("sent = %+v, want a single usage-hint reply", send.sent)
	}
}

func TestServeHTTP_SuccessRepliesWithCreatedMessage(t *testing.T) {
	send := &fakeSender{}
	tracker := fakeTracker{ticket: domain.Ticket{Key: "MA-1", URL: "https://jira.example/browse/MA-1"}}
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crashes"}}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), drafter, tracker, fakeResolver{}, testLogger())

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 2,
		Chat:      &tgbotapi.Chat{ID: allowedChatID},
		Text:      "/to_ticket",
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: "alice"},
			Text: "Safari crashes on export",
			Date: 100,
		},
	}}
	h.processTicket(update.Message)

	if len(send.sent) != 1 {
		t.Fatalf("sent = %+v, want a single reply", send.sent)
	}
	want := "Created MA-1: Export crashes — https://jira.example/browse/MA-1"
	if send.sent[0].Text != want {
		t.Fatalf("reply = %q, want %q", send.sent[0].Text, want)
	}
}

func TestServeHTTP_UnresolvedAssigneeNotedInReply(t *testing.T) {
	send := &fakeSender{}
	tracker := fakeTracker{ticket: domain.Ticket{Key: "MA-2", URL: "https://jira.example/browse/MA-2"}}
	resolver := fakeResolver{ok: false}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, tracker, resolver, testLogger())

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: allowedChatID},
		Text: "/to_ticket @nobody",
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: "alice"},
			Text: "it broke",
		},
	}}
	h.processTicket(update.Message)

	if len(send.sent) != 1 {
		t.Fatalf("sent = %+v, want a single reply", send.sent)
	}
	if !bytes.Contains([]byte(send.sent[0].Text), []byte("nobody")) {
		t.Fatalf("reply = %q, want it to mention the unresolved assignee", send.sent[0].Text)
	}
}

func TestServeHTTP_GeminiFailureRepliesWithClearError(t *testing.T) {
	send := &fakeSender{}
	drafter := fakeDrafter{err: errors.New("timeout")}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), drafter, fakeTracker{}, fakeResolver{}, testLogger())

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		Chat:           &tgbotapi.Chat{ID: allowedChatID},
		Text:           "/to_ticket",
		ReplyToMessage: &tgbotapi.Message{From: &tgbotapi.User{UserName: "alice"}, Text: "it broke"},
	}}
	h.processTicket(update.Message)

	if len(send.sent) != 1 || !bytes.Contains([]byte(send.sent[0].Text), []byte("Gemini")) {
		t.Fatalf("sent = %+v, want a reply distinguishing the Gemini failure", send.sent)
	}
}

func TestServeHTTP_JiraFailureSurfacesErrorMessages(t *testing.T) {
	send := &fakeSender{}
	tracker := fakeTracker{err: errors.New("jira: create issue failed (400): issuetype is required")}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, tracker, fakeResolver{}, testLogger())

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		Chat:           &tgbotapi.Chat{ID: allowedChatID},
		Text:           "/to_ticket",
		ReplyToMessage: &tgbotapi.Message{From: &tgbotapi.User{UserName: "alice"}, Text: "it broke"},
	}}
	h.processTicket(update.Message)

	if len(send.sent) != 1 || !bytes.Contains([]byte(send.sent[0].Text), []byte("issuetype is required")) {
		t.Fatalf("sent = %+v, want Jira's errorMessages surfaced", send.sent)
	}
}

func TestServeHTTP_IgnoresNonTicketCommands(t *testing.T) {
	send := &fakeSender{}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, fakeTracker{}, fakeResolver{}, testLogger())

	update := tgbotapi.Update{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: allowedChatID}, Text: "just chatting"}}
	req := httptest.NewRequest(http.MethodPost, "/webhook", newUpdateBody(t, update))
	req.Header.Set(secretHeaderName, "secret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(send.sent) != 0 {
		t.Fatalf("sent = %+v, want no reply for a non-command message", send.sent)
	}
}

// --- chat allowlist ---

func newAllowlistedHandler(send *fakeSender, tracker fakeTracker) *Handler {
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, tracker, fakeResolver{}, testLogger())
	h.botID = botID
	return h
}

func serveUpdate(t *testing.T, h *Handler, update tgbotapi.Update) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook", newUpdateBody(t, update))
	req.Header.Set(secretHeaderName, "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServeHTTP_CommandsFromNonAllowlistedChatHaveNoEffect(t *testing.T) {
	for _, text := range []string{"/to_ticket", "/to_ticket @alice", "/status"} {
		send := &fakeSender{}
		tracker := fakeTracker{ticket: domain.Ticket{Key: "MA-1"}}
		h := newAllowlistedHandler(send, tracker)
		dispatched := false
		h.dispatch = func(func()) { dispatched = true }

		update := tgbotapi.Update{Message: &tgbotapi.Message{
			MessageID:      9,
			Chat:           &tgbotapi.Chat{ID: allowedChatID + 1},
			Text:           text,
			ReplyToMessage: &tgbotapi.Message{From: &tgbotapi.User{ID: botID}, Text: "Created MA-1: x — u"},
		}}
		rec := serveUpdate(t, h, update)

		if rec.Code != http.StatusOK {
			t.Fatalf("%q: status = %d, want 200", text, rec.Code)
		}
		if dispatched {
			t.Fatalf("%q from a non-allowlisted chat was dispatched, want no processing", text)
		}
	}
}

func TestServeHTTP_IgnoredChatIsStillLoggedWithChatID(t *testing.T) {
	var logs bytes.Buffer
	h := newHandler(&fakeSender{}, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID),
		fakeDrafter{}, fakeTracker{}, fakeResolver{}, slog.New(slog.NewTextHandler(&logs, nil)))

	serveUpdate(t, h, tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 9,
		Chat:      &tgbotapi.Chat{ID: 999},
		Text:      "/to_ticket",
	}})

	logged := logs.String()
	if !strings.Contains(logged, "chat_id=999") {
		t.Fatalf("logs = %q, want the ignored chat ID recorded", logged)
	}
}

func TestServeHTTP_AllowlistedChatIsProcessed(t *testing.T) {
	send := &fakeSender{}
	h := newAllowlistedHandler(send, fakeTracker{ticket: domain.Ticket{Key: "MA-1"}})

	h.processTicket(&tgbotapi.Message{
		MessageID:      1,
		Chat:           &tgbotapi.Chat{ID: allowedChatID},
		Text:           "/to_ticket",
		ReplyToMessage: &tgbotapi.Message{From: &tgbotapi.User{UserName: "alice"}, Text: "it broke"},
	})

	if len(send.sent) != 1 {
		t.Fatalf("sent = %+v, want the allowlisted chat's command answered", send.sent)
	}
}

// --- /status ---

func statusCommand(reply *tgbotapi.Message) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID:      5,
		Chat:           &tgbotapi.Chat{ID: allowedChatID},
		Text:           "/status",
		ReplyToMessage: reply,
	}
}

func botConfirmation(text string) *tgbotapi.Message {
	return &tgbotapi.Message{From: &tgbotapi.User{ID: botID}, Text: text}
}

func TestProcessStatus_RepliesWithAssigneeAndStatus(t *testing.T) {
	send := &fakeSender{}
	var gotKey string
	h := newAllowlistedHandler(send, fakeTracker{
		gotStatusKey: &gotKey,
		statusTicket: domain.Ticket{
			Key:      "MA-1",
			URL:      "https://jira.example/browse/MA-1",
			Status:   "In Progress",
			Assignee: domain.Assignee{AccountID: "acct-1", DisplayName: "Ada Lovelace"},
		},
	})

	h.processStatus(statusCommand(botConfirmation("Created MA-1: Export crashes — https://jira.example/browse/MA-1")))

	if gotKey != "MA-1" {
		t.Errorf("looked up key %q, want MA-1", gotKey)
	}
	if len(send.sent) != 1 {
		t.Fatalf("sent = %+v, want a single reply", send.sent)
	}
	reply := send.sent[0].Text
	if !strings.Contains(reply, "In Progress") || !strings.Contains(reply, "Ada Lovelace") {
		t.Fatalf("reply = %q, want the workflow status and assignee display name", reply)
	}
}

func TestProcessStatus_UnassignedIssueRendersUnassigned(t *testing.T) {
	send := &fakeSender{}
	h := newAllowlistedHandler(send, fakeTracker{statusTicket: domain.Ticket{Key: "MA-1", Status: "To Do"}})

	h.processStatus(statusCommand(botConfirmation("Created MA-1: Export crashes — u")))

	if len(send.sent) != 1 || !strings.Contains(send.sent[0].Text, "Unassigned") {
		t.Fatalf("sent = %+v, want the reply to say Unassigned", send.sent)
	}
}

func TestProcessStatus_RejectsReplyToNonConfirmationMessage(t *testing.T) {
	cases := map[string]*tgbotapi.Message{
		"no reply target":              nil,
		"bot message without a key":    botConfirmation("here you go"),
		"user impersonating the reply": {From: &tgbotapi.User{ID: botID + 1}, Text: "Created MA-1: fake — u"},
	}

	for name, reply := range cases {
		send := &fakeSender{}
		var gotKey string
		h := newAllowlistedHandler(send, fakeTracker{gotStatusKey: &gotKey})

		h.processStatus(statusCommand(reply))

		if gotKey != "" {
			t.Errorf("%s: tracker was called with %q, want no lookup", name, gotKey)
		}
		if len(send.sent) != 1 || send.sent[0].Text != statusUsageHint {
			t.Errorf("%s: sent = %+v, want a clear usage hint rather than silence", name, send.sent)
		}
	}
}

func TestProcessStatus_InaccessibleIssueRepliesWithClearError(t *testing.T) {
	send := &fakeSender{}
	h := newAllowlistedHandler(send, fakeTracker{statusErr: errors.New("jira: issue MA-1 not found")})

	h.processStatus(statusCommand(botConfirmation("Created MA-1: gone — u")))

	if len(send.sent) != 1 || !strings.Contains(send.sent[0].Text, "not found") {
		t.Fatalf("sent = %+v, want Jira's own message surfaced", send.sent)
	}
}

func TestRenderStatus_AssigneeLabelFallsBackToAccountID(t *testing.T) {
	if got := assigneeLabel(domain.Assignee{}); got != "Unassigned" {
		t.Errorf("assigneeLabel(zero) = %q, want Unassigned", got)
	}
	if got := assigneeLabel(domain.Assignee{AccountID: "acct-1"}); got != "acct-1" {
		t.Errorf("assigneeLabel(no display name) = %q, want the account ID", got)
	}
}

func TestCommandName(t *testing.T) {
	cases := map[string]string{
		"/to_ticket":            commandToTicket,
		"/to_ticket@MyBot @bob": commandToTicket,
		"/STATUS":               commandStatus,
		"/status@MyBot":         commandStatus,
		"just chatting":         "",
		"":                      "",
	}
	for text, want := range cases {
		if got := commandName(text); got != want {
			t.Errorf("commandName(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestRegisterCommandsPublishesEveryRoutedCommand(t *testing.T) {
	send := &fakeSender{}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, fakeTracker{}, fakeResolver{}, testLogger())

	if err := h.RegisterCommands(); err != nil {
		t.Fatalf("RegisterCommands() = %v, want nil", err)
	}

	if len(send.requested) != 1 {
		t.Fatalf("requests = %d, want 1", len(send.requested))
	}
	config, ok := send.requested[0].(tgbotapi.SetMyCommandsConfig)
	if !ok {
		t.Fatalf("request = %T, want tgbotapi.SetMyCommandsConfig", send.requested[0])
	}

	published := map[string]string{}
	for _, command := range config.Commands {
		published[command.Command] = command.Description
	}
	for _, routed := range []string{commandToTicket, commandStatus} {
		name := strings.TrimPrefix(routed, "/")
		description, found := published[name]
		if !found {
			t.Fatalf("command %q is routed but not published to Telegram", name)
		}
		// Telegram rejects descriptions shorter than 3 characters.
		if len(description) < 3 {
			t.Fatalf("description for %q = %q, too short for Telegram", name, description)
		}
	}
	if len(published) != 2 {
		t.Fatalf("published %d commands, want only the routed ones", len(published))
	}
}

func TestRegisterCommandsReturnsTelegramError(t *testing.T) {
	send := &fakeSender{requestErr: errors.New("telegram down")}
	h := newHandler(send, &fakeFiles{}, "secret", domain.NewChatAllowlist(allowedChatID), fakeDrafter{}, fakeTracker{}, fakeResolver{}, testLogger())

	if err := h.RegisterCommands(); err == nil {
		t.Fatal("RegisterCommands() = nil, want an error when Telegram rejects the call")
	}
}
