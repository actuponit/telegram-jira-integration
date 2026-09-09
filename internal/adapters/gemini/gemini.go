// Package gemini implements ports.TicketDrafter using the Gemini API.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
)

const model = "gemini-3.6-flash"

const systemInstruction = `You draft Jira tickets from a Telegram conversation.
Stay strictly factual: only use details present in the conversation, never invent
information that isn't there. If the severity of the issue is unclear, default
priority to Medium.`

var responseSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"title":       {Type: genai.TypeString},
		"description": {Type: genai.TypeString},
		"issue_type": {
			Type: genai.TypeString,
			Enum: []string{string(domain.IssueTypeBug), string(domain.IssueTypeTask), string(domain.IssueTypeStory)},
		},
		"priority": {
			Type: genai.TypeString,
			Enum: []string{string(domain.PriorityHighest), string(domain.PriorityHigh), string(domain.PriorityMedium), string(domain.PriorityLow)},
		},
		"labels": {
			Type:  genai.TypeArray,
			Items: &genai.Schema{Type: genai.TypeString},
		},
	},
	Required: []string{"title", "description", "issue_type", "priority"},
}

// Drafter implements ports.TicketDrafter against the Gemini API.
type Drafter struct {
	client *genai.Client
}

// New creates a Drafter using the given Gemini API key.
func New(ctx context.Context, apiKey string) (*Drafter, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}
	return &Drafter{client: client}, nil
}

// Draft renders messages into prompt content and asks Gemini for a
// structured ticket draft.
//
// TODO(ticket 05): request/response shape still targets a single Draft,
// not the split-aware DraftSet schema — wrapped here only so the
// TicketDrafter interface compiles.
func (d *Drafter) Draft(ctx context.Context, messages []ports.Message) (domain.DraftSet, error) {
	contents := make([]*genai.Content, 0, len(messages))
	for _, m := range messages {
		text := fmt.Sprintf("[%s @ %d] %s", m.SenderName, m.Timestamp, m.Text)
		contents = append(contents, genai.NewContentFromParts([]*genai.Part{{Text: text}}, genai.RoleUser))
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromParts([]*genai.Part{{Text: systemInstruction}}, genai.RoleUser),
		ResponseMIMEType:  "application/json",
		ResponseSchema:    responseSchema,
	}

	resp, err := d.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return domain.DraftSet{}, fmt.Errorf("gemini: generate content: %w", err)
	}

	draft, err := parseDraft([]byte(resp.Text()))
	if err != nil {
		return domain.DraftSet{}, err
	}

	return domain.DraftSet{
		Candidates: []domain.Candidate{{Draft: draft, Status: domain.CandidateReady}},
	}, nil
}

// DraftFromAnswer redrafts a single Draft from the bot's prior summary and
// the reporter's answer.
//
// TODO(ticket 05): not yet wired to a Gemini call.
func (d *Drafter) DraftFromAnswer(ctx context.Context, summary, answer string) (domain.Draft, error) {
	return domain.Draft{}, fmt.Errorf("gemini: DraftFromAnswer not implemented")
}

// draftResponse is the shape Gemini returns per responseSchema.
type draftResponse struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	IssueType   string   `json:"issue_type"`
	Priority    string   `json:"priority"`
	Labels      []string `json:"labels"`
}

// parseDraft unmarshals a Gemini structured-output response into a domain.Draft.
func parseDraft(data []byte) (domain.Draft, error) {
	var r draftResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return domain.Draft{}, fmt.Errorf("gemini: parse response: %w", err)
	}

	issueType := domain.IssueType(r.IssueType)
	switch issueType {
	case domain.IssueTypeBug, domain.IssueTypeTask, domain.IssueTypeStory:
	default:
		return domain.Draft{}, fmt.Errorf("gemini: unknown issue_type %q", r.IssueType)
	}

	priority := domain.Priority(r.Priority)
	switch priority {
	case domain.PriorityHighest, domain.PriorityHigh, domain.PriorityMedium, domain.PriorityLow:
	default:
		return domain.Draft{}, fmt.Errorf("gemini: unknown priority %q", r.Priority)
	}

	return domain.Draft{
		Title:       r.Title,
		Description: r.Description,
		IssueType:   issueType,
		Priority:    priority,
		Labels:      r.Labels,
	}, nil
}
