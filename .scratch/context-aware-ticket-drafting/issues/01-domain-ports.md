# 01 — Domain & ports: Candidate, App, DraftSet

**What to build:** the domain types and port signatures that every other
ticket in this feature builds on. `internal/domain` gains `Candidate`, `App`,
and `DraftSet`; `internal/ports.TicketDrafter` changes shape.

**Blocked by:** none

**Status:** done

## Scope

- `internal/domain`: new `Candidate` type — a `Draft` plus a `CandidateStatus`
  (`Ready` or `NeedsClarification`) and `OpenQuestions` ([]string).
- `internal/domain`: new `App` type, exactly three values: `Mela App`,
  `Merchant App`, `Backend`. Single-valued, mutually exclusive. Added as a
  field on `Draft`.
- `internal/domain`: new `DraftSet` — `SplitReasoning` (string) plus
  `Candidates` ([]Candidate).
- `Priority` and `IssueType` are unchanged — do not touch them.
- `internal/ports.TicketDrafter.Draft` return type changes from
  `domain.Draft` to `domain.DraftSet`. This is the breaking change — every
  caller and every fake in the existing test suite needs updating to compile.
- `internal/ports.TicketDrafter` gains a second method for the clarification
  path: takes the bot's own prior summary text and the reporter's answer,
  returns a single `domain.Draft`. Separate method, not a flag on `Draft` —
  its input shape (summary + answer) differs from the message-thread shape of
  the first method.
- `internal/ports.IssueTracker` and `internal/ports.AssigneeResolver` are
  unchanged.
- No new port for App Context — it's adapter configuration injected at wiring
  time (ticket 05/07), not a port. Do not add one.

## Done when

- `go build ./...` compiles with the new `DraftSet`-returning
  `TicketDrafter.Draft` signature and the new clarification method on the
  interface (existing adapters/usecases will not yet implement the new
  behavior correctly — that's later tickets — but everything must compile,
  including updated fakes).
- `domain.App` rejects any value outside the three named constants at
  construction/parse time, matching how `Priority`/`IssueType` already
  enforce their enums.
- A `DraftSet` with a `NeedsClarification` Candidate carrying no
  `OpenQuestions` is representable in the type but is the kind of value
  ticket 05's parser must reject — note this for ticket 05, don't enforce it
  here.

## Notes
