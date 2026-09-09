package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
	"github.com/actuponit/telegram-jira-integration/internal/usecase"
)

type fakeDrafter struct {
	draft    domain.Draft
	draftSet domain.DraftSet
	err      error
}

func (f fakeDrafter) Draft(ctx context.Context, messages []ports.Message) (domain.DraftSet, error) {
	if f.err != nil {
		return domain.DraftSet{}, f.err
	}
	if len(f.draftSet.Candidates) > 0 {
		return f.draftSet, nil
	}
	return domain.DraftSet{Candidates: []domain.Candidate{{Draft: f.draft, Status: domain.CandidateReady}}}, nil
}

func (f fakeDrafter) DraftFromAnswer(ctx context.Context, summary, answer string) (domain.Draft, error) {
	return f.draft, f.err
}

type fakeTracker struct {
	ticket        domain.Ticket
	err           error
	gotDraft      domain.Draft
	gotAssignee   domain.Assignee
	gotAttachment *ports.Attachment
	createCalled  bool

	// failOnTitle, when set, makes CreateIssue fail only for the Draft
	// with this Title — the rest succeed. Lets a test drive a
	// tracker failure on one of several Candidates.
	failOnTitle string

	// ticketByTitle, when set, returns a distinct Ticket per Draft Title
	// instead of the single ticket field, so a fan-out test can tell
	// which Candidate produced which Issue.
	ticketByTitle map[string]domain.Ticket

	gotDrafts      []domain.Draft
	gotAssignees   []domain.Assignee
	gotAttachments []*ports.Attachment

	statusTicket domain.Ticket
	statusErr    error
	gotStatusKey string
}

func (f *fakeTracker) CreateIssue(ctx context.Context, draft domain.Draft, assignee domain.Assignee, attachment *ports.Attachment) (domain.Ticket, error) {
	f.createCalled = true
	f.gotDraft = draft
	f.gotAssignee = assignee
	f.gotAttachment = attachment
	f.gotDrafts = append(f.gotDrafts, draft)
	f.gotAssignees = append(f.gotAssignees, assignee)
	f.gotAttachments = append(f.gotAttachments, attachment)

	if f.failOnTitle != "" && draft.Title == f.failOnTitle {
		return domain.Ticket{}, f.err
	}
	if f.err != nil && f.failOnTitle == "" {
		return domain.Ticket{}, f.err
	}
	if ticket, ok := f.ticketByTitle[draft.Title]; ok {
		return ticket, nil
	}
	return f.ticket, nil
}

func (f *fakeTracker) GetIssueStatus(ctx context.Context, key string) (domain.Ticket, error) {
	f.gotStatusKey = key
	return f.statusTicket, f.statusErr
}

type fakeResolver struct {
	assignee domain.Assignee
	ok       bool
}

func (f fakeResolver) Resolve(ctx context.Context, telegramHandle string) (domain.Assignee, bool) {
	return f.assignee, f.ok
}

func validMessages() []ports.Message {
	return []ports.Message{
		{SenderName: "Alice", Text: "export crashes on Safari", Timestamp: 100},
	}
}

func readyCandidate(title string) domain.Candidate {
	return domain.Candidate{Draft: domain.Draft{Title: title}, Status: domain.CandidateReady}
}

func TestCreateTicketFromMessage_NoAssignee_CreatesUnassigned(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash", IssueType: domain.IssueTypeBug, Priority: domain.PriorityMedium}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-1", URL: "https://example.atlassian.net/browse/PROJ-1"}}
	resolver := fakeResolver{}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
		TelegramHandle:  "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tracker.gotAssignee.IsUnassigned() {
		t.Fatalf("expected unassigned, got %+v", tracker.gotAssignee)
	}
	if len(result.Created) != 1 {
		t.Fatalf("expected one created ticket, got %+v", result.Created)
	}
	if result.Created[0].AssigneeUnresolved {
		t.Fatalf("expected AssigneeUnresolved false for omitted handle")
	}
	if result.Created[0].Ticket.Key != "PROJ-1" {
		t.Fatalf("expected ticket key PROJ-1, got %q", result.Created[0].Ticket.Key)
	}
}

func TestCreateTicketFromMessage_ResolvedAssignee_CreatesWithAccountID(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash"}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-2"}}
	resolver := fakeResolver{assignee: domain.Assignee{AccountID: "acc-123", DisplayName: "Carol"}, ok: true}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
		TelegramHandle:  "carol",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.gotAssignee.AccountID != "acc-123" {
		t.Fatalf("expected resolved account id, got %+v", tracker.gotAssignee)
	}
	if len(result.Created) != 1 || result.Created[0].AssigneeUnresolved {
		t.Fatalf("expected AssigneeUnresolved false for resolved handle, got %+v", result.Created)
	}
}

