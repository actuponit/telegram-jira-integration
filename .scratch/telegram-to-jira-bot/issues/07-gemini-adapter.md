# 07 — Gemini TicketDrafter adapter

**What to build:** the `gemini` adapter implements `ports.TicketDrafter` using `google.golang.org/genai`.

**Blocked by:** 01 — Project scaffold

**Status:** done

- [x] Implements `ports.TicketDrafter.Draft(ctx, []ports.Message) (domain.Draft, error)`
- [x] Uses `google.golang.org/genai`, model `gemini-2.5-flash`
- [x] `responseSchema`/`responseMimeType: application/json` returning `{title, description, issue_type, priority, labels}`; `issue_type` enum `Bug`/`Task`/`Story`, `priority` enum `Highest`/`High`/`Medium`/`Low`
- [x] System instruction: stay factual, never invent details absent from the conversation, default to Medium priority when severity is unclear
- [x] Each `ports.Message` (sender name + timestamp + text) rendered into the prompt content, not flattened into an unattributed blob
- [x] Gemini timeout/error returns a plain Go `error` — no panics, no partial `domain.Draft`
- [x] Contract test against a recorded structured-output fixture (saved JSON response) confirms the schema still round-trips into a `domain.Draft` — no live Gemini call in this test

## Comments

Split out of ticket 02 (Core ticket creation) — see 02's tracking note.
