package domain

// ChatAllowlist is the set of Telegram chat IDs the bot answers in.
// Membership is chat-level only — there is no per-user allowlist inside a
// group.
//
// The zero value allows nothing. This bot fails closed: a misconfigured or
// forgotten allowlist means the bot stays silent rather than answering
// every chat it happens to be added to.
type ChatAllowlist struct {
	ids map[int64]struct{}
}

// NewChatAllowlist builds an allowlist from the given chat IDs.
func NewChatAllowlist(ids ...int64) ChatAllowlist {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return ChatAllowlist{ids: set}
}

// Allows reports whether the bot should act on commands from chatID.
func (a ChatAllowlist) Allows(chatID int64) bool {
	_, ok := a.ids[chatID]
	return ok
}
