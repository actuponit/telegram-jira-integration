// Package jira implements ports.IssueTracker against the Jira REST API v3.
package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
)

// sprintName is the fixed Sprint every created Issue is placed on.
const sprintName = "MA Sprint Board 4"

const requestTimeout = 30 * time.Second

var _ ports.IssueTracker = (*Tracker)(nil)

// Tracker implements ports.IssueTracker against the Jira REST API v3.
type Tracker struct {
	httpClient *http.Client
	baseURL    string
	email      string
	apiToken   string
	projectKey string

	// sprintFieldID is the custom field id for "Sprint", resolved by
	// ValidateStartup before any Issue is created.
	sprintFieldID string

	// sprintID is the numeric id of the fixed Sprint every created Issue is
	// placed on, resolved by ValidateStartup via the Agile API.
	sprintID int
}

// New creates a Tracker against the given Jira site.
func New(baseURL, email, apiToken, projectKey string) *Tracker {
	return &Tracker{
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		email:      email,
		apiToken:   apiToken,
		projectKey: projectKey,
	}
}

func (t *Tracker) authHeader() string {
	token := base64.StdEncoding.EncodeToString([]byte(t.email + ":" + t.apiToken))
	return "Basic " + token
}

// --- create issue ---

type createIssueRequest struct {
	Fields map[string]any `json:"fields"`
}

type createIssueResponse struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

type errorResponse struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

func (e errorResponse) String() string {
	parts := append([]string{}, e.ErrorMessages...)
	for field, msg := range e.Errors {
		parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
	}
	return strings.Join(parts, "; ")
}

// CreateIssue creates a Jira Issue from draft, assigning it to assignee
// when resolved, uploading attachment when present, and always placing it
// on the fixed Sprint.
func (t *Tracker) CreateIssue(ctx context.Context, draft domain.Draft, assignee domain.Assignee, attachment *ports.Attachment) (domain.Ticket, error) {
	fields := map[string]any{
		"project":     map[string]any{"key": t.projectKey},
		"summary":     draft.Title,
		"description": buildADF(draft.Description),
		"issuetype":   map[string]any{"name": string(draft.IssueType)},
		"priority":    map[string]any{"name": string(draft.Priority)},
	}
	if len(draft.Labels) > 0 {
		fields["labels"] = draft.Labels
	}
	if !assignee.IsUnassigned() {
		fields["assignee"] = map[string]any{"accountId": assignee.AccountID}
	}
	if t.sprintFieldID != "" && t.sprintID != 0 {
		fields[t.sprintFieldID] = t.sprintID
	}

	body, err := json.Marshal(createIssueRequest{Fields: fields})
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: encode create issue request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/rest/api/3/issue", bytes.NewReader(body))
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: build create issue request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", t.authHeader())

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: create issue: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: read create issue response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return domain.Ticket{}, fmt.Errorf("jira: create issue failed (%d): %s", resp.StatusCode, decodeJiraError(respBody))
	}

	var created createIssueResponse
	if err := json.Unmarshal(respBody, &created); err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: parse create issue response: %w", err)
	}

	ticket := domain.Ticket{
		Key:      created.Key,
		URL:      fmt.Sprintf("%s/browse/%s", t.baseURL, created.Key),
		Assignee: assignee,
		Status:   "",
	}

	if attachment != nil {
		if err := t.uploadAttachment(ctx, created.Key, attachment); err != nil {
			return ticket, fmt.Errorf("jira: upload attachment to %s: %w", created.Key, err)
		}
	}

	return ticket, nil
}

func (t *Tracker) uploadAttachment(ctx context.Context, issueKey string, attachment *ports.Attachment) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", attachment.Filename)
	if err != nil {
		return fmt.Errorf("build multipart body: %w", err)
	}
	if _, err := part.Write(attachment.Data); err != nil {
		return fmt.Errorf("write attachment data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s/attachments", t.baseURL, issueKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("build attachment request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", t.authHeader())
	req.Header.Set("X-Atlassian-Token", "no-check")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("attachment upload failed (%d): %s", resp.StatusCode, decodeJiraError(respBody))
	}
	return nil
}

