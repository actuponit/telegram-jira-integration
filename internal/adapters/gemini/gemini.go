// Package gemini implements ports.TicketDrafter using the Gemini API.
package gemini

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
)

const model = "gemini-3.6-flash"

const baseSystemInstruction = `You draft Jira tickets from a Telegram conversation.
Stay strictly factual: only use details present in the conversation, never invent
information that isn't there. If the severity of the issue is unclear, default
priority to Medium.`

//go:embed appcontext.md
var appContext string

// draftSchema is the response shape for a single redraft — used by
// DraftFromAnswer, whose input is a summary and an answer rather than a
// message thread, so there is no split to reason about.
var draftSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"app": {
			Type:        genai.TypeString,
			Enum:        []string{string(domain.AppMelaApp), string(domain.AppMerchantApp), string(domain.AppBackend)},
			Description: "The app the problem was observed in. Choose Backend only on an explicit server, API or data-side signal, or when the symptom is app-agnostic (the same wrong data in both apps, or no UI surface at all). Otherwise label by where the problem was observed. Never infer which layer a fix belongs to.",
		},
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
	Required: []string{"app", "title", "description", "issue_type", "priority"},
}

// candidateSchema is one element of draftSetSchema's candidates array.
var candidateSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"status": {
			Type:        genai.TypeString,
			Enum:        []string{string(domain.CandidateReady), string(domain.CandidateNeedsClarification)},
			Description: "Ready if you can name something observable that should be different. Missing repro steps, device, severity or exact screen are gaps to note in the description, not reasons for NeedsClarification. Use NeedsClarification only when you cannot tell what the reporter wants changed.",
		},
		"app": {
			Type:        genai.TypeString,
			Enum:        []string{string(domain.AppMelaApp), string(domain.AppMerchantApp), string(domain.AppBackend)},
			Description: "The app the problem was observed in. Choose Backend only on an explicit server, API or data-side signal, or when the symptom is app-agnostic (the same wrong data in both apps, or no UI surface at all). Otherwise label by where the problem was observed. Never infer which layer a fix belongs to.",
		},
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
		"open_questions": {
			Type:        genai.TypeArray,
			Items:       &genai.Schema{Type: genai.TypeString},
			Description: "Required and non-empty when status is NeedsClarification: the specific things you cannot tell from the conversation. Everyday language, no engineering jargon. Omit or leave empty when status is Ready.",
		},
	},
	Required:         []string{"status", "app", "title", "description", "issue_type", "priority"},
	PropertyOrdering: []string{"status", "app", "title", "description", "issue_type", "priority", "open_questions"},
}

// draftSetSchema is the response shape for Draft: the model's reasoning for
// how it split the conversation, plus one to five Candidates.
var draftSetSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"split_reasoning": {
			Type:        genai.TypeString,
			Description: "Explain your segmentation before drafting. Split only where each piece could ship alone and someone would notice an improvement — a piece only meaningful once another piece is done is a step, not a Candidate; merge it. Feature area is the tiebreaker: complaints in the same feature area are usually one Candidate, complaints in different areas are usually two. Default to one Candidate; a second requires positive evidence.",
		},
		"candidates": {
			Type:     genai.TypeArray,
			Items:    candidateSchema,
			MinItems: genai.Ptr(int64(1)),
			MaxItems: genai.Ptr(int64(5)),
		},
	},
	// split_reasoning must generate before candidates: it forces the model
	// to commit to a segmentation before drafting, and Schema.Properties is
	// a Go map, so without PropertyOrdering field order is nondeterministic.
	Required:         []string{"split_reasoning", "candidates"},
	PropertyOrdering: []string{"split_reasoning", "candidates"},
}

// Drafter implements ports.TicketDrafter against the Gemini API.
type Drafter struct {
	client            *genai.Client
	systemInstruction string
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
	return &Drafter{
		client:            client,
		systemInstruction: fmt.Sprintf("%s\n\n%s", baseSystemInstruction, appContext),
	}, nil
}

// Draft renders messages into prompt content and asks Gemini for a
// structured DraftSet: its reasoning for how it split the conversation,
// plus one Candidate per independent problem.
func (d *Drafter) Draft(ctx context.Context, messages []ports.Message) (domain.DraftSet, error) {
	contents := make([]*genai.Content, 0, len(messages))
	for _, m := range messages {
		text := fmt.Sprintf("[%s @ %d] %s", m.SenderName, m.Timestamp, m.Text)
		contents = append(contents, genai.NewContentFromParts([]*genai.Part{{Text: text}}, genai.RoleUser))
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromParts([]*genai.Part{{Text: d.systemInstruction}}, genai.RoleUser),
		ResponseMIMEType:  "application/json",
		ResponseSchema:    draftSetSchema,
	}

	resp, err := d.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return domain.DraftSet{}, fmt.Errorf("gemini: generate content: %w", err)
	}

	return parseDraftSet([]byte(resp.Text()))
}

