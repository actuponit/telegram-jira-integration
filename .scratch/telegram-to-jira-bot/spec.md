Status: ready-for-agent

# Telegram → Jira Ticket Bot — Spec

## Problem Statement

Engineering discussion happens in a Telegram group. When a thread resolves into "this should be a ticket," someone has to leave Telegram, open Jira, and manually write up a title, description and issue type from a scroll of chat messages — then come back and paste the link. That context-switch is friction enough that some things that should become tickets just don't.

## Solution

A Telegram bot lives in the work group. Replying to the message that captures the problem with `/to_ticket [@assignee]` turns that message (plus its own reply chain, if any) into a Jira Issue automatically: Gemini drafts a structured ticket from the source message, the bot resolves `@assignee` to a Jira `accountId` via a static mapping (soft-failing to unassigned if it can't), creates the Issue via the Jira REST API v3 with its Sprint field always set to "MA Sprint Board 4", and replies in the same Telegram thread with a link to the new ticket. No confirm step — fire-and-forget, editable afterward directly in Jira. A companion `/status` command, sent as a reply to the bot's own ticket-created confirmation message, looks up that Issue and reports its current assignee and status back into the chat.

## User Stories

1. As a Telegram group member, I want to reply to any message with `/to_ticket`, so that it becomes a Jira Issue without me leaving the chat.
2. As a Telegram group member, I want to add `@assignee` to the command, so that the Issue is pre-assigned to the right person.
3. As a Telegram group member, I want the bot to tell me it couldn't resolve an `@assignee` rather than silently failing, so that I know the Issue was created unassigned and why.
4. As a Telegram group member, I want the bot to still create the Issue when the assignee doesn't resolve, so that a typo'd handle never blocks ticket creation.
5. As a Telegram group member, I want the source message's own reply chain included as context, so that Gemini drafts an accurate ticket even when the problem was described over several messages.
6. As a Telegram group member, I want a reply in the same thread with a direct Jira link, so that I can jump straight to the created Issue.
7. As a Telegram group member, I want an image attached to the source message carried over to the Jira Issue as an attachment, so that visual evidence (e.g. a screenshot of a bug) isn't lost.
8. As a Telegram group member, I want a video attached to the source message linked (not uploaded) in the Issue description, so that large media doesn't block or slow down ticket creation.
9. As a Telegram group member, I want `/to_ticket` with no assignee to create an unassigned Issue, so that I can file a ticket without knowing who should own it yet.
10. As a Telegram group member, I want a clear usage hint if I run `/to_ticket` without replying to a message, so that I understand the command requires a reply.
11. As a Telegram group member outside the allowlisted chat, I want the bot to ignore my commands entirely, so that ticket creation stays scoped to the intended group.
12. As an engineer reviewing a created ticket, I want the Jira description to note it was reported via Telegram and by whom, so that provenance is traceable back to the original conversation.
13. As an engineer, I want Gemini to default to Medium priority when severity is unclear from the conversation, so that priority isn't wildly over- or under-stated.
14. As an engineer, I want Gemini instructed not to invent details absent from the conversation, so that the ticket accurately reflects what was actually reported.
15. As an on-call engineer, I want every ticket-creation attempt (success or failure) logged with the Telegram message ID and chat ID, so that a failed run is diagnosable from Cloud Logging alone.
16. As an on-call engineer, I want Jira's own `errorMessages` surfaced back into the chat on a failed creation, so that the reporter knows what to fix rather than seeing a generic failure.
17. As an on-call engineer, I want the Gemini call's failure/timeout surfaced as a clear chat reply, so that a stuck command doesn't look like it silently did nothing.
18. As the team maintaining the bot, I want the Telegram username → Jira accountId mapping in a hand-edited config file, so that no database is needed for this stage.
19. As the team maintaining the bot, I want the bot to validate the configured Jira project key, issue type names, and priority names against `createmeta` at startup, so that bad config fails fast instead of failing on the first real ticket attempt.
20. As the team operating the bot, I want it deployed on Cloud Run with webhook delivery from day one, so that it scales to zero and needs no long-polling infrastructure.
21. As the team operating the bot, I want all secrets (`TELEGRAM_BOT_TOKEN`, `GEMINI_API_KEY`, `JIRA_API_TOKEN`, webhook `secret_token`) in Google Secret Manager, so that nothing sensitive lives in env vars or source.
22. As the team operating the bot, I want the Telegram webhook secret token verified on every incoming request, so that only genuine Telegram traffic is accepted.
23. As a future maintainer, I want the codebase split into domain / usecase / ports / adapters layers, so that the business rule ("turn a source message into a Jira Issue") is independent of Telegram/Gemini/Jira SDK details and testable without live network calls.
24. As a future maintainer, I want a fourth integration (e.g. a second issue tracker) addable by writing one new adapter against an existing port, so that the use case layer never has to change for that.
25. As a future maintainer, I want the use case layer tested against in-memory fakes of its ports, so that unit tests run with no live Telegram/Gemini/Jira calls.
26. As a future maintainer, I want adapters covered by thin contract tests against recorded fixtures (a saved `createmeta` response, a saved Gemini structured-output response), so that vendor API shape changes are caught without full integration coverage.
27. As a Telegram group member, I want every Issue created via `/to_ticket` placed on "MA Sprint Board 4" automatically, so that new tickets show up on the active sprint without a manual Jira edit.
28. As a Telegram group member, I want to reply `/status` to the bot's own "Created PROJ-482..." confirmation message, so that I can check who's assigned and what state the ticket is in without leaving Telegram.
29. As a Telegram group member, I want `/status` to reply with the Issue's current assignee (or "Unassigned") and its current workflow status, so that I get a quick answer without opening Jira.
30. As a Telegram group member, I want a clear error if I send `/status` as a reply to something other than the bot's own confirmation message, so that I understand the command only works on a message the bot itself posted.
31. As a Telegram group member, I want `/status` on an Issue that's since been deleted or made inaccessible to fail with a clear chat message, so that I'm not left guessing why it didn't answer.

## Implementation Decisions

- **Domain module (`internal/domain`)**: `Ticket`, `Draft`, `Priority`, `IssueType`, `Assignee`. Pure types, no imports beyond the standard library, no knowledge of Telegram/Gemini/Jira.
- **Use case module (`internal/usecase`)**: `CreateTicketFromMessage` is the single end-to-end flow — resolve source message + reply-chain context, call the drafter, resolve the assignee, call the tracker, return a result for the adapter to render back to Telegram. Imports only `domain` and `ports`.
- **Ports (`internal/ports`)**: `TicketDrafter` (produces a `Draft` from message context — implemented by Gemini), `IssueTracker` (creates an Issue from a `Draft` + resolved `Assignee`, and separately fetches an Issue's current assignee + workflow status by key — implemented by Jira, also owns the `createmeta` validation call), `AssigneeResolver` (resolves a Telegram handle to a Jira `accountId` or reports unresolved — implemented by the static config file). Interfaces named for what the use case needs, not the vendor.
- **Use case module addition**: `CheckTicketStatus` — a second use case alongside `CreateTicketFromMessage`. Takes an Issue key, calls `IssueTracker` to fetch current assignee + status, returns a result for the adapter to render back to Telegram. Imports only `domain` and `ports`, same as `CreateTicketFromMessage`.
- **Adapters (`internal/adapters`)**: one package per external system — `telegram/` (inbound webhook handler, parses `/to_ticket` and `/status`, extracts the Issue key from the bot's own confirmation-message format, renders replies), `gemini/` (implements `TicketDrafter`, uses `google.golang.org/genai` with `responseSchema`/`responseMimeType: application/json`, model `gemini-2.5-flash`), `jira/` (implements `IssueTracker`, hand-rolled `net/http` client against `POST /rest/api/3/issue`, builds ADF description, calls `GET /rest/api/3/issue/createmeta` at startup), `config/` (implements `AssigneeResolver`, loads `configs/assignees.yaml` and secrets). Adapters never import each other; vendor SDK types (`genai.*`, `tgbotapi.*`) never leak outside their adapter package.
- **Platform (`internal/platform`)**: `logging` (structured JSON to stdout, picked up by Cloud Logging, every line carries Telegram message ID + chat ID + stage-of-failure), `httpserver` (Cloud Run entrypoint, health checks).
- **Wiring (`cmd/bot/main.go`)**: constructs concrete adapters, injects into use cases, starts the HTTP server. No business logic here.
- **Command parsing**: `/to_ticket` must be a reply to another message — the replied-to message is the source message. No reply → usage-hint response, not silent failure. Optional `@assignee` argument.
- **`/status` command**: must be sent as a reply to a message authored by the bot itself matching the "Created <KEY>: ..." confirmation format. The bot extracts the Issue key from that message's text (no separate storage — the key was already in the text it posted). Replying to any other message (someone else's, or a bot message that isn't a confirmation) → clear error reply, not a silent no-op or misparse. On a valid key, calls `CheckTicketStatus`, which calls `IssueTracker` (`GET /rest/api/3/issue/{key}`) and replies with current assignee (display name, or "Unassigned") and current workflow status name. An Issue that's deleted/inaccessible (404/403 from Jira) → clear chat error, not a crash or silent drop.
- **Sprint field**: every Issue created via `/to_ticket` has its Sprint field set to the fixed value "MA Sprint Board 4" — hardcoded, not configurable per-request. Resolved to the correct sprint field ID (a custom field, per standard Jira Cloud Software behavior) the same way `issuetype`/`priority` names are validated: checked at startup so a renamed/closed sprint fails fast rather than on first real ticket.
- **Context gathering**: include the source message plus its own reply chain if the source message is itself a reply — not a flat "last N messages" window. Each included message tagged with sender name and timestamp when passed to Gemini.
- **Assignee resolution**: empty `@assignee` → unassigned (no `assignee` key sent to Jira, never `null`). Present but unresolved → soft-fail: reply in chat that resolution failed, still create the Issue unassigned. Mapping lives in `configs/assignees.yaml`, hand-edited, takes effect on redeploy — no runtime mutation, no database.
- **Gemini contract**: structured output via `responseSchema` — `{title, description, issue_type,   labels}`, `issue_type` enum `Bug`/`Task`/`Story`, `priority` enum `Highest`/`High`/`Medium`/`Low`, `labels` a string array. System instruction directs Gemini to stay factual, not invent unreported details, and default to Medium priority when severity is unclear.
- **Jira issue creation**: `POST /rest/api/3/issue`, Basic Auth (`base64(email:api_token)`) for MVP. Description built as Atlassian Document Format, not a plain string, and notes it was reported via Telegram by the source message's author. `issuetype`/`priority` names validated against `GET /rest/api/3/issue/createmeta` at bot startup — startup fails fast on a bad config rather than failing on first real ticket.
- **Media handling**: an image on the source message is downloaded via Telegram `getFile` and re-uploaded as a Jira attachment. A video is never downloaded — only linked by URL in the Issue description text. Images elsewhere in the gathered context (not on the source message) are not touched.
- **Access control**: allowlist by Telegram chat ID only — commands from outside the allowlisted chat are ignored outright. No per-user allowlist inside the group.
- **Delivery mode**: Telegram webhook from day one (not long-polling), with Telegram's `secret_token` set on the webhook and verified on every incoming request.
- **Hosting**: Google Cloud Run — HTTPS out of the box for the webhook, scales to zero.
- **Secrets**: `TELEGRAM_BOT_TOKEN`, `GEMINI_API_KEY`, `JIRA_API_TOKEN`, webhook `secret_token` all in Google Secret Manager via Cloud Run's native integration — never plain env vars, never source.
- **Scope**: single Jira project, no per-topic/per-channel routing.
- **UX**: fire-and-forget — the Issue is created immediately on command, no confirm/edit step. The chat reply includes the Jira link and, implicitly, that the reporter should edit directly in Jira if Gemini got something wrong.
- **Error surfacing**: a Gemini failure/timeout produces a clear chat reply, not a silently dropped command. A Jira failure surfaces Jira's own `errorMessages` into the chat. Every attempt (success or failure) is logged with the Telegram message ID, chat ID, and stage-of-failure.

## Testing Decisions

- A good test here exercises observable behavior through `CreateTicketFromMessage` — given a source message (plus reply-chain context) and fake port responses, assert on the resulting Issue-creation call and/or chat-reply content. Never assert on internal call sequencing or adapter-internal state.
- **Primary seam, and the only one used for business-logic tests**: `internal/usecase.CreateTicketFromMessage` and `internal/usecase.CheckTicketStatus`, both tested against in-memory fakes of `TicketDrafter`, `IssueTracker`, and `AssigneeResolver`. This covers: assignee resolution (empty/resolved/unresolved), soft-fail chat messaging, Gemini-failure and Jira-failure error paths, reply-chain context assembly, the fixed Sprint field being set on every creation, and `/status` lookups (found, not-found/inaccessible) — all without any live network call.
- The `telegram/` adapter's own-confirmation-message key extraction (used by `/status`) is a plain string-parsing function — test it directly as a unit, no fake needed, since it has no port dependency of its own.
- **Adapter tests are secondary and thin**: `gemini/` against a recorded structured-output fixture (confirms the schema/config still round-trips into a `Draft`); `jira/` against a recorded `createmeta` fixture and the ADF-building logic (confirms the payload shape still matches what Jira v3 expects). These are contract tests, not full integration coverage — no live Gemini/Jira calls in CI.
- Prefer testing through the use case layer over testing an adapter in isolation whenever both are possible, per ADR-0002 — the use case is the actual contract that matters.
- No existing test suite in this repo yet (fresh project) — these seams establish the pattern going forward; the `mattpocock-skills:tdd` skill's red-green-refactor flow applies per ticket.

## Out of Scope

- Confirm/edit step before Jira creation (deferred nice-to-have per `general-plan.md` §10).
- Duplicate-ticket detection via Jira JQL search (deferred nice-to-have per `general-plan.md` §10).
- Multi-project or per-topic Jira routing.
- Per-user allowlist inside the group (only chat-level allowlist for now).
- Assignee Mapping backed by a database or admin Telegram command for runtime edits — file-based, redeploy-to-change only.
- Durable, queryable ticket-creation history beyond Cloud Logging's retention window.
- OAuth 2.0 (3LO) for Jira — Basic Auth via API token is MVP scope; OAuth is a later concern if this becomes a shared/production app.
- Long-polling delivery mode — webhook only, per ADR-0001.
- Telegram Serverless as a hosting target — rejected in ADR-0001 (JS-only, undocumented secrets/limits story).

## Further Notes

- All terminology above (Ticket Request, source message, Draft, Issue, Assignee Mapping, Attachment) follows `CONTEXT.md` — use these exact terms in ticket titles and code, not synonyms.
- Decisions here trace to ADR-0001 (stack/hosting: Go, Cloud Run, webhooks, no database, media handling, access control, UX, assignee fallback, Gemini model) and ADR-0002 (clean/hexagonal layering and its testing approach) — both already Accepted. Any ticket that would contradict either ADR should flag that explicitly rather than silently drifting from it.
- `general-plan.md` remains the original research reference (endpoint shapes, SDK links, starter-template survey) but its open questions (§13) are now resolved by the two ADRs — treat the ADRs as authoritative where they overlap.
