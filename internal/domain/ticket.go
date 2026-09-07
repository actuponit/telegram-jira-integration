// Package domain holds pure business types shared by usecase and adapters.
// No imports beyond the standard library.
package domain

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

// Draft is the structured ticket content produced from a source message,
// not yet a Jira Issue.
type Draft struct {
	Title       string
	Description string
	IssueType   IssueType
	Priority    Priority
	Labels      []string
}

// Ticket is the Jira Issue created from a Draft.
type Ticket struct {
	Key      string
	URL      string
	Assignee Assignee
	Status   string
}
