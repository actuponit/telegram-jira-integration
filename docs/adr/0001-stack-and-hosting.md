# ADR-0001: Go on Cloud Run, webhook mode, no database

## Status

Accepted (2026-09-07, from a grilling session — see `general-plan.md` for the
original research).

## Context

`general-plan.md` proposed Go with either long-polling (MVP) or webhooks
(later). We decided to build webhooks from day one and looked at Telegram's
new **Telegram Serverless** offering (core.telegram.org/bots/serverless,
launched ~July 2026) as a possible host, since it removes webhook plumbing
entirely.

Research found Telegram Serverless is JavaScript-only (V8 sandbox), has no
Go support, and leaves secrets/execution-limit handling undocumented. Betting
the whole stack — and the plan's already-chosen Go SDKs
(`go-telegram-bot-api`, `google.golang.org/genai`) — on a month-old,
JS-only, under-documented platform was rejected.

## Decision

- **Language/stack:** Go, as in `general-plan.md` §3, unchanged.
- **Delivery mode:** webhooks from the start, not long-polling-then-migrate.
- **Hosting:** Google Cloud Run. Gives HTTPS out of the box (required for
  Telegram webhooks), scales to zero, and keeps the Go stack intact. GCP
  project and billing already exist for this work.
- **Secrets:** Google Secret Manager via Cloud Run's native integration —
  not plain environment variables. Applies to `TELEGRAM_BOT_TOKEN`,
  `GEMINI_API_KEY`, `JIRA_API_TOKEN`, and the Telegram webhook `secret_token`.
- **No database.** Two things that might have wanted one were each pushed to
  a simpler option instead:
  - **Assignee Mapping:** a plain YAML/JSON file in the repo
    (`configs/assignees.yaml`), edited by hand, redeploy to take effect. Not
    Firestore — rejected as more infrastructure than this stage needs.
  - **Audit log** (plan §9 — log every ticket-creation attempt): structured
    `log.Printf`/JSON to stdout, captured for free by Cloud Logging. Every
    log line carries the Telegram message ID, chat ID, and
    stage-of-failure, so a failed run is diagnosable from Cloud Logging
    alone without a bespoke store.
- **Media:** only the image on the *replied-to* message is downloaded (via
  Telegram `getFile`) and re-uploaded as a Jira attachment. Video is never
  downloaded — only linked in the Issue description text. Images elsewhere
  in the context window are not touched.
- **Access control:** allowlist by Telegram chat ID only. No per-user
  allowlist inside the group — anyone in the allowlisted chat can run
  `/to-ticket`.
- **Jira scope:** single project, no per-topic routing.
- **UX:** fire-and-forget. The bot creates the Issue immediately and replies
  with the link; there is no confirm/edit step before creation. The reply
  tells the reporter to edit the Issue directly in Jira if Gemini got
  something wrong.
- **Assignee fallback:** an unresolved or omitted `@assignee` always creates
  an unassigned Issue plus a soft-fail chat message — never a default
  fallback owner.
- **Gemini model:** `gemini-2.5-flash` — this is a background ops tool, not
  worth preview-tier instability for marginal quality gains.

## Consequences

- Every mapping change requires a rebuild + redeploy. Acceptable at current
  team size; revisit (e.g. move to a small KV store with an admin Telegram
  command) if edit frequency or team size grows.
- No durable, queryable ticket-creation history beyond Cloud Logging's
  retention window. Acceptable for now; add a real store only if reporting
  on ticket history becomes a real need.
- Revisit Telegram Serverless once/if it adds non-JS runtimes or documents
  its secrets and limits story — nothing here is permanent, it's what fits
  today's constraints.
