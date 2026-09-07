# 03 — Chat allowlist

**What to build:** commands from outside the one allowlisted Telegram chat are ignored outright — ticket creation stays scoped to the intended group.

**Blocked by:** 02 — Core ticket creation

**Status:** ready-for-agent

- [ ] Allowlisted chat ID is configurable (env/config, not hardcoded)
- [ ] `/to-ticket` and `/status` from any other chat ID produce no bot response and no side effects
- [ ] Ignored commands are still logged (chat ID visible) for diagnosability
- [ ] No per-user allowlist inside the group — chat-level only
