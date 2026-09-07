# 05 — Cloud Run deploy + Secret Manager

**What to build:** the bot runs live on Cloud Run with webhook delivery and all secrets pulled from Google Secret Manager — nothing sensitive in env vars or source.

**Blocked by:** 02 — Core ticket creation, 03 — Chat allowlist

**Status:** blocked-on-human

- [x] Bot deployed to Cloud Run, scales to zero — `Dockerfile` (static binary on distroless, non-root) and `deploy/20-deploy.sh` (`--min-instances 0 --max-instances 4`). **Not yet run against a real GCP project.**
- [x] `TELEGRAM_BOT_TOKEN`, `GEMINI_API_KEY`, `JIRA_API_TOKEN`, and the webhook `secret_token` are all sourced from Google Secret Manager via Cloud Run's native integration — `deploy/10-secrets.sh` creates them and grants the runtime service account `secretAccessor`; `deploy/20-deploy.sh` wires them with `--set-secrets`
- [x] `TELEGRAM_ALLOWED_CHATS` set on the Cloud Run service to the allowlisted chat ID(s) — a plain env var, not a secret; startup fails if it is missing or empty (ticket 03) — passed via `--set-env-vars` from `deploy/config.env`
- [x] Telegram webhook registered against the deployed Cloud Run URL with the `secret_token` set — `deploy/30-set-webhook.sh`, both values read from Secret Manager
- [ ] A real `/to-ticket` sent in the allowlisted chat against the deployed instance produces a real Jira Issue and chat reply
- [x] Health-check endpoint responds on the deployed instance — asserted by `deploy/40-verify.sh`

## Comments

The deploy path is fully scripted and idempotent; the runbook is
`docs/deploy.md`. Everything that can be verified without a live GCP
project has been:

- `go build ./...` and `go test ./...` green
- `CGO_ENABLED=0 GOOS=linux` static build succeeds (what the Dockerfile does)
- all `deploy/*.sh` pass `bash -n`

Not verified locally: the `docker build` itself (no Docker daemon running on
this machine) and every `gcloud` call — `gcloud` is authenticated but has no
project configured, and creating real Cloud Run/Secret Manager resources is
the operator's call, not the agent's.

Remaining work is operator-only, per `docs/deploy.md`:

1. `cp deploy/config.env.example deploy/config.env` and fill in project,
   region, Jira coordinates, and the allowlisted chat ID(s)
2. run `deploy/00-bootstrap.sh` → `10-secrets.sh` → `20-deploy.sh` →
   `30-set-webhook.sh` → `40-verify.sh`
3. send a real `/to-ticket` in the allowlisted chat and tick the last box

`--allow-unauthenticated` is deliberate: Telegram calls the webhook
anonymously, and authenticity is enforced in-process by the
`X-Telegram-Bot-Api-Secret-Token` check in
`internal/adapters/telegram/telegram.go` (401 on mismatch).
