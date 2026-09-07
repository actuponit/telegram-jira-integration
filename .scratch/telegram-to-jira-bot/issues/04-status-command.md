# 04 — /status command

**What to build:** a Telegram group member replies `/status` to the bot's own "Created <KEY>: ..." confirmation message and gets that Issue's current assignee and workflow status back in the chat.

**Blocked by:** 02 — Core ticket creation

**Status:** ready-for-agent

- [ ] Bot's own confirmation-message key extraction is a plain string-parsing function, unit tested directly (no fakes needed)
- [ ] `/status` as a reply to a valid bot confirmation message resolves the Issue key and calls `CheckTicketStatus`
- [ ] `CheckTicketStatus` use case calls `IssueTracker` (`GET /rest/api/3/issue/{key}`) and returns assignee + status for the adapter to render
- [ ] Reply shows current assignee display name, or "Unassigned"
- [ ] Reply shows current workflow status name
- [ ] `/status` as a reply to anything other than the bot's own confirmation message → clear error reply, not a silent no-op or misparse
- [ ] `/status` on a deleted/inaccessible Issue (404/403 from Jira) → clear chat error, not a crash or silent drop
- [ ] `CheckTicketStatus` tested against in-memory fake of `IssueTracker` — found and not-found/inaccessible cases, no live network calls
