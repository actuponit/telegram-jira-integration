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
	draft domain.Draft
	err   error
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
	ticket        domain.Ticket
	err           error
	gotDraft      domain.Draft
	gotAssignee   domain.Assignee
	gotAttachment *ports.Attachment
	createCalled  bool

	statusTicket domain.Ticket
	statusErr    error
	gotStatusKey string
}

func (f *fakeTracker) CreateIssue(ctx context.Context, draft domain.Draft, assignee domain.Assignee, attachment *ports.Attachment) (domain.Ticket, error) {
	f.createCalled = true
	f.gotDraft = draft
	f.gotAssignee = assignee
	f.gotAttachment = attachment
	return f.ticket, f.err
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
	if result.AssigneeUnresolved {
		t.Fatalf("expected AssigneeUnresolved false for omitted handle")
	}
	if result.Ticket.Key != "PROJ-1" {
		t.Fatalf("expected ticket key PROJ-1, got %q", result.Ticket.Key)
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
	if result.AssigneeUnresolved {
		t.Fatalf("expected AssigneeUnresolved false for resolved handle")
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
	if !result.AssigneeUnresolved {
		t.Fatalf("expected AssigneeUnresolved true for unresolved handle")
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
	if tracker.createCalled {
		t.Fatal("expected no issue creation attempt after drafter failure")
	}
}

func TestCreateTicketFromMessage_TrackerError_Propagated(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash"}}
	tracker := &fakeTracker{err: errors.New("jira: field 'priority' is required")}
	resolver := fakeResolver{}

	_, err := usecase.CreateTicketFromMessage(context.Background(), drafter, tracker, resolver, usecase.CreateTicketRequest{
		ContextMessages: validMessages(),
	})
	if err == nil {
		t.Fatal("expected error from tracker failure")
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