// DraftFromAnswer redrafts a single Draft from the bot's prior summary and
// the reporter's answer.
func (d *Drafter) DraftFromAnswer(ctx context.Context, summary, answer string) (domain.Draft, error) {
	prompt := fmt.Sprintf("You previously summarised an unresolved report as:\n%s\n\nThe reporter replied:\n%s\n\nDraft the ticket now, using whatever the reporter gave you even if some detail is still missing.", summary, answer)
	contents := []*genai.Content{genai.NewContentFromParts([]*genai.Part{{Text: prompt}}, genai.RoleUser)}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromParts([]*genai.Part{{Text: d.systemInstruction}}, genai.RoleUser),
		ResponseMIMEType:  "application/json",
		ResponseSchema:    draftSchema,
	}

	resp, err := d.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return domain.Draft{}, fmt.Errorf("gemini: generate content: %w", err)
	}

	return parseDraft([]byte(resp.Text()))
}

// draftResponse is the shape Gemini returns per draftSchema.
type draftResponse struct {
	App         string   `json:"app"`
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

	app, err := domain.ParseApp(r.App)
	if err != nil {
		return domain.Draft{}, fmt.Errorf("gemini: %w", err)
	}

	issueType, err := parseIssueType(r.IssueType)
	if err != nil {
		return domain.Draft{}, err
	}

	priority, err := parsePriority(r.Priority)
	if err != nil {
		return domain.Draft{}, err
	}

	return domain.Draft{
		Title:       r.Title,
		Description: r.Description,
		App:         app,
		IssueType:   issueType,
		Priority:    priority,
		Labels:      r.Labels,
	}, nil
}

// parseIssueType and parsePriority are shared by parseDraft and
// parseCandidate so the set of valid enum values lives in one place.
func parseIssueType(s string) (domain.IssueType, error) {
	issueType := domain.IssueType(s)
	switch issueType {
	case domain.IssueTypeBug, domain.IssueTypeTask, domain.IssueTypeStory:
		return issueType, nil
	default:
		return "", fmt.Errorf("gemini: unknown issue_type %q", s)
	}
}

func parsePriority(s string) (domain.Priority, error) {
	priority := domain.Priority(s)
	switch priority {
	case domain.PriorityHighest, domain.PriorityHigh, domain.PriorityMedium, domain.PriorityLow:
		return priority, nil
	default:
		return "", fmt.Errorf("gemini: unknown priority %q", s)
	}
}

// draftSetResponse is the shape Gemini returns per draftSetSchema.
type draftSetResponse struct {
	SplitReasoning string              `json:"split_reasoning"`
	Candidates     []candidateResponse `json:"candidates"`
}

type candidateResponse struct {
	Status        string   `json:"status"`
	App           string   `json:"app"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	IssueType     string   `json:"issue_type"`
	Priority      string   `json:"priority"`
	OpenQuestions []string `json:"open_questions"`
}

// parseDraftSet unmarshals a Gemini structured-output response into a
// domain.DraftSet, in candidate order.
func parseDraftSet(data []byte) (domain.DraftSet, error) {
	var r draftSetResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return domain.DraftSet{}, fmt.Errorf("gemini: parse response: %w", err)
	}

	candidates := make([]domain.Candidate, 0, len(r.Candidates))
	for _, c := range r.Candidates {
		candidate, err := parseCandidate(c)
		if err != nil {
			return domain.DraftSet{}, err
		}
		candidates = append(candidates, candidate)
	}

	return domain.DraftSet{
		SplitReasoning: r.SplitReasoning,
		Candidates:     candidates,
	}, nil
}

func parseCandidate(c candidateResponse) (domain.Candidate, error) {
	status := domain.CandidateStatus(c.Status)
	switch status {
	case domain.CandidateReady, domain.CandidateNeedsClarification:
	default:
		return domain.Candidate{}, fmt.Errorf("gemini: unknown status %q", c.Status)
	}

	app, err := domain.ParseApp(c.App)
	if err != nil {
		return domain.Candidate{}, fmt.Errorf("gemini: %w", err)
	}

	issueType, err := parseIssueType(c.IssueType)
	if err != nil {
		return domain.Candidate{}, err
	}

	priority, err := parsePriority(c.Priority)
	if err != nil {
		return domain.Candidate{}, err
	}

	if status == domain.CandidateNeedsClarification && len(c.OpenQuestions) == 0 {
		return domain.Candidate{}, fmt.Errorf("gemini: NeedsClarification candidate %q carries no open_questions", c.Title)
	}

	return domain.Candidate{
		Draft: domain.Draft{
			Title:       c.Title,
			Description: c.Description,
			App:         app,
			IssueType:   issueType,
			Priority:    priority,
		},
		Status:        status,
		OpenQuestions: c.OpenQuestions,
	}, nil
}
