package usecase

import (
	"context"
	"fmt"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
)

// CreateTicketFromAnswer is the clarification-answer flow: it redrafts a
// single Draft from the bot's own prior summary of an unresolved Candidate
// and the reporter's reply, then creates exactly one Issue. Unlike
// CreateTicketFromMessage, there is no fan-out and no further
// clarification — the redraft is always Ready.
//
// The Issue is created unassigned and without an attachment: neither the
// original assignee nor the source image survives the stateless
// question/answer carrier (see ticket 06), and both are cheap for a human
// to fix in Jira.
func CreateTicketFromAnswer(ctx context.Context, drafter ports.TicketDrafter, tracker ports.IssueTracker, summary, answer string) (CreatedTicket, error) {
	draft, err := drafter.DraftFromAnswer(ctx, summary, answer)
	if err != nil {
		return CreatedTicket{}, fmt.Errorf("%w: %w", ErrDraftFailed, err)
	}

	ticket, err := tracker.CreateIssue(ctx, draft, domain.Assignee{}, nil)
	if err != nil {
		return CreatedTicket{}, fmt.Errorf("%w: %w", ErrCreateIssueFailed, err)
	}

	return CreatedTicket{Ticket: ticket, Title: draft.Title}, nil
}
