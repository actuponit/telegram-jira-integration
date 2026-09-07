# 01 — Project scaffold

**What to build:** the skeletal Go module and clean-architecture layout the rest of the bot builds into. No business logic — just types, port interfaces, empty adapter stubs, a logger, and an HTTP server that boots and answers a health check.

**Blocked by:** None — can start immediately.

**Status:** done

- [x] `go.mod` initialized, module builds and runs
- [x] `internal/domain` defines `Ticket`, `Draft`, `Priority`, `IssueType`, `Assignee` as pure types with no imports beyond the standard library
- [x] `internal/ports` defines `TicketDrafter`, `IssueTracker`, `AssigneeResolver` interfaces, named for what the use case needs (not vendor names)
- [x] `internal/adapters/{telegram,gemini,jira,config}` exist as empty packages (no vendor SDK wired yet)
- [x] `internal/platform/logging` emits structured JSON to stdout
- [x] `internal/platform/httpserver` starts an HTTP server with a health-check endpoint
- [x] `cmd/bot/main.go` wires the above and starts the server; contains no business logic
- [x] Layout matches `CONTEXT.md`'s "Where things live" section exactly
