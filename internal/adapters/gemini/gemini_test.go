package gemini

import (
	"os"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
)

func TestParseDraft_RoundTripsRecordedFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/draft_response.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	got, err := parseDraft(data)
	if err != nil {
		t.Fatalf("parseDraft: %v", err)
	}

	want := domain.Draft{
		Title:       "Checkout button unresponsive on iOS Safari",
		Description: "Users report tapping the checkout button on iOS Safari does nothing. No error shown, cart is unaffected.",
		IssueType:   domain.IssueTypeBug,
		Priority:    domain.PriorityHigh,
		Labels:      []string{"checkout", "ios", "safari"},
	}

	if got.Title != want.Title || got.Description != want.Description || got.IssueType != want.IssueType || got.Priority != want.Priority {
		t.Fatalf("parseDraft() = %+v, want %+v", got, want)
	}
	if len(got.Labels) != len(want.Labels) {
		t.Fatalf("labels = %v, want %v", got.Labels, want.Labels)
	}
	for i, l := range want.Labels {
		if got.Labels[i] != l {
			t.Fatalf("labels[%d] = %q, want %q", i, got.Labels[i], l)
		}
	}
}

func TestParseDraft_UnknownIssueType(t *testing.T) {
	_, err := parseDraft([]byte(`{"title":"t","description":"d","issue_type":"Epic","priority":"High"}`))
	if err == nil {
		t.Fatal("expected error for unknown issue_type, got nil")
	}
}

func TestParseDraft_UnknownPriority(t *testing.T) {
	_, err := parseDraft([]byte(`{"title":"t","description":"d","issue_type":"Bug","priority":"Critical"}`))
	if err == nil {
		t.Fatal("expected error for unknown priority, got nil")
	}
}

func TestParseDraft_MalformedJSON(t *testing.T) {
	_, err := parseDraft([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}
