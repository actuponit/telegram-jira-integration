# 09 — Telegram webhook adapter

**What to build:** the inbound `telegram` adapter — webhook handler, `/to-ticket` parsing, reply-chain gathering, media extraction, and chat replies. Calls `usecase.CreateTicketFromMessage` (wired in ticket 10).

**Blocked by:** 01 — Project scaffold

**Status:** done

- [x] Webhook handler verifies the `secret_token` header on every incoming request; unverified requests are rejected (401, no processing)
- [x] `/to-ticket` with no reply target → usage-hint chat response, not silent failure
- [x] `/to-ticket` with a reply target → gathers the source message plus its own reply chain (if the source message is itself a reply), each tagged with sender name and timestamp, built into `[]ports.Message` — not a flat last-N-messages window
- [x] Optional `@assignee` argument parsed from the command text (empty when omitted)
- [x] Image on the source message downloaded via Telegram `getFile` and turned into a `*ports.Attachment`; images elsewhere in the gathered context are left untouched
- [x] Video on the source message is never downloaded — only its Telegram file URL extracted into `usecase.CreateTicketRequest.VideoURL`
- [x] On `usecase.CreateTicketFromMessage` success: replies in the same thread with `Created <KEY>: <title> — <url>` (parseable later by ticket 04's `/status`); if `AssigneeUnresolved`, the reply also explains the `@assignee` couldn't be resolved
- [x] On `usecase.CreateTicketFromMessage` error: clear chat reply distinguishing a Gemini failure/timeout from a Jira failure (surfacing Jira's `errorMessages` when present) — never a dropped command
- [x] Every attempt (success or failure) logged via `internal/platform/logging` with Telegram message ID, chat ID, and stage-of-failure
- [x] Unit tests for command parsing (no-reply usage hint, assignee arg present/absent), reply-chain assembly, and image/video extraction — against fakes/fixtures, no live Telegram API calls
- [x] Confirmation-message key-extraction function (`Created <KEY>: ...` → `<KEY>`) is a pure string-parsing function, unit-tested directly (needed by ticket 04's `/status`)

## Comments

Split out of ticket 02 (Core ticket creation) — see 02's tracking note.