func TestCreateTicketFromMessage_UnresolvedAssignee_StillCreatesUnassigned(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash"}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-3"}}
	resolver := fakeResolver{ok: false}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
		TelegramHandle:  "unknownhandle",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tracker.gotAssignee.IsUnassigned() {
		t.Fatalf("expected unassigned issue on unresolved handle, got %+v", tracker.gotAssignee)
	}
	if len(result.Created) != 1 || !result.Created[0].AssigneeUnresolved {
		t.Fatalf("expected AssigneeUnresolved true for unresolved handle, got %+v", result.Created)
	}
}

func TestCreateTicketFromMessage_DrafterError_NoIssueCreated(t *testing.T) {
	drafter := fakeDrafter{err: errors.New("gemini timeout")}
	tracker := &fakeTracker{}
	resolver := fakeResolver{}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
	})
	if err == nil {
		t.Fatal("expected error from drafter failure")
	}
	if !errors.Is(err, usecase.ErrDraftFailed) {
		t.Fatalf("expected ErrDraftFailed, got %v", err)
	}
	if tracker.createCalled {
		t.Fatal("expected no issue creation attempt after drafter failure")
	}
}

func TestCreateTicketFromMessage_TrackerError_RecordedAsFailedCandidate(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash"}}
	tracker := &fakeTracker{err: errors.New("jira: field 'priority' is required")}
	resolver := fakeResolver{}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if len(result.Created) != 0 {
		t.Fatalf("expected no created tickets, got %+v", result.Created)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("expected one failed candidate, got %+v", result.Failed)
	}
	if !errors.Is(result.Failed[0].Err, usecase.ErrCreateIssueFailed) {
		t.Fatalf("expected ErrCreateIssueFailed, got %v", result.Failed[0].Err)
	}
}

func TestCreateTicketFromMessage_NoContextMessages_Errors(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "x"}}
	tracker := &fakeTracker{}
	resolver := fakeResolver{}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{})
	if err == nil {
		t.Fatal("expected error for empty context messages")
	}
	if tracker.createCalled {
		t.Fatal("expected no issue creation attempt with no source message")
	}
}

func TestCreateTicketFromMessage_DescriptionNotesTelegramReporter(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash", Description: "Export button crashes on Safari."}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-4"}}
	resolver := fakeResolver{}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: []ports.Message{
			{SenderName: "Alice", Text: "export crashes on Safari", Timestamp: 100},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	desc := tracker.gotDraft.Description
	if !strings.Contains(desc, "Export button crashes on Safari.") || !strings.Contains(desc, "Alice") {
		t.Fatalf("expected description to retain original text and note reporter, got %q", desc)
	}
}

func TestCreateTicketFromMessage_ImageAttachment_PassedToTracker(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash"}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-5"}}
	resolver := fakeResolver{}
	attachment := &ports.Attachment{Filename: "screenshot.png", Data: []byte("fake-bytes")}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
		ImageAttachment: attachment,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.gotAttachment != attachment {
		t.Fatalf("expected image attachment passed through to tracker, got %+v", tracker.gotAttachment)
	}
}

func TestCreateTicketFromMessage_VideoURL_LinkedInDescriptionNotDownloaded(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash", Description: "Video shows the crash."}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-6"}}
	resolver := fakeResolver{}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
		VideoURL:        "https://api.telegram.org/file/bot123/videos/file_1.mp4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(tracker.gotDraft.Description, "https://api.telegram.org/file/bot123/videos/file_1.mp4") {
		t.Fatalf("expected video URL linked in description, got %q", tracker.gotDraft.Description)
	}
	if tracker.gotAttachment != nil {
		t.Fatalf("expected no attachment for video-only source message, got %+v", tracker.gotAttachment)
	}
}

func TestCreateTicketFromMessage_ThreeReadyCandidates_CreatesThreeIssues(t *testing.T) {
	drafter := fakeDrafter{draftSet: domain.DraftSet{Candidates: []domain.Candidate{
		readyCandidate("A"), readyCandidate("B"), readyCandidate("C"),
	}}}
	tracker := &fakeTracker{ticketByTitle: map[string]domain.Ticket{
		"A": {Key: "PROJ-10"},
		"B": {Key: "PROJ-11"},
		"C": {Key: "PROJ-12"},
	}}
	resolver := fakeResolver{}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Created) != 3 {
		t.Fatalf("expected three created tickets, got %+v", result.Created)
	}
	gotKeys := map[string]bool{}
	for _, c := range result.Created {
		gotKeys[c.Ticket.Key] = true
	}
	for _, want := range []string{"PROJ-10", "PROJ-11", "PROJ-12"} {
		if !gotKeys[want] {
			t.Fatalf("expected created tickets to include %q, got %+v", want, result.Created)
		}
	}
}

