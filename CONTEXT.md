# Telegram → Jira Ticket Bot — Domain Glossary

Single-context repo. Read this before exploring or making changes; check
`docs/adr/` for decisions in the area you're touching.

## Core terms

- **Ticket Request** — a Telegram `/to_ticket [@assignee]` command, always sent
  as a reply to another message. The replied-to message is the **source
  message**.
- **Draft** — the structured `{title, description, issue_type, priority,
  labels}` object Gemini returns for a source message. A Draft is not yet a
  Jira issue.
- **Issue** — the Jira ticket created from a Draft. Identified by its Jira
  `key` (e.g. `PROJ-482`).
- **Assignee Mapping** — the `telegram_username -> jira_account_id` table
  used to resolve `@assignee`. Lives as a config file, not a database (see
  ADR-0001). An unresolved or omitted assignee is a soft-fail: the Issue is
  still created, unassigned.
- **Attachment** — an image on the source message, downloaded from Telegram
  and re-uploaded to the created Issue. Video is never downloaded — only
  linked in the Issue description. See ADR-0001.

## Layering vocabulary (see ADR-0002)

- **Domain** — pure business types (`Ticket`, `Draft`, `Priority`,
  `IssueType`) with no knowledge of Telegram, Jira, or Gemini.
- **Use case** — application service orchestrating one end-to-end flow (e.g.
  `CreateTicketFromMessage`). Depends only on domain types and **ports**.
- **Port** — an interface the use case layer depends on but does not
  implement: `TicketDrafter` (Gemini), `IssueTracker` (Jira),
  `AssigneeResolver` (mapping file). Named for what the use case needs, not
  for the vendor behind it.
- **Adapter** — a concrete implementation of a port, or the inbound Telegram
  webhook handler that turns an HTTP request into a use case call.

Use these terms in issue titles, ADRs, and code — don't drift to synonyms
(e.g. "handler" for what is actually a use case, or "client" for what is
actually a port).

## Where things live

```
cmd/bot/              wiring only — constructs adapters, injects into use cases
internal/domain/      Ticket, Draft, Priority, IssueType — zero external imports
internal/usecase/     CreateTicketFromMessage and friends
internal/ports/       TicketDrafter, IssueTracker, AssigneeResolver interfaces
internal/adapters/
  telegram/           inbound webhook handler
  gemini/              implements TicketDrafter
  jira/                 implements IssueTracker
  config/                implements AssigneeResolver, loads secrets
internal/platform/
  logging/             structured stdout logger (Cloud Logging picks it up)
  httpserver/           Cloud Run entrypoint, health checks
configs/assignees.yaml  the Assignee Mapping — hand-edited, redeploy to change
docs/adr/               architectural decisions
.scratch/               specs and tickets (mattpocock skills issue tracker)
```
