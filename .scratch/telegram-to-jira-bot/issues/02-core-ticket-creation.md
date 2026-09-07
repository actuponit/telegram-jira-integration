# 02 — Core ticket creation

**What to build:** a Telegram group member replies `/to-ticket [@assignee]` to a message and gets a real Jira Issue back, linked in the same thread. Covers the full `/to-ticket` behavior: webhook security, Draft generation via Gemini using the source message's full reply-chain context, assignee resolution (empty/resolved/unresolved), Issue creation on the fixed Sprint with image/video media handling, and clear chat-visible errors on any failure. This is the bot's core value end to end.

**Blocked by:** 01 — Project scaffold

**Status:** ready-for-agent

- [ ] Telegram webhook handler verifies the `secret_token` on every incoming request; unverified requests are rejected
- [ ] `/to-ticket` with no reply target gets a usage-hint response, not silent failure
- [ ] Source message plus its own reply chain (if it is itself a reply) is gathered, each message tagged with sender name and timestamp, and passed to Gemini — not a flat last-N-messages window
- [ ] `gemini` adapter implements `TicketDrafter` via `google.golang.org/genai`, model `gemini-2.5-flash`, `responseSchema`/`responseMimeType: application/json` returning `{title, description, issue_type, priority, labels}`; system instruction stays factual, defaults to Medium priority when severity is unclear, never invents unreported details
- [ ] Empty `@assignee` → Issue created unassigned (no `assignee` key sent to Jira, never `null`)
- [ ] Present but unresolved `@assignee` → chat reply explains resolution failed, Issue still created unassigned
- [ ] Resolved `@assignee` → Issue created with the resolved `accountId`
- [ ] `config` adapter implements `AssigneeResolver`, loading `configs/assignees.yaml`
- [ ] `jira` adapter implements `IssueTracker`, hand-rolled `net/http` against `POST /rest/api/3/issue`, Basic Auth, ADF-built description noting it was reported via Telegram and by whom
- [ ] Every created Issue has its Sprint field set to "MA Sprint Board 4", resolved to the correct custom field ID
- [ ] At bot startup, `issuetype`/`priority` names and the Sprint field are validated against `GET /rest/api/3/issue/createmeta`; startup fails fast on bad config
- [ ] Image on the source message is downloaded via Telegram `getFile` and re-uploaded as a Jira attachment; images elsewhere in gathered context are untouched
- [ ] Video on the source message is never downloaded — only linked by URL in the Issue description text
- [ ] On success, bot replies in the same thread with a message containing the Jira key and link (format `Created <KEY>: ...`, parseable later by `/status`)
- [ ] Gemini failure/timeout produces a clear chat reply, not a dropped command
- [ ] Jira failure surfaces Jira's own `errorMessages` into the chat
- [ ] Every attempt (success or failure) is logged with Telegram message ID, chat ID, and stage-of-failure
- [ ] `CreateTicketFromMessage` use case tested against in-memory fakes of all three ports — no live network calls in these tests
- [ ] `gemini` and `jira` adapters have thin contract tests against recorded fixtures (structured-output response, `createmeta` response)