func TestCreateTicketFromMessage_MixedReadyAndClarification_CreatesOnlyReady(t *testing.T) {
	drafter := fakeDrafter{draftSet: domain.DraftSet{Candidates: []domain.Candidate{
		readyCandidate("Ready one"),
		{Draft: domain.Draft{Title: "Needs info"}, Status: domain.CandidateNeedsClarification, OpenQuestions: []string{"Which browser?"}},
	}}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-20"}}
	resolver := fakeResolver{}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Created) != 1 || result.Created[0].Ticket.Key != "PROJ-20" {
		t.Fatalf("expected exactly the ready candidate created, got %+v", result.Created)
	}
	if len(result.Clarification) != 1 || result.Clarification[0].Draft.Title != "Needs info" {
		t.Fatalf("expected the unresolved candidate surfaced, got %+v", result.Clarification)
	}
	if len(result.Clarification[0].OpenQuestions) != 1 {
		t.Fatalf("expected open questions carried through, got %+v", result.Clarification[0].OpenQuestions)
	}
}

func TestCreateTicketFromMessage_MultipleClarificationCandidates_AllSurfaced(t *testing.T) {
	drafter := fakeDrafter{draftSet: domain.DraftSet{Candidates: []domain.Candidate{
		{Draft: domain.Draft{Title: "First"}, Status: domain.CandidateNeedsClarification, OpenQuestions: []string{"q1"}},
		{Draft: domain.Draft{Title: "Second"}, Status: domain.CandidateNeedsClarification, OpenQuestions: []string{"q2"}},
	}}}
	tracker := &fakeTracker{}
	resolver := fakeResolver{}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Clarification) != 2 {
		t.Fatalf("expected both unresolved candidates surfaced, got %+v", result.Clarification)
	}
	if tracker.createCalled {
		t.Fatal("expected no issue creation for NeedsClarification candidates")
	}
}

func TestCreateTicketFromMessage_TrackerFailureOnSecondOfThree_CreatesOtherTwo(t *testing.T) {
	drafter := fakeDrafter{draftSet: domain.DraftSet{Candidates: []domain.Candidate{
		readyCandidate("A"), readyCandidate("B"), readyCandidate("C"),
	}}}
	tracker := &fakeTracker{
		failOnTitle: "B",
		err:         errors.New("jira: create issue failed"),
		ticketByTitle: map[string]domain.Ticket{
			"A": {Key: "PROJ-30"},
			"C": {Key: "PROJ-31"},
		},
	}
	resolver := fakeResolver{}

	result, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Created) != 2 {
		t.Fatalf("expected two created tickets, got %+v", result.Created)
	}
	if len(result.Failed) != 1 || result.Failed[0].Draft.Title != "B" {
		t.Fatalf("expected candidate B recorded as failed, got %+v", result.Failed)
	}
}

func TestCreateTicketFromMessage_AssigneeAppliedToEveryIssue(t *testing.T) {
	drafter := fakeDrafter{draftSet: domain.DraftSet{Candidates: []domain.Candidate{
		readyCandidate("A"), readyCandidate("B"),
	}}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-40"}}
	resolver := fakeResolver{assignee: domain.Assignee{AccountID: "acc-999"}, ok: true}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
		TelegramHandle:  "carol",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tracker.gotAssignees) != 2 {
		t.Fatalf("expected two CreateIssue calls, got %d", len(tracker.gotAssignees))
	}
	for _, got := range tracker.gotAssignees {
		if got.AccountID != "acc-999" {
			t.Fatalf("expected assignee applied to every issue, got %+v", got)
		}
	}
}

func TestCreateTicketFromMessage_AttachmentPassedToEveryIssue(t *testing.T) {
	drafter := fakeDrafter{draftSet: domain.DraftSet{Candidates: []domain.Candidate{
		readyCandidate("A"), readyCandidate("B"),
	}}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-41"}}
	resolver := fakeResolver{}
	attachment := &ports.Attachment{Filename: "screenshot.png", Data: []byte("fake-bytes")}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
		ImageAttachment: attachment,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tracker.gotAttachments) != 2 {
		t.Fatalf("expected two CreateIssue calls, got %d", len(tracker.gotAttachments))
	}
	for _, got := range tracker.gotAttachments {
		if got != attachment {
			t.Fatalf("expected attachment passed to every issue, got %+v", got)
		}
	}
}
