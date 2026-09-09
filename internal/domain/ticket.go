// Package domain holds pure business types shared by usecase and adapters.
// No imports beyond the standard library.
package domain

import "fmt"

// Priority is a Jira issue priority.
type Priority string

const (
	PriorityHighest Priority = "Highest"
	PriorityHigh    Priority = "High"
	PriorityMedium  Priority = "Medium"
	PriorityLow     Priority = "Low"
)

// IssueType is the kind of Jira issue to create.
type IssueType string

const (
	IssueTypeBug   IssueType = "Bug"
	IssueTypeTask  IssueType = "Task"
	IssueTypeStory IssueType = "Story"
)

// Assignee is a resolved Jira account to assign an Issue to.
// A zero-value Assignee (empty AccountID) means unassigned.
type Assignee struct {
	AccountID   string
	DisplayName string
}

// IsUnassigned reports whether this Assignee resolves to no one.
func (a Assignee) IsUnassigned() bool {
	return a.AccountID == ""
}

// App is the Mela product a Draft's problem was observed in. It is
// single-valued and mutually exclusive, and is written to the Issue as a
// label.
type App string

const (
	AppMelaApp     App = "Mela App"
	AppMerchantApp App = "Merchant App"
	AppBackend     App = "Backend"
)

// ParseApp validates s against the three named App values, rejecting
// anything else, matching how Priority and IssueType are enforced at parse
// time by the gemini adapter.
func ParseApp(s string) (App, error) {
	switch app := App(s); app {
	case AppMelaApp, AppMerchantApp, AppBackend:
		return app, nil
	default:
		return "", fmt.Errorf("domain: unknown app %q", s)
	}
}

// Draft is the structured ticket content produced from a source message,
// not yet a Jira Issue.
type Draft struct {
	Title       string
	Description string
	App         App
	IssueType   IssueType
	Priority    Priority
	Labels      []string
}

// CandidateStatus reports whether a Candidate is ready to become an Issue
// or still needs the reporter to clarify what they want changed.
type CandidateStatus string

const (
	CandidateReady              CandidateStatus = "Ready"
	CandidateNeedsClarification CandidateStatus = "NeedsClarification"
)

// Candidate is a proposed Issue split out of a Ticket Request, not yet
// created. It sits between Draft and Issue in the glossary: a Draft plus
// whether it's ready to file and, if not, what's missing.
type Candidate struct {
	Draft         Draft
	Status        CandidateStatus
	OpenQuestions []string
}

// DraftSet is everything a single Ticket Request draft call produces: the
// model's reasoning for how it split the conversation, plus the resulting
// Candidates.
type DraftSet struct {
	SplitReasoning string
	Candidates     []Candidate
}

// Ticket is the Jira Issue created from a Draft.
type Ticket struct {
	Key      string
	URL      string
	Assignee Assignee
	Status   string
}
