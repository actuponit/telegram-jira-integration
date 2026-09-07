package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
)

// ParseChatAllowlist reads a comma-separated list of Telegram chat IDs
// (e.g. "-1001234567890,-1009876543210") into a domain.ChatAllowlist.
//
// It rejects a list that names no chat: an empty allowlist would silence
// the bot everywhere, which is far more likely a deployment mistake than an
// intent, so startup fails fast instead.
func ParseChatAllowlist(raw string) (domain.ChatAllowlist, error) {
	var ids []int64
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return domain.ChatAllowlist{}, fmt.Errorf("config: chat allowlist entry %q is not a chat ID: %w", field, err)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return domain.ChatAllowlist{}, fmt.Errorf("config: chat allowlist %q names no chat ID", raw)
	}
	return domain.NewChatAllowlist(ids...), nil
}
