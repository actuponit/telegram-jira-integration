# 08 — Docs: ADR amendment, new ADR, glossary

**What to build:** the documentation this spec's design decisions imply, per
its "Further Notes" section — an ADR-0001 amendment, a new ADR for the
stateless clarification carrier, and `CONTEXT.md` glossary additions.

**Blocked by:** 01 — Domain & ports

**Status:** open

## Scope

- Amend ADR-0001: narrow its "there is no confirm/edit step before creation"
  clause. Fire-and-forget still governs everything the bot can draft; a
  clarification question is not a confirmation because nothing sits pending
  approval. Record this as an amendment, not a silent contradiction.
- New ADR: the stateless clarification carrier — the bot's own message
  holds the state, expiry is read from its timestamp. Document the
  reasoning: Telegram's one-level reply-nesting cap and the Bot API's
  missing `getMessage` are what rule out a stored-state alternative.
- `CONTEXT.md`: add `Candidate`, `App Context`, `Clarification`, and `App` to
  the glossary. Update the existing `Draft` entry — a Ticket Request now
  yields a `DraftSet`, not a `Draft`.

## Done when

- ADR-0001 has an amendment section (not an edit that erases the original
  decision) narrowing the confirm/edit clause.
- A new ADR file exists documenting the stateless carrier decision and its
  Telegram-API-shaped reasoning.
- `CONTEXT.md` glossary has all four new terms and an updated `Draft` entry.

## Notes
