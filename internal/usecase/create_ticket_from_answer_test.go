package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
	"github.com/actuponit/telegram-jira-integration/internal/usecase"
)

func TestCreateTicketFromAnswer_CreatesOneUnassignedIssueWithoutAttachment(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash", Description: "Crashes on Safari.", IssueType: domain.IssueTypeBug, Priority: domain.PriorityMedium}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-50"}}

	result, err := usecase.CreateTicketFromAnswer(context.Background(), drafter, tracker, "Bot summary: export crashes", "It's on iOS Safari")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Ticket.Key != "PROJ-50" {
		t.Fatalf("expected ticket key PROJ-50, got %+v", result.Ticket)
	}
	if result.Title != "Export crash" {
		t.Fatalf("expected title carried through, got %q", result.Title)
	}
	if !tracker.gotAssignee.IsUnassigned() {
		t.Fatalf("expected unassigned issue, got %+v", tracker.gotAssignee)
	}
	if tracker.gotAttachment != nil {
		t.Fatalf("expected no attachment, got %+v", tracker.gotAttachment)
	}
}

func TestCreateTicketFromAnswer_DrafterError_NoIssueCreated(t *testing.T) {
	drafter := fakeDrafter{err: errors.New("gemini timeout")}
	tracker := &fakeTracker{}

	_, err := usecase.CreateTicketFromAnswer(context.Background(), drafter, tracker, "summary", "answer")
	if !errors.Is(err, usecase.ErrDraftFailed) {
		t.Fatalf("expected ErrDraftFailed, got %v", err)
	}
	if tracker.createCalled {
		t.Fatal("expected no issue creation attempt after drafter failure")
	}
}

func TestCreateTicketFromAnswer_TrackerError_DistinguishableFromDrafterError(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "Export crash"}}
	tracker := &fakeTracker{err: errors.New("jira: field 'priority' is required")}

	_, err := usecase.CreateTicketFromAnswer(context.Background(), drafter, tracker, "summary", "answer")
	if !errors.Is(err, usecase.ErrCreateIssueFailed) {
		t.Fatalf("expected ErrCreateIssueFailed, got %v", err)
	}
	if errors.Is(err, usecase.ErrDraftFailed) {
		t.Fatalf("tracker error should not also match ErrDraftFailed: %v", err)
	}
}

func TestCreateTicketFromAnswer_PassesSummaryAndAnswerToDrafter(t *testing.T) {
	drafter := fakeDrafter{draft: domain.Draft{Title: "x"}}
	tracker := &fakeTracker{ticket: domain.Ticket{Key: "PROJ-51"}}

	var gotSummary, gotAnswer string
	spy := spyDrafter{fakeDrafter: drafter, onDraftFromAnswer: func(summary, answer string) {
		gotSummary, gotAnswer = summary, answer
	}}

	_, err := usecase.CreateTicketFromAnswer(context.Background(), spy, tracker, "Bot summary text", "the reporter's reply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSummary != "Bot summary text" || gotAnswer != "the reporter's reply" {
		t.Fatalf("expected summary/answer passed through, got summary=%q answer=%q", gotSummary, gotAnswer)
	}
}

// spyDrafter wraps fakeDrafter to observe DraftFromAnswer's arguments
// without adding call-count assertions to fakeTracker/fakeDrafter itself.
type spyDrafter struct {
	fakeDrafter
	onDraftFromAnswer func(summary, answer string)
}

func (s spyDrafter) DraftFromAnswer(ctx context.Context, summary, answer string) (domain.Draft, error) {
	s.onDraftFromAnswer(summary, answer)
	return s.fakeDrafter.DraftFromAnswer(ctx, summary, answer)
}

var _ ports.TicketDrafter = spyDrafter{}
