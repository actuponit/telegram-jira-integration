// Package config implements ports.AssigneeResolver and loads secrets.
package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
	"gopkg.in/yaml.v3"
)

var _ ports.AssigneeResolver = (*Resolver)(nil)

type mappingEntry struct {
	JiraAccountID string `yaml:"jira_account_id"`
	DisplayName   string `yaml:"display_name"`
}

// Resolver implements ports.AssigneeResolver from a static mapping loaded
// once at startup — see ADR-0001 (config file, not a database).
type Resolver struct {
	mapping map[string]domain.Assignee
}

// Load reads the Assignee Mapping from a YAML file at path. It fails fast
// with a clear error on a missing or malformed file rather than panicking.
func Load(path string) (*Resolver, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: reading assignee mapping %q: %w", path, err)
	}

	var raw map[string]mappingEntry
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: parsing assignee mapping %q: %w", path, err)
	}

	mapping := make(map[string]domain.Assignee, len(raw))
	for handle, entry := range raw {
		mapping[normalizeHandle(handle)] = domain.Assignee{
			AccountID:   entry.JiraAccountID,
			DisplayName: entry.DisplayName,
		}
	}

	return &Resolver{mapping: mapping}, nil
}

// Resolve looks up telegramHandle in the Assignee Mapping. Handles are
// matched case-insensitively and tolerate a leading "@" and surrounding
// whitespace, since callers may pass either the raw command argument or a
// handle typed straight from chat.
func (r *Resolver) Resolve(ctx context.Context, telegramHandle string) (domain.Assignee, bool) {
	assignee, ok := r.mapping[normalizeHandle(telegramHandle)]
	return assignee, ok
}

func normalizeHandle(handle string) string {
	handle = strings.TrimSpace(handle)
	handle = strings.TrimPrefix(handle, "@")
	return strings.ToLower(handle)
}
