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

// TicketDrafter produces a DraftSet from a source message and its
// reply-chain context, and redrafts a single Candidate once the reporter
// has answered a clarification question. Implemented by the gemini
// adapter.
type TicketDrafter interface {
	Draft(ctx context.Context, messages []Message) (domain.DraftSet, error)

	// DraftFromAnswer redrafts a single Draft from the bot's own prior
	// summary of an unresolved Candidate and the reporter's reply to it.
	// Unlike Draft, its input is a summary and an answer, not a message
	// thread.
	DraftFromAnswer(ctx context.Context, summary, answer string) (domain.Draft, error)
}

// Attachment is an image downloaded from the source message, to be
// re-uploaded to the created Issue. Video is never downloaded — see
// CreateTicketRequest.VideoURL in internal/usecase.
type Attachment struct {
	Filename string
	Data     []byte
}

// IssueTracker creates Issues and reports on their current state.
// Implemented by the jira adapter. attachment is nil when the source
// message carries no image.
type IssueTracker interface {
	CreateIssue(ctx context.Context, draft domain.Draft, assignee domain.Assignee, attachment *Attachment) (domain.Ticket, error)
	GetIssueStatus(ctx context.Context, key string) (domain.Ticket, error)
}

// AssigneeResolver resolves a Telegram handle to a Jira Assignee.
// Implemented by the config adapter's static mapping.
type AssigneeResolver interface {
	Resolve(ctx context.Context, telegramHandle string) (domain.Assignee, bool)
}
