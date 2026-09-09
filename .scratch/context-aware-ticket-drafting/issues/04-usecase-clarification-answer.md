# 04 — Use case: clarification answer

**What to build:** a second use case that takes the bot's prior summary text
plus the reporter's reply and turns it into exactly one Issue.

**Blocked by:** 01 — Domain & ports

**Status:** done

## Scope

- New use case (separate from ticket 03's, not a conditional branch of it):
  takes the bot's prior summary text and the reporter's answer, calls
  `TicketDrafter`'s clarification method (ticket 01), and creates exactly one
  Issue via `IssueTracker`.
- Never produces further questions — the redraft always yields a `Ready`
  Draft. If the drafter's clarification method were to somehow return
  something other than Ready, that's a drafter contract violation, not
  something this use case needs a branch for — treat it as an error.
- Per spec's Fan-out and failure section: clarification-path Issues are
  created **unassigned and without the attachment**. Neither survives on the
  carrier (there's no assignee or image to carry across the stateless
  question/answer boundary — see ticket 06), and both are cheap for a human
  to fix in Jira. Do not try to recover the original assignee or image.

## Done when

- The clarification use case produces exactly one Issue, unassigned, with no
  attachment.
- A drafter error on the clarification call is distinguishable from a
  tracker error, same sentinel-error pattern as ticket 03.
- Tests against fake ports, asserting the created Issue's shape — prior art:
  `internal/usecase/create_ticket_test.go`.

## Notes
