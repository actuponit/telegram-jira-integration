# ADR-0003: Stateless clarification carrier

## Status

Accepted (2026-09-10, from the context-aware multi-ticket drafting spec —
see `.scratch/context-aware-ticket-drafting/spec.md`).

## Context

A Candidate the bot cannot draft — where it cannot tell what the reporter
wants changed — is not filed immediately. Instead the bot asks a question in
the chat, and the reporter's reply must produce the Issue. Something has to
carry, from the question to the answer, what the bot already understood and
which Candidates are still open.

The obvious carrier is a store keyed by message ID: save the pending
Candidates when the question is asked, look them up when the reply arrives.
Two facts about the Telegram Bot API rule this out, and rule out every
pointer-based alternative with it:

- **Reply nesting caps at one level.** A reply to the bot's question yields
  the question message, never the original Ticket Request the question was
  about. There is no thread to walk back up.
- **There is no `getMessage`.** A message ID alone cannot later be exchanged
  for that message's content. Even if the handler stored only an ID, it
  could not resolve it back to text when the answer arrived — it would still
  need the substance stored alongside it.

ADR-0001 already rules out a database for this project. Even setting that
aside, no ID-based scheme survives the two constraints above: the pointer
has nothing on the other end to resolve to.

## Decision

The bot's own question message *is* the state. There is no store.

- The question message contains a short marker, a one-line restatement of
  each unresolved Candidate, and up to three questions.
- When the reporter replies to it, the handler reads the marker and the
  restatement straight out of the replied-to message — the same message
  Telegram hands the webhook — and redrafts from that plus the reporter's
  answer.
- Expiry needs no sweep: the replied-to message's own timestamp is compared
  against a 24-hour window at the moment the answer arrives. A late answer
  is ignored silently — no ticket, no error message.
- The marker is tamper-proof in practice: only a message's own sender can
  edit it in Telegram, and the handler additionally requires the replied-to
  message came from the bot itself.

## Consequences

- The redraft works from the bot's summary of the Candidate, not the
  original message thread. Accepted: the original was by definition too
  vague to draft from, and the reporter sees and can correct the summary in
  the same reply.
- No cross-restart durability is needed or provided — the carrier lives
  entirely in Telegram's own message history, which outlives any bot
  redeploy.
- Deployment stays a single stateless Cloud Run service, consistent with
  ADR-0001's no-database decision.
- If Telegram ever lifts the one-level nesting cap or ships `getMessage`,
  this constraint disappears and a pointer-based carrier becomes viable —
  nothing here should be read as ruling that out permanently.
