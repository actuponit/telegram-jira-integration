// Package usecase orchestrates end-to-end flows against the ports interfaces.
// It imports only internal/domain and internal/ports — never a concrete
// adapter.
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
)

// CreateTicketRequest is the input to CreateTicketFromMessage.
//
// ContextMessages is the source message plus its own reply chain, ordered
// oldest to newest, with the source message last.
type CreateTicketRequest struct {
	ContextMessages []ports.Message
	TelegramHandle  string

	// ImageAttachment is the image on the source message, already
	// downloaded by the telegram adapter. Nil when the source message
	// carries no image.
	ImageAttachment *ports.Attachment

	// VideoURL is the download URL of a video on the source message.
	// Never downloaded — only linked into the Issue description.
	VideoURL string
}

// CreateTicketResult is what the inbound adapter renders back to Telegram.
type CreateTicketResult struct {
	Ticket             domain.Ticket
	AssigneeUnresolved bool
}

// CreateTicketFromMessage is the Ticket Request flow: draft a ticket from
// the gathered message context, resolve the optional assignee, and create
// the Issue.
func CreateTicketFromMessage(ctx context.Context, drafter ports.TicketDrafter, tracker ports.IssueTracker, resolver ports.AssigneeResolver, req CreateTicketRequest) (CreateTicketResult, error) {
	if len(req.ContextMessages) == 0 {
		return CreateTicketResult{}, errors.New("create ticket: no source message in context")
	}

	draft, err := drafter.Draft(ctx, req.ContextMessages)
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("draft ticket: %w", err)
	}

	source := req.ContextMessages[len(req.ContextMessages)-1]
	draft.Description = fmt.Sprintf("%s\n\nReported via Telegram by %s.", draft.Description, source.SenderName)
	if req.VideoURL != "" {
		draft.Description = fmt.Sprintf("%s\n\nVideo: %s", draft.Description, req.VideoURL)
	}

	var assignee domain.Assignee
	unresolved := false
	if req.TelegramHandle != "" {
		resolved, ok := resolver.Resolve(ctx, req.TelegramHandle)
		if ok {
			assignee = resolved
		} else {
			unresolved = true
		}
	}

	ticket, err := tracker.CreateIssue(ctx, draft, assignee, req.ImageAttachment)
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("create issue: %w", err)
	}

	return CreateTicketResult{Ticket: ticket, AssigneeUnresolved: unresolved}, nil
}
