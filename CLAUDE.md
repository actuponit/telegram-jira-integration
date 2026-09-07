## Agent skills

### Issue tracker

Issues tracked as local markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context layout — `CONTEXT.md` + `docs/adr/` at repo root. See `docs/agents/domain.md`.

### Architecture

Go clean/hexagonal architecture (domain / usecase / ports / adapters).
Rules: `.claude/skills/go-clean-architecture/SKILL.md`. Rationale:
`docs/adr/0002-clean-architecture-layering.md`. Stack/hosting decisions:
`docs/adr/0001-stack-and-hosting.md`. Consult the `go-clean-architecture`
skill before placing any new Go file, and the `codebase-design` skill when
shaping a new port's interface.

## Workflow

This project follows the mattpocock-skills pipeline end to end:

1. **Grill the design** — `/mattpocock-skills:grilling` (or `grill-me`) to
   turn a rough idea into a fully-decided design. Don't skip this for
   anything with real architectural surface area.
2. **Spec it** — `/mattpocock-skills:to-spec` synthesizes the grilled
   conversation into a spec and publishes it to `.scratch/<feature-slug>/spec.md`.
   These are user-invoked slash commands, not something an agent triggers
   on its own.
3. **Ticket it** — `/mattpocock-skills:to-tickets` breaks the approved spec
   into tracer-bullet tickets under `.scratch/<feature-slug>/issues/`, one
   file per ticket, blocking edges declared. See
   `docs/agents/issue-tracker.md` for the file conventions.
4. **Build** — work the ticket frontier (unblocked, unclaimed, lowest
   number first). Follow `mattpocock-skills:tdd` for red-green-refactor and
   the `go-clean-architecture` skill for where code goes. Use the domain
   glossary in `CONTEXT.md` throughout — don't invent synonyms for terms
   already defined there.
5. **Review** — `mattpocock-skills:code-review` (Standards + Spec axes)
   before considering a ticket done.
6. If a change contradicts an existing ADR, surface that explicitly instead
   of silently overriding it — see `docs/agents/domain.md`.
