package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/usecase"
)

func TestCheckTicketStatus_ReturnsAssigneeAndStatus(t *testing.T) {
	tracker := &fakeTracker{statusTicket: domain.Ticket{
		Key:      "PROJ-482",
		URL:      "https://example.atlassian.net/browse/PROJ-482",
		Status:   "In Progress",
		Assignee: domain.Assignee{AccountID: "acct-1", DisplayName: "Ada Lovelace"},
	}}

	ticket, err := usecase.CheckTicketStatus(context.Background(), tracker, "PROJ-482")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.gotStatusKey != "PROJ-482" {
		t.Errorf("tracker got key %q, want %q", tracker.gotStatusKey, "PROJ-482")
	}
	if ticket.Status != "In Progress" {
		t.Errorf("status = %q, want %q", ticket.Status, "In Progress")
	}
	if ticket.Assignee.DisplayName != "Ada Lovelace" {
		t.Errorf("assignee = %q, want %q", ticket.Assignee.DisplayName, "Ada Lovelace")
	}
}

func TestCheckTicketStatus_UnassignedIssue(t *testing.T) {
	tracker := &fakeTracker{statusTicket: domain.Ticket{Key: "PROJ-1", Status: "To Do"}}

	ticket, err := usecase.CheckTicketStatus(context.Background(), tracker, "PROJ-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ticket.Assignee.IsUnassigned() {
		t.Error("ticket should report unassigned")
	}
}

func TestCheckTicketStatus_NotFoundIsWrappedNotPanicked(t *testing.T) {
	tracker := &fakeTracker{statusErr: errors.New("jira: issue PROJ-9 not found")}

	_, err := usecase.CheckTicketStatus(context.Background(), tracker, "PROJ-9")
	if err == nil {
		t.Fatal("want error for missing issue, got nil")
	}
	if !errors.Is(err, usecase.ErrCheckStatusFailed) {
		t.Errorf("error %v should match ErrCheckStatusFailed", err)
	}
	if got := err.Error(); !strings.Contains(got, "not found") {
		t.Errorf("error %q should carry the tracker's own message", got)
	}
}

func TestCheckTicketStatus_RejectsEmptyKeyWithoutCallingTracker(t *testing.T) {
	tracker := &fakeTracker{}

	if _, err := usecase.CheckTicketStatus(context.Background(), tracker, "   "); err == nil {
		t.Fatal("want error for empty key, got nil")
	}
	if tracker.gotStatusKey != "" {
		t.Errorf("tracker should not be called for an empty key, got %q", tracker.gotStatusKey)
	}
}
