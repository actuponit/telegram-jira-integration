package domain_test

import (
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
)

func TestChatAllowlist_AllowsOnlyListedChats(t *testing.T) {
	allowlist := domain.NewChatAllowlist(-100123, 456)

	if !allowlist.Allows(-100123) {
		t.Error("listed chat -100123 should be allowed")
	}
	if !allowlist.Allows(456) {
		t.Error("listed chat 456 should be allowed")
	}
	if allowlist.Allows(999) {
		t.Error("unlisted chat 999 should not be allowed")
	}
}

func TestChatAllowlist_ZeroValueFailsClosed(t *testing.T) {
	var allowlist domain.ChatAllowlist

	if allowlist.Allows(0) || allowlist.Allows(-100123) {
		t.Error("zero-value allowlist should allow nothing")
	}
}

func TestChatAllowlist_EmptyConstructedListFailsClosed(t *testing.T) {
	allowlist := domain.NewChatAllowlist()

	if allowlist.Allows(-100123) {
		t.Error("empty allowlist should allow nothing")
	}
}