func decodeJiraError(body []byte) string {
	var e errorResponse
	if err := json.Unmarshal(body, &e); err != nil || (len(e.ErrorMessages) == 0 && len(e.Errors) == 0) {
		return string(body)
	}
	return e.String()
}

// --- get issue status ---

type getIssueResponse struct {
	Key    string `json:"key"`
	Fields struct {
		Status struct {
			Name string `json:"name"`
		} `json:"status"`
		Assignee *struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		} `json:"assignee"`
	} `json:"fields"`
}

// GetIssueStatus fetches the current status of the Issue identified by key.
func (t *Tracker) GetIssueStatus(ctx context.Context, key string) (domain.Ticket, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", t.baseURL, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: build get issue request: %w", err)
	}
	req.Header.Set("Authorization", t.authHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: get issue %s: %w", key, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: read get issue response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return domain.Ticket{}, fmt.Errorf("jira: issue %s not found", key)
	case http.StatusForbidden:
		return domain.Ticket{}, fmt.Errorf("jira: not permitted to view issue %s", key)
	default:
		return domain.Ticket{}, fmt.Errorf("jira: get issue %s failed (%d): %s", key, resp.StatusCode, decodeJiraError(respBody))
	}

	var parsed getIssueResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return domain.Ticket{}, fmt.Errorf("jira: parse get issue response: %w", err)
	}

	ticket := domain.Ticket{
		Key:    parsed.Key,
		URL:    fmt.Sprintf("%s/browse/%s", t.baseURL, parsed.Key),
		Status: parsed.Fields.Status.Name,
	}
	if parsed.Fields.Assignee != nil {
		ticket.Assignee = domain.Assignee{
			AccountID:   parsed.Fields.Assignee.AccountID,
			DisplayName: parsed.Fields.Assignee.DisplayName,
		}
	}

	return ticket, nil
}

// --- startup validation ---

type createMetaFieldMeta struct {
	Name          string `json:"name"`
	AllowedValues []struct {
		Name string `json:"name"`
	} `json:"allowedValues"`
}

type createMetaResponse struct {
	Projects []struct {
		Key        string `json:"key"`
		IssueTypes []struct {
			Name   string                         `json:"name"`
			Fields map[string]createMetaFieldMeta `json:"fields"`
		} `json:"issuetypes"`
	} `json:"projects"`
}

// ValidateStartup checks that this Jira project's issue types and
// priorities match what this bot needs, and resolves the custom field id
// for "Sprint". It returns an error that should fail the bot's startup
// fast on bad config.
func (t *Tracker) ValidateStartup(ctx context.Context) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/createmeta?projectKeys=%s&expand=projects.issuetypes.fields", t.baseURL, t.projectKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("jira: build createmeta request: %w", err)
	}
	req.Header.Set("Authorization", t.authHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jira: fetch createmeta: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("jira: read createmeta response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jira: createmeta failed (%d): %s", resp.StatusCode, decodeJiraError(respBody))
	}

	var meta createMetaResponse
	if err := json.Unmarshal(respBody, &meta); err != nil {
		return fmt.Errorf("jira: parse createmeta response: %w", err)
	}

	sprintFieldID, err := validateCreateMeta(meta, t.projectKey)
	if err != nil {
		return err
	}
	t.sprintFieldID = sprintFieldID

	sprintID, err := t.resolveSprintID(ctx)
	if err != nil {
		return err
	}
	t.sprintID = sprintID
	return nil
}

type agileBoardResponse struct {
	IsLast bool `json:"isLast"`
	Values []struct {
		ID int `json:"id"`
	} `json:"values"`
}

