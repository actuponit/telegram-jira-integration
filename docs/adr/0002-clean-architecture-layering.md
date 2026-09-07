# ADR-0002: Clean/hexagonal layering for the Go codebase

## Status

Accepted (2026-09-07).

## Context

User asked for "clean architecture and a professional codebase." The bot
touches three external systems (Telegram, Gemini, Jira) plus a config file
and a logger — exactly the shape hexagonal architecture (ports & adapters)
is for: keep the business rule ("turn a source message into a Jira Issue")
independent of which vendor SDK implements each side of it, and testable
without live network calls.

## Decision

Four layers, dependency rule strictly inward (outer layers import inner
ones, never the reverse):

1. **`internal/domain`** — `Ticket`, `Draft`, `Priority`, `IssueType`,
   `Assignee`. Plain Go types and pure functions. No imports outside the
   standard library. No knowledge that Telegram, Gemini, or Jira exist.
2. **`internal/usecase`** — one file per end-to-end flow (e.g.
   `CreateTicketFromMessage`). Imports `domain` and `ports` only. Contains
   the actual orchestration logic: call the drafter, resolve the assignee,
   call the tracker, return a result. This is what gets unit tested.
3. **`internal/ports`** — interfaces the use case layer needs, named for the
   need not the vendor: `TicketDrafter`, `IssueTracker`, `AssigneeResolver`.
   Defined here, implemented in `adapters`.
4. **`internal/adapters`** — one package per external system
   (`telegram/`, `gemini/`, `jira/`, `config/`). Each adapter implements the
   port(s) relevant to it and is the only place vendor SDK types
   (`genai.*`, `tgbotapi.*`) are allowed to appear. Adapters never import
   each other.

`internal/platform` holds cross-cutting infra (`logging`, `httpserver`) that
isn't part of the business flow. `cmd/bot/main.go` is wiring only:
constructs concrete adapters, injects them into use cases, starts the HTTP
server. No business logic in `main.go`.

## Testing

- **Use cases** are tested against in-memory fakes of the ports — no live
  Telegram/Gemini/Jira calls in unit tests.
- **Adapters** get thin contract tests against recorded fixtures (e.g. a
  saved Jira `createmeta` response, a saved Gemini structured-output
  response) — enough to catch a vendor API shape change, not full
  integration coverage.
- Prefer testing through the use case layer over testing adapters in
  isolation wherever both are possible — the use case is the actual
  contract that matters.

## Consequences

- Adding a fourth integration (e.g. Slack instead of Telegram, or a second
  issue tracker) means writing one new adapter package and satisfying an
  existing port — the use case layer doesn't change.
- Slightly more ceremony than a flat `main.go` for a bot this size. Accepted
  because "professional codebase" was an explicit requirement and the
  project already has three real external dependencies to isolate.
- See the `go-clean-architecture` skill (`.claude/skills/`) for the rules
  agents must follow when adding code in this repo.
