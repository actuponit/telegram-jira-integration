# 04 — /status command

**What to build:** a Telegram group member replies `/status` to the bot's own "Created <KEY>: ..." confirmation message and gets that Issue's current assignee and workflow status back in the chat.

**Blocked by:** 02 — Core ticket creation

**Status:** done

- [x] Bot's own confirmation-message key extraction is a plain string-parsing function, unit tested directly (no fakes needed)
- [x] `/status` as a reply to a valid bot confirmation message resolves the Issue key and calls `CheckTicketStatus`
- [x] `CheckTicketStatus` use case calls `IssueTracker` (`GET /rest/api/3/issue/{key}`) and returns assignee + status for the adapter to render
- [x] Reply shows current assignee display name, or "Unassigned"
- [x] Reply shows current workflow status name
- [x] `/status` as a reply to anything other than the bot's own confirmation message → clear error reply, not a silent no-op or misparse
- [x] `/status` on a deleted/inaccessible Issue (404/403 from Jira) → clear chat error, not a crash or silent drop
- [x] `CheckTicketStatus` tested against in-memory fake of `IssueTracker` — found and not-found/inaccessible cases, no live network calls

## Comments

`ParseTicketKey` was already built in ticket 09 and is reused here rather
than reimplemented. `Handler.confirmationKey` additionally checks the
replied-to message was sent by this bot (`bot.Self.ID`), so a user typing
text that looks like a confirmation gets the usage hint instead of a
lookup.

`usecase.CheckTicketStatus` wraps tracker failures in `ErrCheckStatusFailed`
and carries the tracker's own message through, so the jira adapter's
existing 404/403 wording ("issue MA-1 not found", "not permitted to view
issue MA-1") reaches chat unmodified.
