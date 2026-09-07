package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
)

// ErrCheckStatusFailed lets the inbound adapter distinguish a status lookup
// failure (via errors.Is) from any other error without parsing error text.
// The wrapped error carries the tracker's own message — a not-found or
// inaccessible Issue reads differently in chat than a transport failure.
var ErrCheckStatusFailed = fmt.Errorf("check ticket status failed")

// CheckTicketStatus looks up the current state of an already-created Issue
// by its key. The rendering of the returned Ticket into a chat reply is the
// inbound adapter's job.
func CheckTicketStatus(ctx context.Context, tracker ports.IssueTracker, key string) (domain.Ticket, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return domain.Ticket{}, fmt.Errorf("%w: no issue key", ErrCheckStatusFailed)
	}

	ticket, err := tracker.GetIssueStatus(ctx, key)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("%w: %w", ErrCheckStatusFailed, err)
	}
	return ticket, nil
}
