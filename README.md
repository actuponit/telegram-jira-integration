# telegram-jira-integration

A Telegram bot that turns a chat message into a Jira Issue. Reply
`/to_ticket` to any message in an allowlisted group and the bot gathers that
message plus its reply chain, drafts an Issue with Gemini, creates it in
Jira, and replies with the link. Reply `/status` to the bot's own
confirmation message to get that Issue's current assignee and workflow
status back.

Runs on Google Cloud Run, webhook delivery, no database. Design decisions
live in [`docs/adr/`](docs/adr/); domain vocabulary in
[`CONTEXT.md`](CONTEXT.md).

---

## 1. Every value you need, and where to get it

Nine values total. Four are secrets and go in Google Secret Manager; four are
plain config and go in `deploy/config.env`; one is a file in the repo.

### 1.1 Secrets (Google Secret Manager)

These are **never** typed into a file in this repo. `deploy/10-secrets.sh`
prompts for each one and pipes it straight into Secret Manager.

#### `TELEGRAM_BOT_TOKEN`

The bot's API token, from Telegram's own bot-creation bot.

1. Open Telegram, message [@BotFather](https://t.me/BotFather).
2. Send `/newbot` (or `/mybots` → pick an existing bot → *API Token*).
3. Answer the name and username prompts.
4. BotFather replies with a token shaped like
   `8123456789:AAF3xK9mQ2pLwEr7TnYvBcXz1QaSdF4gHjK`.

While you are in BotFather, also do this — the bot **cannot read group
messages without it**:

5. `/mybots` → your bot → *Bot Settings* → *Group Privacy* → **Turn off**.
   With privacy mode on, Telegram only delivers messages that explicitly
   `@mention` the bot, so `/to_ticket` as a reply will never arrive.

Secret Manager name: `telegram-bot-token`

#### `GEMINI_API_KEY`

Used to draft the Issue title, description, type, priority, and labels.
Model is `gemini-2.5-flash` (hardcoded, per ADR-0001).

1. Go to [aistudio.google.com/apikey](https://aistudio.google.com/apikey).
2. *Create API key* → pick a Google Cloud project (the same one you deploy
   to is fine).
3. Copy the key — it starts with `AIza`.

Secret Manager name: `gemini-api-key`

#### `JIRA_API_TOKEN`

An Atlassian API token for the account the bot acts as. Paired with
`JIRA_EMAIL` as HTTP Basic auth.

1. Go to
   [id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens).
2. *Create API token*, give it a label like `telegram-jira-bot`.
3. Copy it immediately — Atlassian shows it once.

The account you create it under needs permission to create Issues in the
target project, and read access to that project's `createmeta`.

Secret Manager name: `jira-api-token`

#### `TELEGRAM_WEBHOOK_SECRET`

A shared secret you invent. Telegram sends it back in the
`X-Telegram-Bot-Api-Secret-Token` header on every webhook call; the bot
rejects anything that doesn't match with a 401. It is what makes a publicly
reachable webhook URL safe.

Generate one:

```bash
openssl rand -hex 32
```

Telegram only accepts `A-Z a-z 0-9 _ -`, 1–256 characters — the hex output
above satisfies that. You never type this value anywhere except the
`10-secrets.sh` prompt; `30-set-webhook.sh` reads it back out of Secret
Manager when registering the webhook.

Secret Manager name: `telegram-webhook-secret`

### 1.2 Plain config (`deploy/config.env`)

Not credentials, so they are ordinary Cloud Run env vars. Copy the template
first:

```bash
cp deploy/config.env.example deploy/config.env
```

`deploy/config.env` is gitignored.

#### `JIRA_BASE_URL`

Your Jira Cloud site root, no trailing path. Look at any Jira browser tab:
`https://acme.atlassian.net/jira/software/projects/...` → use
`https://acme.atlassian.net`.

#### `JIRA_EMAIL`

The email address of the Atlassian account whose API token you created
above. Must be that exact account — Basic auth is
`base64(JIRA_EMAIL:JIRA_API_TOKEN)`.

#### `JIRA_PROJECT_KEY`

The short uppercase key of the target project — the prefix on every Issue
key. `MA-142` → the key is `MA`. Also visible in *Project settings →
Details*, or in the project list.

Single project only; there is no per-topic routing (ADR-0001).

#### `TELEGRAM_ALLOWED_CHATS`

Comma-separated Telegram chat IDs allowed to use the bot. Commands from any
other chat are ignored outright — no reply, no side effects. **Startup fails
if this is empty**, deliberately: a typo should stop the bot, not silently
make it answer nobody.

Group chat IDs are negative and usually start `-100`, e.g.
`-1001234567890`.

To find yours:

1. Add the bot to the group.
2. Send any message in the group.
3. Run, with your bot token:

   ```bash
   curl -s "https://api.telegram.org/bot<TELEGRAM_BOT_TOKEN>/getUpdates" \
     | python3 -m json.tool
   ```

4. Read `result[].message.chat.id`.

If `getUpdates` returns `409 Conflict`, a webhook is already registered —
`getUpdates` and webhooks are mutually exclusive. Either find the ID before
running `30-set-webhook.sh`, or temporarily
`curl -s "https://api.telegram.org/bot<TOKEN>/deleteWebhook"`, read the ID,
then re-run `deploy/30-set-webhook.sh`.

Multiple chats: `TELEGRAM_ALLOWED_CHATS=-1001234567890,-1009876543210`.

#### GCP coordinates (same file)

| Variable | What it is | How to pick it |
| --- | --- | --- |
| `PROJECT_ID` | GCP project the bot deploys into | `gcloud projects list` |
| `REGION` | Cloud Run region | Anything close to your team, e.g. `europe-west1`, `us-central1` |
| `SERVICE_NAME` | Cloud Run service name | Leave as `telegram-jira-bot` unless it clashes |
| `AR_REPO` | Artifact Registry repo for images | Leave as `telegram-jira-bot`; created by `00-bootstrap.sh` |
| `RUNTIME_SA_NAME` | Runtime service account | Leave as default; created by `00-bootstrap.sh` |

### 1.3 The Assignee Mapping file

[`configs/assignees.yaml`](configs/assignees.yaml) maps a Telegram handle to
a Jira account. It is baked into the container image — hand-edited, redeploy
to take effect (ADR-0001). An unmapped or omitted `@assignee` creates an
unassigned Issue plus a soft-fail chat note; it never falls back to a
default owner.

```yaml
alice_tg:
  jira_account_id: "5f8a1b2c3d4e5f6a7b8c9d0e"
  display_name: "Alice Kebede"
```

- **Key** — the Telegram username without `@`. Matching is
  case-insensitive and tolerates a leading `@`.
- **`jira_account_id`** — the Atlassian account ID, *not* an email or
  username. Two ways to find it:
  - Open the person's Jira profile; the URL ends in
    `/jira/people/<accountId>`.
  - Or query the API:

    ```bash
    curl -s -u "$JIRA_EMAIL:$JIRA_API_TOKEN" \
      "$JIRA_BASE_URL/rest/api/3/user/search?query=alice@example.com" \
      | python3 -m json.tool
    ```

    Read `accountId` from the result.
- **`display_name`** — used in chat replies only; cosmetic.

### 1.4 Full variable reference

| Variable | Kind | Lives in | Source |
| --- | --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | secret | Secret Manager `telegram-bot-token` | @BotFather |
| `GEMINI_API_KEY` | secret | Secret Manager `gemini-api-key` | Google AI Studio |
| `JIRA_API_TOKEN` | secret | Secret Manager `jira-api-token` | Atlassian account settings |
| `TELEGRAM_WEBHOOK_SECRET` | secret | Secret Manager `telegram-webhook-secret` | `openssl rand -hex 32` |
| `JIRA_BASE_URL` | config | Cloud Run env var | your Jira site URL |
| `JIRA_EMAIL` | config | Cloud Run env var | the API token's account |
| `JIRA_PROJECT_KEY` | config | Cloud Run env var | Jira project settings |
| `TELEGRAM_ALLOWED_CHATS` | config | Cloud Run env var | `getUpdates` |
| `ASSIGNEE_MAPPING_PATH` | config | set in the Dockerfile | defaults to `configs/assignees.yaml` |
| `PORT` | config | set by Cloud Run | defaults to `8080` |

Secret Manager names are just the env var name lowercased with dashes.
Cloud Run's native Secret Manager integration injects the four secrets as
env vars at runtime, so the bot reads all nine values identically whether
it's running locally or deployed — there is no Cloud Run–specific code path.

---

## 2. Prerequisites

| Tool | Why | Check |
| --- | --- | --- |
| Go 1.24+ | build and test | `go version` |
| `gcloud` CLI | everything in `deploy/` | `gcloud version` |
| `curl` | webhook registration and checks | `curl --version` |
| Docker | optional — only for building the image locally | `docker info` |

You also need a GCP project with **billing enabled** (Cloud Build and
Artifact Registry require it) and the IAM rights to enable APIs, create
service accounts, and create secrets — `roles/owner` on the project is the
simple answer.

Authenticate once:

```bash
gcloud auth login
gcloud config set project <PROJECT_ID>
```

---

## 3. Run it locally (optional, before deploying)

Useful for checking your Jira and Gemini credentials before involving Cloud
Run at all. The bot needs a webhook it can receive on, so this only exercises
startup validation unless you tunnel a public URL to it.

```bash
export TELEGRAM_BOT_TOKEN='...'
export TELEGRAM_WEBHOOK_SECRET="$(openssl rand -hex 32)"
export TELEGRAM_ALLOWED_CHATS='-1001234567890'
export GEMINI_API_KEY='...'
export JIRA_BASE_URL='https://acme.atlassian.net'
export JIRA_EMAIL='you@example.com'
export JIRA_API_TOKEN='...'
export JIRA_PROJECT_KEY='MA'

go run ./cmd/bot
```

Startup validates everything up front and fails fast with a specific error:
missing env vars are reported all at once, then the Assignee Mapping is
parsed, then Jira `createmeta` is checked for your project key, issue types,
priorities, and the Sprint field. Success looks like:

```
{"level":"INFO","msg":"starting server","addr":":8080"}
```

Health check:

```bash
curl -i localhost:8080/health    # expect 200
```

Tests:

```bash
go test ./...
```

All tests run against in-memory fakes and recorded fixtures — no live
Telegram, Gemini, or Jira calls.

---

## 4. Deploy, step by step

Five scripts, run in order, from the repo root. All are idempotent — re-run
any of them safely. Full reference and troubleshooting:
[`docs/deploy.md`](docs/deploy.md).

### Step 0 — fill in config

```bash
cp deploy/config.env.example deploy/config.env
$EDITOR deploy/config.env
```

Fill in `PROJECT_ID`, `REGION`, the three Jira values, and
`TELEGRAM_ALLOWED_CHATS` from §1.2. Leave the rest at their defaults.

### Step 1 — bootstrap the project

```bash
./deploy/00-bootstrap.sh
```

Enables Cloud Run, Artifact Registry, Cloud Build, and Secret Manager APIs;
creates the Artifact Registry repo; creates the runtime service account.
One time per project. Takes a minute or two the first time — API enablement
is slow.

### Step 2 — load the secrets

```bash
./deploy/10-secrets.sh
```

Prompts for each of the four secrets in turn, hidden as you type. Values go
straight into Secret Manager via stdin — never into your shell history, a
file, or the process list. Also grants the runtime service account
`roles/secretmanager.secretAccessor` on each secret.

If a secret already has a value, the script asks before adding a new version;
answer `n` to keep the existing value and just re-apply the IAM binding.

### Step 3 — build and deploy

```bash
./deploy/20-deploy.sh
```

Builds the container with Cloud Build, pushes it to Artifact Registry
(tagged with the current git short SHA), and deploys the Cloud Run service:
scale to zero, max 4 instances, 120s timeout, running as the runtime service
account, with the four secrets wired via `--set-secrets` and the four config
values via `--set-env-vars`.

Prints the service URL at the end. First build takes a few minutes.

> `--allow-unauthenticated` is deliberate. Telegram calls the webhook
> anonymously, so the URL must be publicly reachable. Authenticity is
> enforced inside the app instead: every request must carry
> `X-Telegram-Bot-Api-Secret-Token` matching `TELEGRAM_WEBHOOK_SECRET`, or
> it gets a 401 before anything else happens.

### Step 4 — register the webhook

```bash
./deploy/30-set-webhook.sh
```

Points Telegram at `<service-url>/webhook/telegram` with the `secret_token`
set. Both the bot token and the webhook secret are read out of Secret
Manager — you type neither. Expect `{"ok":true,...}`.

### Step 5 — verify

```bash
./deploy/40-verify.sh
```

Asserts `/health` returns 200, confirms `getWebhookInfo` points at this
service, and prints the last 20 log lines.

Then the one check no script can do: in the allowlisted group, reply
`/to_ticket` to a real message and confirm a real Jira Issue is created and
its link comes back in chat.

```
/to_ticket @alice_tg
```

Then reply `/status` to the bot's own confirmation message and confirm it
reports the assignee and workflow status.

---

## 5. Day-two operations

### Redeploy after a code or mapping change

```bash
./deploy/20-deploy.sh
```

The webhook URL is stable across revisions, so step 4 does not need
re-running.

### Rotate a secret

```bash
printf %s "$NEW_VALUE" | gcloud secrets versions add jira-api-token \
  --project "$PROJECT_ID" --data-file=-
./deploy/20-deploy.sh
```

Rotating `TELEGRAM_BOT_TOKEN` or `TELEGRAM_WEBHOOK_SECRET` also needs
`./deploy/30-set-webhook.sh` afterwards.

### Change who can use the bot

Edit `TELEGRAM_ALLOWED_CHATS` in `deploy/config.env`, then
`./deploy/20-deploy.sh`.

### Read logs

```bash
gcloud run services logs read telegram-jira-bot \
  --region "$REGION" --project "$PROJECT_ID" --limit 50
```

Every line carries the Telegram message ID, chat ID, and stage of failure, so
a failed run is diagnosable from logs alone (ADR-0001 — there is no separate
audit store).

---

## 6. Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Revision won't start, logs say `startup failed: missing required env vars: ...` | a value never made it onto the service | Re-run `10-secrets.sh` / check `deploy/config.env`, then `20-deploy.sh` |
| `startup failed: validate jira config` | wrong project key, or the account lacks access, or a needed issue type / priority / the Sprint field is absent from that project | Check `JIRA_PROJECT_KEY` and the account's project permissions |
| BotFather shows `Commands: no commands yet`, no autocomplete in clients | the command menu was never published to Telegram | Deploy — startup calls `setMyCommands` (`Handler.RegisterCommands`); a failure is logged as `register telegram commands` and is non-fatal |
| Bot silent in the group | privacy mode still on, or the chat ID isn't in `TELEGRAM_ALLOWED_CHATS` | BotFather → *Group Privacy* → off; verify the chat ID with `getUpdates` |
| Webhook calls return 401 | Secret Manager's `telegram-webhook-secret` and the token registered with Telegram have drifted apart | `./deploy/30-set-webhook.sh` |
| `getWebhookInfo` shows a `last_error_message` | the service returned non-2xx for that update | Read logs for that message ID |
| `getUpdates` returns `409 Conflict` | a webhook is registered; the two modes are exclusive | `deleteWebhook`, read the ID, then re-run `30-set-webhook.sh` |
| `PERMISSION_DENIED` accessing a secret | the runtime service account lost `secretAccessor` | Re-run `10-secrets.sh` (decline the new-version prompt to keep values) |

---

## 7. Repository layout

```
cmd/bot/              wiring only — builds adapters, injects use cases, starts the server
internal/domain/      pure types (Ticket, Draft, Priority, Assignee, ChatAllowlist)
internal/usecase/     CreateTicketFromMessage, CheckTicketStatus
internal/ports/       TicketDrafter, IssueTracker, AssigneeResolver interfaces
internal/adapters/    telegram/ gemini/ jira/ config/ — one per external system
internal/platform/    httpserver (Cloud Run entrypoint, /health), logging (JSON to stdout)
configs/              assignees.yaml — the Assignee Mapping
deploy/               the five deploy scripts + config.env.example
docs/adr/             architecture decision records
docs/deploy.md        deploy runbook
CONTEXT.md            domain glossary — use these terms, don't invent synonyms
```

Layering rules: [`.claude/skills/go-clean-architecture/SKILL.md`](.claude/skills/go-clean-architecture/SKILL.md)
and [ADR-0002](docs/adr/0002-clean-architecture-layering.md).
