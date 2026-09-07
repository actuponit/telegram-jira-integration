// Package ports defines the interfaces internal/usecase depends on,
// named for what the use case needs rather than the vendor behind them.
package ports

import (
	"context"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
)

// Message is one message in the context gathered for a Ticket Request:
// the source message plus its own reply chain.
type Message struct {
	SenderName string
	Text       string
	Timestamp  int64
}

// TicketDrafter produces a Draft from a source message and its reply-chain
// context. Implemented by the gemini adapter.
type TicketDrafter interface {
	Draft(ctx context.Context, messages []Message) (domain.Draft, error)
}

// IssueTracker creates Issues and reports on their current state.
// Implemented by the jira adapter.
type IssueTracker interface {
	CreateIssue(ctx context.Context, draft domain.Draft, assignee domain.Assignee) (domain.Ticket, error)
	GetIssueStatus(ctx context.Context, key string) (domain.Ticket, error)
}

// AssigneeResolver resolves a Telegram handle to a Jira Assignee.
// Implemented by the config adapter's static mapping.
type AssigneeResolver interface {
	Resolve(ctx context.Context, telegramHandle string) (domain.Assignee, bool)
}
