# 10 — Wiring & startup validation

**What to build:** `cmd/bot/main.go` constructs the concrete adapters (06–09) and wires them into `usecase.CreateTicketFromMessage`, the httpserver, and the Jira `createmeta` startup check. No business logic here — wiring only.

**Blocked by:** 06 — Config adapter & Assignee Resolver, 07 — Gemini adapter, 08 — Jira adapter, 09 — Telegram webhook adapter

**Status:** blocked

- [ ] `cmd/bot/main.go` constructs `config`, `gemini`, `jira`, `telegram` adapters and injects them into `usecase.CreateTicketFromMessage` — no concrete adapter type referenced outside this file
- [ ] Telegram webhook route registered on `internal/platform/httpserver`, alongside the existing health check
- [ ] At startup, calls the jira adapter's `createmeta` validation (issuetype/priority names, Sprint field ID resolution); process exits non-zero with a clear log line on failure — fails fast on bad config
- [ ] Secrets (`TELEGRAM_BOT_TOKEN`, `GEMINI_API_KEY`, `JIRA_API_TOKEN`, webhook `secret_token`) read via env vars for local dev; ticket 05 (Cloud Run deploy) wires these to Secret Manager in production
- [ ] Smoke-tested locally: server boots, health check responds, startup validation actually runs (can be verified with a fake/mock Jira `createmeta` response if no live Jira credentials are available)

## Comments

Split out of ticket 02 (Core ticket creation) — see 02's tracking note.
