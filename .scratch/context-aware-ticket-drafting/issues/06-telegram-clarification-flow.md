# 06 — Telegram adapter: clarification carrier + grouped reply

**What to build:** the bot can ask a stateless clarification question
(carried entirely in its own message), route a reply back to the right use
case, and render one grouped reply per Ticket Request instead of separate
messages per outcome.

**Blocked by:** 03 — Use case: multi-candidate ticket creation,
04 — Use case: clarification answer

**Status:** open

## Scope

- No storage of any kind — the bot's own question message *is* the state.
  The question message contains: a short marker, a one-line restatement of
  each unresolved Candidate, and up to three questions total.
- Handler now inspects every message in an allowlisted chat, not only
  commands. Acts only when a message replies to a bot message carrying the
  clarification marker; everything else is ignored without a Gemini call.
- Replies to `Created <KEY>: ...` confirmations keep routing to `/status`
  exactly as today — marker detection mirrors the existing key-parsing
  helper (a second instance of the same pattern, not a rewrite of it).
- Marker trust model: only a message's own sender can edit it in Telegram,
  and the handler additionally requires the replied-to message came from the
  bot itself. Don't add anything beyond that — no extra crypto/signing, the
  spec is explicit this is "tamper-proof in practice" as-is.
- Expiry: 24 hours after the question was asked, evaluated from the
  replied-to message's own timestamp at the moment the answer arrives (no
  record to sweep). A late answer is ignored silently — no ticket, no error
  message.
- One grouped reply per Ticket Request: created Issues with links, then
  failures, then the question — in that order, not separate messages.
- Telegram's 4096-char cap: when five Candidates would overflow it,
  per-Candidate summaries are truncated. Candidates are never dropped from
  the reply.

## Done when

- An ordinary chat message triggers nothing.
- A reply to a bot confirmation still routes to `/status`.
- A reply to a marked clarification message routes to ticket 04's
  clarification use case.
- A reply to a marked message older than 24 hours is ignored silently.
- A reply to a marked message that did not come from the bot is ignored.
- The grouped reply renders created links, failures, and the question
  together, in that order.
- Five verbose Candidates render within Telegram's character cap without
  losing a Candidate.
- A message from a chat outside the allowlist is ignored — unchanged from
  today.
- Tests against a fake bot client. Prior art:
  `internal/adapters/telegram/telegram_test.go`.

## Notes
