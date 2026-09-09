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

// CreatedTicket is one Issue created from a Ready Candidate.
type CreatedTicket struct {
	Ticket             domain.Ticket
	Title              string
	AssigneeUnresolved bool
}

// FailedCandidate is a Ready Candidate whose Issue creation failed. Err is
// always wrapped with ErrCreateIssueFailed.
type FailedCandidate struct {
	Draft domain.Draft
	Err   error
}

// CreateTicketResult is what the inbound adapter renders back to Telegram:
// every Issue created, every Candidate whose creation failed, and — at
// most once — the Candidates still needing the reporter's clarification.
type CreateTicketResult struct {
	Created       []CreatedTicket
	Failed        []FailedCandidate
	Clarification []domain.Candidate
}

// ErrDraftFailed and ErrCreateIssueFailed let callers distinguish which
// stage of CreateTicketFromMessage failed (via errors.Is) without parsing
// error text, since the telegram adapter renders a different chat message
// for a drafting failure than for an issue-creation failure.
var (
	ErrDraftFailed       = errors.New("draft ticket failed")
	ErrCreateIssueFailed = errors.New("create issue failed")
)

// CreateTicketFromMessage is the Ticket Request flow: draft a ticket from
// the gathered message context, resolve the optional assignee, and create
// one Issue per Ready Candidate. A Candidate that fails to create is
// recorded in the result rather than aborting the rest — there is no
// rollback. NeedsClarification Candidates never reach IssueTracker; they
// are carried back as-is for the caller to turn into a clarification
// request.
func CreateTicketFromMessage(ctx context.Context, drafter ports.TicketDrafter, tracker ports.IssueTracker, resolver ports.AssigneeResolver, req CreateTicketRequest) (CreateTicketResult, error) {
	if len(req.ContextMessages) == 0 {
		return CreateTicketResult{}, errors.New("create ticket: no source message in context")
	}

	draftSet, err := drafter.Draft(ctx, req.ContextMessages)
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("%w: %w", ErrDraftFailed, err)
	}

	source := req.ContextMessages[len(req.ContextMessages)-1]

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

	var result CreateTicketResult
	for _, candidate := range draftSet.Candidates {
		if candidate.Status != domain.CandidateReady {
			result.Clarification = append(result.Clarification, candidate)
			continue
		}

		draft := candidate.Draft
		draft.Description = fmt.Sprintf("%s\n\nReported via Telegram by %s.", draft.Description, source.SenderName)
		if req.VideoURL != "" {
			draft.Description = fmt.Sprintf("%s\n\nVideo: %s", draft.Description, req.VideoURL)
		}

		ticket, err := tracker.CreateIssue(ctx, draft, assignee, req.ImageAttachment)
		if err != nil {
			result.Failed = append(result.Failed, FailedCandidate{Draft: draft, Err: fmt.Errorf("%w: %w", ErrCreateIssueFailed, err)})
			continue
		}
		result.Created = append(result.Created, CreatedTicket{Ticket: ticket, Title: draft.Title, AssigneeUnresolved: unresolved})
	}

	return result, nil
}
