package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
)

func TestNew_UsesBoundedHTTPClient(t *testing.T) {
	tracker := New("https://jira.example", "bot@example.com", "token", "MA")
	if tracker.httpClient.Timeout != requestTimeout {
		t.Fatalf("HTTP client timeout = %s, want %s", tracker.httpClient.Timeout, requestTimeout)
	}
}

func TestValidateCreateMeta_ResolvesSprintFieldFromFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/createmeta.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var meta createMetaResponse
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	fieldID, err := validateCreateMeta(meta, "MA")
	if err != nil {
		t.Fatalf("validateCreateMeta: %v", err)
	}
	if fieldID != "customfield_10020" {
		t.Fatalf("sprintFieldID = %q, want customfield_10020", fieldID)
	}
}

func TestValidateCreateMeta_MissingIssueTypeFails(t *testing.T) {
	meta := createMetaResponse{}
	meta.Projects = []struct {
		Key        string `json:"key"`
		IssueTypes []struct {
			Name   string                         `json:"name"`
			Fields map[string]createMetaFieldMeta `json:"fields"`
		} `json:"issuetypes"`
	}{
		{
			Key: "MA",
			IssueTypes: []struct {
				Name   string                         `json:"name"`
				Fields map[string]createMetaFieldMeta `json:"fields"`
			}{
				{Name: "Bug", Fields: map[string]createMetaFieldMeta{"customfield_10020": {Name: "Sprint"}}},
			},
		},
	}

	if _, err := validateCreateMeta(meta, "MA"); err == nil {
		t.Fatal("expected error for missing Task/Story issue types, got nil")
	}
}

func TestValidateCreateMeta_UnknownProjectFails(t *testing.T) {
	data, err := os.ReadFile("testdata/createmeta.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var meta createMetaResponse
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	if _, err := validateCreateMeta(meta, "NOPE"); err == nil {
		t.Fatal("expected error for unknown project key, got nil")
	}
}

func TestBuildADF_ParagraphsAndHardBreaks(t *testing.T) {
	got := buildADF("first line\nsecond line\n\nnew paragraph")

	want := map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []map[string]any{
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": "first line"},
					{"type": "hardBreak"},
					{"type": "text", "text": "second line"},
				},
			},
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": "new paragraph"},
				},
			},
		},
	}

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("buildADF() = %s, want %s", gotJSON, wantJSON)
	}
}

func TestCreateIssue_OmitsAssigneeKeyWhenUnassigned(t *testing.T) {
	var capturedFields map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		capturedFields = req.Fields
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createIssueResponse{ID: "10001", Key: "MA-1"})
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")

	draft := domain.Draft{
		Title:       "Something broke",
		Description: "It broke.",
		IssueType:   domain.IssueTypeBug,
		Priority:    domain.PriorityMedium,
	}

	ticket, err := tracker.CreateIssue(context.Background(), draft, domain.Assignee{}, nil)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if ticket.Key != "MA-1" {
		t.Fatalf("ticket.Key = %q, want MA-1", ticket.Key)
	}
	if _, present := capturedFields["assignee"]; present {
		t.Fatalf("assignee key present in request for unassigned Issue: %v", capturedFields["assignee"])
	}
}

func TestCreateIssue_SetsAssigneeAccountIDWhenResolved(t *testing.T) {
	var capturedFields map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		capturedFields = req.Fields
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createIssueResponse{ID: "10002", Key: "MA-2"})
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")

	draft := domain.Draft{
		Title:       "Something broke",
		Description: "It broke.",
		IssueType:   domain.IssueTypeBug,
		Priority:    domain.PriorityMedium,
	}
	assignee := domain.Assignee{AccountID: "abc123", DisplayName: "Ada"}

	if _, err := tracker.CreateIssue(context.Background(), draft, assignee, nil); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	got, ok := capturedFields["assignee"].(map[string]any)
	if !ok {
		t.Fatalf("assignee field missing or wrong shape: %v", capturedFields["assignee"])
	}
	if got["accountId"] != "abc123" {
		t.Fatalf("assignee.accountId = %v, want abc123", got["accountId"])
	}
}

func TestCreateIssue_CapturesJiraErrorMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errorResponse{
			ErrorMessages: []string{"issuetype is required"},
		})
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")
	_, err := tracker.CreateIssue(context.Background(), domain.Draft{}, domain.Assignee{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "issuetype is required") {
		t.Fatalf("error %q does not surface Jira's errorMessages", got)
	}
}

func TestCreateIssue_UploadsAttachmentAfterCreation(t *testing.T) {
	var attachmentReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(createIssueResponse{ID: "10003", Key: "MA-3"})
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue/MA-3/attachments":
			if r.Header.Get("X-Atlassian-Token") != "no-check" {
				t.Errorf("missing X-Atlassian-Token: no-check header")
			}
			attachmentReceived = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")
	draft := domain.Draft{Title: "t", Description: "d", IssueType: domain.IssueTypeBug, Priority: domain.PriorityMedium}
	attachment := &ports.Attachment{Filename: "screenshot.png", Data: []byte("fake-png-bytes")}

	if _, err := tracker.CreateIssue(context.Background(), draft, domain.Assignee{}, attachment); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if !attachmentReceived {
		t.Fatal("attachment was never uploaded")
	}
}

func TestGetIssueStatus_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")
	_, err := tracker.GetIssueStatus(context.Background(), "MA-999")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestGetIssueStatus_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")
	_, err := tracker.GetIssueStatus(context.Background(), "MA-1")
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
}

func TestGetIssueStatus_ParsesTicket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"MA-1","fields":{"status":{"name":"In Progress"},"assignee":{"accountId":"abc123","displayName":"Ada"}}}`))
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")
	ticket, err := tracker.GetIssueStatus(context.Background(), "MA-1")
	if err != nil {
		t.Fatalf("GetIssueStatus: %v", err)
	}
	if ticket.Status != "In Progress" {
		t.Fatalf("Status = %q, want In Progress", ticket.Status)
	}
	if ticket.Assignee.AccountID != "abc123" {
		t.Fatalf("Assignee.AccountID = %q, want abc123", ticket.Assignee.AccountID)
	}
}

func TestValidateStartup_ResolvesSprintFieldOverHTTP(t *testing.T) {
	fixture, err := os.ReadFile("testdata/createmeta.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	tracker := New(server.URL, "bot@example.com", "token", "MA")
	if err := tracker.ValidateStartup(context.Background()); err != nil {
		t.Fatalf("ValidateStartup: %v", err)
	}
	if tracker.sprintFieldID != "customfield_10020" {
		t.Fatalf("sprintFieldID = %q, want customfield_10020", tracker.sprintFieldID)
	}
}
