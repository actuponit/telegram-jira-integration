# Deploying to Cloud Run

Implements ticket `05 — Cloud Run deploy + Secret Manager`. Hosting and
secrets decisions come from [ADR-0001](adr/0001-stack-and-hosting.md):
Cloud Run, webhook delivery, all credentials in Google Secret Manager.

## What lives where

| Value | Where it lives | Why |
| --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | Secret Manager | credential |
| `GEMINI_API_KEY` | Secret Manager | credential |
| `JIRA_API_TOKEN` | Secret Manager | credential |
| `TELEGRAM_WEBHOOK_SECRET` | Secret Manager | webhook `secret_token` |
| `JIRA_BASE_URL`, `JIRA_EMAIL`, `JIRA_PROJECT_KEY` | Cloud Run env var | config, not secret |
| `TELEGRAM_ALLOWED_CHATS` | Cloud Run env var | config, not secret (ticket 03) |
| `configs/assignees.yaml` | baked into the image | hand-edited, redeploy to change |

Cloud Run's native Secret Manager integration injects the four secrets as
env vars at runtime, so `cmd/bot` reads everything the same way it does
locally — no code path is Cloud Run specific. Startup fails fast if any of
them is missing or empty.

Secret Manager names are the env var name lowercased with dashes:
`TELEGRAM_BOT_TOKEN` → `telegram-bot-token`.

## First deploy

```bash
cp deploy/config.env.example deploy/config.env
$EDITOR deploy/config.env        # project, region, Jira coordinates, allowed chats

./deploy/00-bootstrap.sh   # APIs, Artifact Registry repo, runtime service account
./deploy/10-secrets.sh     # create secrets (prompts, values read from stdin), grant accessor
./deploy/20-deploy.sh      # Cloud Build image + gcloud run deploy
./deploy/30-set-webhook.sh # point Telegram at the deployed URL with secret_token
./deploy/40-verify.sh      # /healthz, getWebhookInfo, recent logs
```

`deploy/config.env` is gitignored and holds no credentials.

## Redeploying

Code or `configs/assignees.yaml` changed:

```bash
./deploy/20-deploy.sh
```

The webhook URL does not change between revisions, so `30-set-webhook.sh`
only needs re-running if the service is recreated or the bot token or
webhook secret is rotated.

## Rotating a secret

```bash
printf %s "$NEW_VALUE" | gcloud secrets versions add jira-api-token \
  --project "$PROJECT_ID" --data-file=-
./deploy/20-deploy.sh   # new revision picks up :latest
```

Rotating `TELEGRAM_BOT_TOKEN` or `TELEGRAM_WEBHOOK_SECRET` also needs
`./deploy/30-set-webhook.sh` afterwards.

## Service shape

- `--min-instances 0` — scales to zero, per ADR-0001.
- `--max-instances 4` — caps spend on a low-traffic ops bot.
- `--timeout 120` — covers a Gemini draft plus a Jira create.
- `--allow-unauthenticated` — Telegram calls the webhook anonymously.
  Authenticity is enforced in-process: every request must carry
  `X-Telegram-Bot-Api-Secret-Token` matching `TELEGRAM_WEBHOOK_SECRET`
  (see `internal/adapters/telegram/telegram.go`). The service is public but
  the webhook route is not usable without that token.
- Runs as a dedicated runtime service account holding only
  `roles/secretmanager.secretAccessor` on the four secrets.

## Verifying

`./deploy/40-verify.sh` covers the automated checks:

- `GET /healthz` returns 200 on the deployed URL.
- `getWebhookInfo` points at `<service-url>/webhook/telegram`.
- Recent Cloud Logging output for the service.

The last acceptance check is manual: send `/to-ticket` as a reply to a
message in the allowlisted chat and confirm a real Jira Issue is created and
its link comes back in chat. A command sent from a non-allowlisted chat must
be ignored outright.

## Troubleshooting

- **Revision fails to start** — `gcloud run services logs read <service>`.
  Startup validation errors (missing env var, bad Jira project key, empty
  allowlist) are logged as `startup failed` with the specific cause.
- **Telegram shows `last_error_message` in `getWebhookInfo`** — the service
  returned non-2xx. Check logs for that message ID.
- **Webhook returns 401** — `TELEGRAM_WEBHOOK_SECRET` in Secret Manager and
  the `secret_token` registered with Telegram have drifted apart. Re-run
  `./deploy/30-set-webhook.sh`.
- **Permission denied reading a secret** — the runtime service account lost
  `secretAccessor` on that secret; re-run `./deploy/10-secrets.sh` (it
  re-grants without changing values if you decline the new-version prompt).
