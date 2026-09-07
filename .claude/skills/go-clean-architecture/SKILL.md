---
name: go-clean-architecture
description: Layering rules for this repo's Go codebase (domain / usecase / ports / adapters). Use whenever writing, reviewing, or placing new Go code in this project — deciding what package something belongs in, adding a new external integration, or reviewing a diff for layer violations.
---

# Go Clean Architecture (this repo)

Full rationale: `docs/adr/0002-clean-architecture-layering.md`. Domain
vocabulary: `CONTEXT.md`. This skill is the enforceable checklist version.

## The four layers, dependency rule strictly inward

```
cmd/bot            wiring only, no logic
  -> internal/usecase   orchestration ("what happens")
       -> internal/domain    pure types, zero external imports
       -> internal/ports     interfaces (Go's `interface`, defined here)
internal/adapters/*   implement ports, import domain, never import each other
internal/platform/*   cross-cutting infra (logging, http server) — not business logic
```

An inner layer never imports an outer one. `domain` never imports
`usecase`, `ports`, or `adapters`. `usecase` never imports `adapters`
directly — only the `ports` interface.

## Placement checklist — before writing a file, ask:

1. **Is this a business rule/type with no vendor knowledge?** →
   `internal/domain`. If you're tempted to import `genai`, `tgbotapi`, or an
   HTTP client here, stop — it belongs in an adapter instead.
2. **Is this "what happens" for one end-to-end flow?** →
   `internal/usecase`, one file per flow. It may only reference `domain`
   types and `ports` interfaces — never a concrete adapter type.
3. **Is this an interface the use case layer needs?** → `internal/ports`.
   Name it for the need (`TicketDrafter`), not the vendor (`GeminiClient`).
4. **Is this vendor-specific code (Telegram/Gemini/Jira/config-file
   parsing)?** → `internal/adapters/<vendor>`. This is the *only* place
   vendor SDK types are allowed to appear. It implements one or more
   `ports` interfaces.
5. **Is this cross-cutting infra with no business meaning** (logger setup,
   HTTP server bootstrap, health checks)? → `internal/platform`.
6. **Is this dependency injection?** → `cmd/bot/main.go` only. If you find
   yourself constructing a concrete adapter anywhere else, move it here.

## Hard rules (flag these in review)

- No vendor SDK import (`genai`, `tgbotapi`, Jira HTTP types) outside
  `internal/adapters/*`.
- No adapter package imports another adapter package. If `jira` needs
  something from `gemini`, that's a sign the shared thing belongs in
  `domain` or `usecase`.
- No business logic (branching on `Priority`, deciding when an Issue is
  unassigned, etc.) inside an adapter. Adapters translate; they don't
  decide.
- Use case functions take and return `domain` types plus primitives —
  never a `ports` implementation's request/response struct.
- New external integration = new port + new adapter package. Don't grow an
  existing adapter to do double duty for two vendors.

## Testing (see ADR-0002 for the fuller rationale)

- Use cases: unit-tested against in-memory fakes of the ports. No live
  network calls in `go test ./internal/usecase/...`.
- Adapters: thin contract tests against recorded fixtures (a saved Jira
  `createmeta` response, a saved Gemini structured-output response) — catch
  a vendor shape change, not full integration coverage.
- When both are possible, prefer testing through the use case over testing
  an adapter in isolation.

## When this skill applies vs. `codebase-design`

Use this skill for repo-specific placement questions (which package does
this go in). Use the `codebase-design` skill for the general vocabulary of
deep modules / seams when actually designing a new port's interface shape.