type agileSprintResponse struct {
	IsLast bool `json:"isLast"`
	Values []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"values"`
}

// resolveSprintID finds the numeric id of sprintName across every board for
// this Tracker's project, by walking the Jira Agile API. The Sprint custom
// field only accepts a sprint id on issue creation, not its name.
func (t *Tracker) resolveSprintID(ctx context.Context) (int, error) {
	startAt := 0
	for {
		url := fmt.Sprintf("%s/rest/agile/1.0/board?projectKeyOrId=%s&startAt=%d", t.baseURL, t.projectKey, startAt)
		var boards agileBoardResponse
		if err := t.getAgile(ctx, url, &boards); err != nil {
			return 0, fmt.Errorf("jira: list boards for project %q: %w", t.projectKey, err)
		}

		for _, board := range boards.Values {
			id, found, err := t.findSprintOnBoard(ctx, board.ID, sprintName)
			if err != nil {
				return 0, err
			}
			if found {
				return id, nil
			}
		}

		if boards.IsLast || len(boards.Values) == 0 {
			break
		}
		startAt += len(boards.Values)
	}

	return 0, fmt.Errorf("jira: no board for project %q has a sprint named %q", t.projectKey, sprintName)
}

func (t *Tracker) findSprintOnBoard(ctx context.Context, boardID int, name string) (int, bool, error) {
	startAt := 0
	for {
		url := fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=active,future,closed&startAt=%d", t.baseURL, boardID, startAt)
		var sprints agileSprintResponse
		if err := t.getAgile(ctx, url, &sprints); err != nil {
			// A board without a sprint backlog (e.g. a kanban board)
			// returns 400; skip it rather than failing startup.
			return 0, false, nil
		}

		for _, sprint := range sprints.Values {
			if sprint.Name == name {
				return sprint.ID, true, nil
			}
		}

		if sprints.IsLast || len(sprints.Values) == 0 {
			return 0, false, nil
		}
		startAt += len(sprints.Values)
	}
}

func (t *Tracker) getAgile(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", t.authHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed (%d): %s", resp.StatusCode, decodeJiraError(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

var requiredIssueTypes = []string{
	string(domain.IssueTypeBug),
	string(domain.IssueTypeTask),
	string(domain.IssueTypeStory),
}

var requiredPriorities = []string{
	string(domain.PriorityHighest),
	string(domain.PriorityHigh),
	string(domain.PriorityMedium),
	string(domain.PriorityLow),
}

// validateCreateMeta checks a parsed createmeta response against this
// bot's required issue types and priority names, and resolves the Sprint
// custom field id. It is split out from ValidateStartup so it can be unit
// tested against a recorded fixture without a live Jira call.
func validateCreateMeta(meta createMetaResponse, projectKey string) (string, error) {
	var project *struct {
		Key        string `json:"key"`
		IssueTypes []struct {
			Name   string                         `json:"name"`
			Fields map[string]createMetaFieldMeta `json:"fields"`
		} `json:"issuetypes"`
	}
	for i := range meta.Projects {
		if meta.Projects[i].Key == projectKey {
			project = &meta.Projects[i]
			break
		}
	}
	if project == nil {
		return "", fmt.Errorf("jira: createmeta has no project %q", projectKey)
	}

	seenIssueTypes := map[string]bool{}
	sprintFieldID := ""
	var seenPriorities map[string]bool

	for _, it := range project.IssueTypes {
		seenIssueTypes[it.Name] = true
		for fieldID, field := range it.Fields {
			if field.Name == "Sprint" {
				sprintFieldID = fieldID
			}
			if field.Name == "Priority" && seenPriorities == nil {
				seenPriorities = map[string]bool{}
				for _, v := range field.AllowedValues {
					seenPriorities[v.Name] = true
				}
			}
		}
	}

	for _, want := range requiredIssueTypes {
		if !seenIssueTypes[want] {
			return "", fmt.Errorf("jira: project %q has no issue type %q", projectKey, want)
		}
	}
	if seenPriorities == nil {
		return "", fmt.Errorf("jira: project %q has no Priority field on any issue type", projectKey)
	}
	for _, want := range requiredPriorities {
		if !seenPriorities[want] {
			return "", fmt.Errorf("jira: project %q has no priority %q", projectKey, want)
		}
	}
	if sprintFieldID == "" {
		return "", fmt.Errorf("jira: project %q has no Sprint field", projectKey)
	}

	return sprintFieldID, nil
}
