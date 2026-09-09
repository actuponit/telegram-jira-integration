# 05 — Gemini adapter: DraftSet schema + App Context

**What to build:** the `gemini` adapter's `TicketDrafter.Draft` returns a
`DraftSet` from a multi-candidate response schema, and the model gets the
App Context so it can label by feature/product correctly. The adapter also
implements the new clarification method from ticket 01.

**Blocked by:** 01 — Domain & ports, 02 — App Context content file

**Status:** open

## Scope

- Response schema becomes an object: `split_reasoning` (string) +
  `candidates` (array of objects), `MinItems` 1, `MaxItems` 5 on the array.
- `PropertyOrdering` set so `split_reasoning` generates **before**
  `candidates` — load-bearing twice: forces the model to commit to a
  segmentation before drafting, and because `Schema.Properties` is a Go map,
  field order is otherwise nondeterministic.
- Each candidate object: `status`, `app`, `title`, `description`,
  `issue_type`, `priority`, `open_questions`.
- The splitting rule, the vagueness bar, and the labelling rule (see spec's
  "The splitting rule" / "The vagueness bar" / "The labelling rule" sections)
  go into per-field `Description` values on the schema, not the prompt body —
  keeps the request prefix cacheable.
- The App Context (ticket 02's file) is supplied to the adapter at
  construction and rendered into `SystemInstruction`, ahead of the
  conversation, so the request prefix stays stable across requests.
- Model stays `gemini-3.6-flash` per ADR-0001 — do not change it.
- Implement the second `TicketDrafter` method (ticket 01) for the
  clarification path: takes the prior summary + reporter answer, one Gemini
  call, returns a single `Draft`.
- One Gemini call per Ticket Request, one per clarification answer — no
  retries that would multiply quota use, per spec story 39.

## Done when

- A multi-candidate fixture response parses into a `DraftSet` in order.
- A single-candidate fixture response parses.
- An unknown `app`, `status`, `issue_type`, or `priority` value is a parse
  error, matching how existing enums already fail.
- A `NeedsClarification` candidate carrying no `open_questions` is a parse
  error.
- Malformed JSON is a parse error.
- Contract tests run against recorded fixtures, no live Gemini call. Prior
  art: `internal/adapters/gemini/gemini_test.go` and its `testdata/`.

## Notes
