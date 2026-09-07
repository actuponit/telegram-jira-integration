# 02 — Core ticket creation

**What to build:** a Telegram group member replies `/to-ticket [@assignee]` to a message and gets a real Jira Issue back, linked in the same thread. Covers the full `/to-ticket` behavior: webhook security, Draft generation via Gemini using the source message's full reply-chain context, assignee resolution (empty/resolved/unresolved), Issue creation on the fixed Sprint with image/video media handling, and clear chat-visible errors on any failure. This is the bot's core value end to end.

**Blocked by:** 01 — Project scaffold

**Status:** tracking — split into child tickets 06–10 (see Comments). This file now tracks the whole flow; the checklist below is checked off as child tickets land.

- [x] `internal/usecase.CreateTicketFromMessage` orchestrates the flow against `ports.TicketDrafter`/`ports.IssueTracker`/`ports.AssigneeResolver` — tested against in-memory fakes, no live network calls (`internal/usecase/create_ticket_test.go`)
- [x] Empty `@assignee` → Issue created unassigned (no `assignee` key sent to Jira, never `null`) — decided in the usecase, enforced by the `jira` adapter (08)
- [x] Present but unresolved `@assignee` → usecase reports `AssigneeUnresolved`; chat wording is ticket 09's job
- [x] Resolved `@assignee` → Issue created with the resolved `accountId`
- [x] Reporter attribution ("reported via Telegram by X") composed into the Draft description by the usecase, not the jira adapter (keeps ADF-building in the adapter free of business logic)
- [x] Image attachment (`*ports.Attachment`) passed through the usecase to `IssueTracker.CreateIssue` untouched
- [x] Video URL linked into the Draft description by the usecase, never downloaded
- [ ] Telegram webhook handler verifies the `secret_token` on every incoming request — ticket 09
- [ ] `/to-ticket` with no reply target gets a usage-hint response — ticket 09
- [ ] Source message + its own reply chain gathered, each tagged with sender name and timestamp — ticket 09
- [ ] `gemini` adapter implements `TicketDrafter` via `google.golang.org/genai` — ticket 07
- [ ] `config` adapter implements `AssigneeResolver`, loading `configs/assignees.yaml` — ticket 06
- [ ] `jira` adapter implements `IssueTracker`, hand-rolled `net/http`, ADF description, Sprint field, `createmeta` startup validation — ticket 08
- [ ] Image downloaded via Telegram `getFile`; video URL extracted (not downloaded) — ticket 09
- [ ] Chat reply on success (`Created <KEY>: ...`) and on Gemini/Jira failure — ticket 09
- [ ] Every attempt logged with Telegram message ID, chat ID, stage-of-failure — ticket 09
- [ ] `gemini` and `jira` adapters have thin contract tests against recorded fixtures — tickets 07, 08
- [ ] `cmd/bot/main.go` wiring + startup `createmeta` validation — ticket 10

## Comments

Scope was too large for one pass (full Telegram/Gemini/Jira integration end to end). Split after building and testing the `internal/usecase` layer (the primary seam per the spec's Testing Decisions) so each adapter can be built and reviewed independently:

- [06 — Config adapter & Assignee Resolver](06-config-assignee-resolver.md)
- [07 — Gemini adapter](07-gemini-adapter.md)
- [08 — Jira adapter](08-jira-adapter.md)
- [09 — Telegram webhook adapter](09-telegram-webhook-adapter.md)
- [10 — Wiring & startup validation](10-wiring-startup-validation.md)

`internal/ports.IssueTracker.CreateIssue` gained a fourth parameter, `*ports.Attachment` (new type, also in `internal/ports/ports.go`), while building the usecase — needed so the usecase can pass an already-downloaded image through to the jira adapter without the adapter reaching back into Telegram itself.

Tickets 03/04/05's "Blocked by: 02" still holds — 02 resolves once 06–10 all resolve.
