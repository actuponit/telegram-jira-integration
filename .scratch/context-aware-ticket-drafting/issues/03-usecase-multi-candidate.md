# 03 — Use case: multi-candidate ticket creation

**What to build:** `CreateTicketFromMessage` becomes plural. One Ticket
Request can now produce several Issues, some Candidates can fail
independently, and at most one clarification request can come back.

**Blocked by:** 01 — Domain & ports

**Status:** done

## Scope

- `internal/usecase.CreateTicketFromMessage`: consumes the `DraftSet` from
  `TicketDrafter.Draft`, iterates `Candidates` in order, creates each `Ready`
  one via `IssueTracker`. A failure on one Candidate is recorded and the loop
  continues — no rollback, ever.
- Returns a result carrying: the Issues created, the Candidates that failed
  to create, and at most one clarification request per Ticket Request. A
  DraftSet can contain more than one `NeedsClarification` Candidate — bundle
  all of them into that single clarification request (one-line restatement
  per unresolved Candidate, up to three questions total). Rendering that into
  the actual Telegram message and the 3-question cap is ticket 06's job; this
  ticket's result type just needs to carry all the unresolved Candidates
  through, not silently drop any past the first.
- `@assignee` (resolved, unresolved, or empty) is applied identically to
  every Issue created from the message — same rules as today
  (`internal/usecase/create_ticket_test.go` prior art), just fanned out.
- The source message's single image attachment is passed to every created
  Issue.
- This is a distinct use case from ticket 04's clarification-answer use
  case — do not merge them into one method with a conditional. Share only
  the Jira-write tail if there's an obvious shared helper; don't force
  sharing beyond that.

## Done when

- A DraftSet of one Candidate creates one Issue — existing behavior,
  unbroken.
- A DraftSet of three Ready Candidates creates three Issues.
- A DraftSet mixing Ready and NeedsClarification creates only the Ready ones
  and surfaces the question.
- A tracker failure on the second of three Candidates still creates the
  first and third, and reports the second as failed.
- The assignee is applied to every Issue; an unresolved handle still creates
  all of them.
- The attachment is passed for every Issue.
- A drafter error is distinguishable from a tracker error via the existing
  sentinel errors.
- All tests live against fake ports, asserting observable outcomes (Issues
  created, text produced) — no call-count assertions, no reaching into
  unexported state. Prior art: `internal/usecase/create_ticket_test.go`.

## Notes
