package gemini

import (
	"os"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
)

func TestModel_IsSupportedFlashModel(t *testing.T) {
	const want = "gemini-3.6-flash"
	if model != want {
		t.Fatalf("model = %q, want %q", model, want)
	}
}

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
		App:         domain.AppMelaApp,
		IssueType:   domain.IssueTypeBug,
		Priority:    domain.PriorityHigh,
		Labels:      []string{"checkout", "ios", "safari"},
	}

	if got.Title != want.Title || got.Description != want.Description || got.App != want.App || got.IssueType != want.IssueType || got.Priority != want.Priority {
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

func TestParseDraftSet_MultiCandidateFixture_ParsesInOrder(t *testing.T) {
	data, err := os.ReadFile("testdata/draftset_response_multi.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	got, err := parseDraftSet(data)
	if err != nil {
		t.Fatalf("parseDraftSet: %v", err)
	}

	if got.SplitReasoning == "" {
		t.Fatal("expected split reasoning carried through")
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got.Candidates))
	}
	if got.Candidates[0].Draft.Title != "Onfido identity verification screen freezes" {
		t.Fatalf("candidate 0 out of order: %+v", got.Candidates[0])
	}
	if got.Candidates[0].Status != domain.CandidateReady {
		t.Fatalf("expected candidate 0 Ready, got %q", got.Candidates[0].Status)
	}
	if got.Candidates[0].Draft.App != domain.AppMelaApp {
		t.Fatalf("expected candidate 0 app Mela App, got %q", got.Candidates[0].Draft.App)
	}
	if got.Candidates[1].Status != domain.CandidateNeedsClarification {
		t.Fatalf("expected candidate 1 NeedsClarification, got %q", got.Candidates[1].Status)
	}
	if len(got.Candidates[1].OpenQuestions) != 1 {
		t.Fatalf("expected candidate 1 to carry its open question, got %+v", got.Candidates[1].OpenQuestions)
	}
}

func TestParseDraftSet_SingleCandidateFixture_Parses(t *testing.T) {
	data, err := os.ReadFile("testdata/draftset_response_single.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	got, err := parseDraftSet(data)
	if err != nil {
		t.Fatalf("parseDraftSet: %v", err)
	}
	if len(got.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got.Candidates))
	}
	if got.Candidates[0].Draft.App != domain.AppBackend {
		t.Fatalf("expected app Backend, got %q", got.Candidates[0].Draft.App)
	}
}

func TestParseDraftSet_UnknownApp(t *testing.T) {
	_, err := parseDraftSet([]byte(`{"split_reasoning":"r","candidates":[{"status":"Ready","app":"Desktop App","title":"t","description":"d","issue_type":"Bug","priority":"High"}]}`))
	if err == nil {
		t.Fatal("expected error for unknown app, got nil")
	}
}

func TestParseDraftSet_UnknownStatus(t *testing.T) {
	_, err := parseDraftSet([]byte(`{"split_reasoning":"r","candidates":[{"status":"Draft","app":"Mela App","title":"t","description":"d","issue_type":"Bug","priority":"High"}]}`))
	if err == nil {
		t.Fatal("expected error for unknown status, got nil")
	}
}

func TestParseDraftSet_UnknownIssueType(t *testing.T) {
	_, err := parseDraftSet([]byte(`{"split_reasoning":"r","candidates":[{"status":"Ready","app":"Mela App","title":"t","description":"d","issue_type":"Epic","priority":"High"}]}`))
	if err == nil {
		t.Fatal("expected error for unknown issue_type, got nil")
	}
}

func TestParseDraftSet_UnknownPriority(t *testing.T) {
	_, err := parseDraftSet([]byte(`{"split_reasoning":"r","candidates":[{"status":"Ready","app":"Mela App","title":"t","description":"d","issue_type":"Bug","priority":"Critical"}]}`))
	if err == nil {
		t.Fatal("expected error for unknown priority, got nil")
	}
}

func TestParseDraftSet_NeedsClarificationWithoutOpenQuestions_IsParseError(t *testing.T) {
	_, err := parseDraftSet([]byte(`{"split_reasoning":"r","candidates":[{"status":"NeedsClarification","app":"Mela App","title":"t","description":"d","issue_type":"Bug","priority":"High","open_questions":[]}]}`))
	if err == nil {
		t.Fatal("expected error for NeedsClarification candidate with no open_questions, got nil")
	}
}

func TestParseDraftSet_MalformedJSON(t *testing.T) {
	_, err := parseDraftSet([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}
