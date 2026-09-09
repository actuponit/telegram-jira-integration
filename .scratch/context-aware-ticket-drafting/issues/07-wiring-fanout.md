# 07 — Wiring & fan-out

**What to build:** `cmd/bot/main.go` wires both use cases and both adapter
methods together; the fan-out rules (assignee, attachment, per-Candidate
failure reporting) hold end to end.

**Blocked by:** 05 — Gemini adapter: DraftSet schema + App Context,
06 — Telegram adapter: clarification carrier + grouped reply

**Status:** open

## Scope

- `cmd/bot/main.go`: wire the Gemini adapter's App Context file path/content
  (ticket 02) in at construction, and wire both use cases (03 and 04) behind
  the Telegram handler's routing (06).
- `@assignee` applied to every Issue from the message — soft-fail to
  unassigned is unchanged from today.
- The source message's single image attached to every created Issue (main
  flow only — not the clarification path, per ticket 04).
- Failures reported per Candidate. No rollback, no duplicate protection —
  re-running the command on the same message creates a second set of Issues.
  The grouped reply (ticket 06) is the only safeguard against confusion here,
  which is why its clarity matters — don't add dedup logic, it's explicitly
  out of scope.
- Every stage of a multi-Issue run logged with the Telegram message ID, so a
  partial failure is diagnosable from Cloud Logging alone.
- `telegram-bot-api/v5` stays pinned at v5.5.1 — do not upgrade it. Nothing
  in this design needs Bot API 7.0.

## Done when

- `go build ./...` and `go test ./...` are green with both use cases wired
  in.
- A real (or fixture-driven) multi-Candidate Ticket Request produces the
  grouped reply, logs each stage with the message ID, and a partial failure
  is traceable from logs alone.
- No new datastore, no dedup logic, no rollback exists anywhere in the wired
  path.

## Notes
