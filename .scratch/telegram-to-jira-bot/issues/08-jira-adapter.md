# 08 — Jira IssueTracker adapter

**What to build:** the `jira` adapter implements `ports.IssueTracker` against the Jira REST API v3, hand-rolled `net/http`.

**Blocked by:** 01 — Project scaffold

**Status:** ready-for-agent

- [ ] Implements `ports.IssueTracker.CreateIssue(ctx, domain.Draft, domain.Assignee, *ports.Attachment) (domain.Ticket, error)` via `POST /rest/api/3/issue`, Basic Auth (`base64(email:api_token)`)
- [ ] Description built as Atlassian Document Format (ADF), not a plain string
- [ ] Empty/zero-value `domain.Assignee` → no `assignee` key sent at all (never `null`)
- [ ] Resolved `domain.Assignee` → Issue created with `accountId`
- [ ] Non-nil `*ports.Attachment` → uploaded to the created Issue via Jira's attachment endpoint after creation
- [ ] Every Issue's Sprint field set to "MA Sprint Board 4" — hardcoded, resolved to the correct custom field ID (see startup validation below)
- [ ] Implements `ports.IssueTracker.GetIssueStatus(ctx, key) (domain.Ticket, error)` via `GET /rest/api/3/issue/{key}`, mapping 404/403 to a clear error (used by ticket 04, `/status`)
- [ ] Startup validation method/call: `GET /rest/api/3/issue/createmeta` — validates `issuetype`/`priority` names and resolves the Sprint custom field ID; returns an error that fails the bot's startup fast on bad config (wired in ticket 10)
- [ ] Jira's own `errorMessages` from a failed `POST` are captured into the returned `error` (not swallowed into a generic message) — ticket 09 surfaces this into chat
- [ ] No business logic here (e.g. deciding when to omit the assignee is already decided by the usecase — see `internal/usecase/create_ticket.go`; this adapter just translates)
- [ ] Contract tests (via `httptest.Server`, no live Jira call): a recorded `createmeta` fixture drives the startup-validation test; an ADF-building unit test confirms the payload shape Jira v3 expects; an assignee-omission test confirms no `assignee` key is present when unassigned

## Comments

Split out of ticket 02 (Core ticket creation) — see 02's tracking note. `ports.IssueTracker.CreateIssue`'s attachment parameter was added while building the usecase layer (internal/usecase/create_ticket.go) to keep `internal/usecase` the seam for attachment pass-through logic.
