# 03 — Chat allowlist

**What to build:** commands from outside the one allowlisted Telegram chat are ignored outright — ticket creation stays scoped to the intended group.

**Blocked by:** 02 — Core ticket creation

**Status:** done

- [x] Allowlisted chat ID is configurable (env/config, not hardcoded) — `TELEGRAM_ALLOWED_CHATS`, comma-separated, parsed by `config.ParseChatAllowlist` into `domain.ChatAllowlist`
- [x] `/to-ticket` and `/status` from any other chat ID produce no bot response and no side effects — gate sits in `Handler.ServeHTTP` before any goroutine is dispatched
- [x] Ignored commands are still logged (chat ID visible) for diagnosability
- [x] No per-user allowlist inside the group — chat-level only

## Comments

Allowlist membership lives in `internal/domain` (`ChatAllowlist`) rather than
in the telegram adapter: deciding whether to answer a chat is a policy
decision, and adapters translate rather than decide. A chat ID is a plain
`int64`, so the domain stays free of vendor types.

Fails closed: an empty or missing `TELEGRAM_ALLOWED_CHATS` is a startup
error, not a silent "answer nobody" or "answer everybody".
