# 06 — Config adapter & Assignee Resolver

**What to build:** the `config` adapter implements `ports.AssigneeResolver` by loading the Assignee Mapping from `configs/assignees.yaml`.

**Blocked by:** 01 — Project scaffold

**Status:** done

- [x] `configs/assignees.yaml` format defined: `telegram_username -> jira_account_id` (+ display name for chat messages)
- [x] `config` adapter implements `ports.AssigneeResolver`, loading the mapping once at startup (no runtime mutation, no database — see ADR-0001)
- [x] `Resolve` returns `(domain.Assignee{}, false)` for a handle not in the mapping, and the resolved `domain.Assignee{AccountID, DisplayName}` for a hit
- [x] Unit tests: hit, miss, empty handle, malformed/missing YAML file at load time (fails fast with a clear error, doesn't panic)

## Comments

Split out of ticket 02 (Core ticket creation) — see 02's tracking note.
