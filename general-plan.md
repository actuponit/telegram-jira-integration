# Telegram → Jira Ticket Bot (with Gemini) — Architecture & Build Plan

## 1. What we're building

A Telegram bot that lives in your work group. When a discussion resolves into
"this should be a ticket," someone **replies to the relevant message** with:

```
/to_ticket [@assignee]
```

The bot then:
1. Pulls the replied-to message (and optionally recent surrounding context).
2. Sends that text to **Gemini** with a strict JSON schema, asking it to produce
   a clean ticket: title, description, priority, labels, issue type.
3. Resolves the assignee (Telegram handle → Jira `accountId`), or leaves it
   unassigned if omitted or unresolved.
4. Creates the issue via the **Jira REST API v3**.
5. Replies in the Telegram thread with a link to the new ticket.

---

## 2. Integration components

| Component | Role | Key facts (current as of the research) |
|---|---|---|
| Telegram Bot API | Command handling, message context, replies | Use long polling for MVP, webhooks for production. `go-telegram-bot-api/telegram-bot-api/v5` is the most battle-tested Go client — thin wrapper, no magic, good for a bot this size. |
| Gemini API | Text → structured ticket JSON | Go SDK is `google.golang.org/genai` (the older `google/generative-ai-go` is deprecated — don't use it). Use `gemini-2.5-flash` for cost/latency, or a `gemini-3-flash-*` preview if you want higher quality and don't mind preview status. Use **structured output** (`responseSchema`) so Gemini returns validated JSON, not prose you have to regex out. |
| Jira REST API v3 | Ticket creation | `POST /rest/api/3/issue`. Descriptions must be in **ADF** (Atlassian Document Format), not plain strings — this trips up almost everyone on first integration. Auth via API token (Basic Auth: `email:token`, base64) for MVP; OAuth 2.0 (3LO) if this becomes a shared/production app later. |

---

## 3. Recommended stack

- **Language:** Go (as requested)
- **Telegram:** `github.com/go-telegram-bot-api/telegram-bot-api/v5`
- **Gemini:** `google.golang.org/genai`
- **Jira:** plain `net/http` + a small typed client (Jira's Go libraries, e.g. `andygrunwald/go-jira`, lag behind v3/ADF support — a ~150-line hand-rolled client is more reliable and easier to debug than fighting a wrapper's abstraction)
- **Config:** env vars via `envconfig` or plain `os.Getenv`, loaded from a `.env` in dev
- **Deployment:** single static binary in a Docker container; long-poll mode needs no public endpoint, so this can run anywhere (a small VM, a container in your existing cluster, even a Raspberry Pi on the office network)
- **Storage:** none required for MVP (stateless). Add SQLite later only if you want a Telegram-user → Jira-accountId mapping cache or an audit log of created tickets.

---

## 4. Message flow

```
User A: "the export button crashes on Safari when the file is >50MB"
User B: (replies to A's message) "/to_ticket @userC"

Bot:
  1. Detects command, extracts replied-to message text + author + timestamp
  2. Optionally pulls last N messages in the chat for context (or related messages may be if that message is a reply to a previous messagee we should include it as well)
  3. Calls Gemini with schema-constrained prompt
  4. Gemini returns: { title, description, issue_type, priority, labels }
  5. Bot resolves "@userC" -> Jira accountId (from a config map, or Jira user search)
  6. Bot POSTs to Jira: /rest/api/3/issue
  7. Bot replies: "✅ Created PROJ-482: <title> — https://yourco.atlassian.net/browse/PROJ-482"
```

### Command parsing
- `/to_ticket` with no argument → ticket created, `assignee: null` (unassigned in Jira).
- `/to_ticket @someone` → look up `someone` in a maintained mapping table (Telegram
  username → Jira accountId). This mapping **cannot** be derived automatically —
  Telegram usernames and Jira accounts aren't linked anywhere — so you'll maintain
  a small static config (a YAML/JSON file or env var) of `telegram_username: jira_account_id`.
- If the command isn't a reply to a message, respond with a usage hint instead of failing silently.

---

## 5. Gemini: prompt & structured output

Use `responseSchema` (previously "JSON mode") so the model is constrained to a schema, not just asked nicely.

**Go SDK call shape:**
```go
client, _ := genai.NewClient(ctx, &genai.ClientConfig{APIKey: os.Getenv("GEMINI_API_KEY")})

schema := &genai.Schema{
    Type: genai.TypeObject,
    Properties: map[string]*genai.Schema{
        "title":       {Type: genai.TypeString},
        "description": {Type: genai.TypeString},
        "issue_type":  {Type: genai.TypeString, Enum: []string{"Bug", "Task", "Story"}},
        "priority":    {Type: genai.TypeString, Enum: []string{"Highest", "High", "Medium", "Low"}},
        "labels":      {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
    },
    Required: []string{"title", "description", "issue_type", "priority"},
}

result, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(prompt),
    &genai.GenerateContentConfig{
        ResponseMIMEType: "application/json",
        ResponseSchema:   schema,
    })
```

**Prompt design:**
- System instruction: "You convert informal chat discussion into a well-formed
  engineering ticket. Be concise, factual, and don't invent details not present
  in the conversation. If severity/impact is unclear, default to Medium priority."
- User content: the replied-to message + surrounding context, each line tagged
  with sender name and timestamp, so Gemini can distinguish "what happened" from
  "who's discussing it."
- Keep context to the last ~10–15 messages max — enough for grounding, not so much
  that unrelated chat leaks into the ticket.

---

## 6. Jira: issue creation

**Endpoint:** `POST https://yourco.atlassian.net/rest/api/3/issue`
**Auth:** `Authorization: Basic base64(email:api_token)`

**Payload shape (description in ADF, not a plain string):**
```json
{
  "fields": {
    "project": { "key": "PROJ" },
    "issuetype": { "name": "Bug" },
    "summary": "Export button crashes on Safari for files >50MB",
    "description": {
      "type": "doc",
      "version": 1,
      "content": [
        {
          "type": "paragraph",
          "content": [
            { "type": "text", "text": "Reported in Telegram by @userA. Safari crashes when exporting files over 50MB..." }
          ]
        }
      ]
    },
    "priority": { "name": "High" },
    "labels": ["telegram-reported", "export"],
    "assignee": { "id": "<jira-account-id or omit if null>" }
  }
}
```

Notes:
- Omit the `assignee` key entirely for unassigned tickets — don't send `null`, Jira can be picky.
- Look up valid `issuetype` and `priority` names/IDs per-project ahead of time via
  `GET /rest/api/3/issue/createmeta` — these aren't universal across Jira instances.
- Response gives you `key` (e.g. `PROJ-482`) and `self` — build the browse URL as
  `https://yourco.atlassian.net/browse/PROJ-482`.

---

## 7. Assignee resolution logic

```
input: "@userC" or empty

if empty:
    return nil  // unassigned
else:
    lookup in static mapping (config file: telegram_username -> jira_account_id)
    if found: return account_id
    else: reply "couldn't resolve @userC to a Jira user — created unassigned" and proceed with nil
```

Keep this a soft failure — never block ticket creation because an assignee didn't resolve.

---

## 8. Security & access control

- **Restrict the bot to your specific group**: check `update.Message.Chat.ID` against
  an allowlisted chat ID; ignore commands from anywhere else.
- **Don't trust `/to_ticket` from just anyone in the group** if that matters to you —
  optionally restrict to a list of Telegram user IDs allowed to file tickets.
- Store `TELEGRAM_BOT_TOKEN`, `GEMINI_API_KEY`, `JIRA_API_TOKEN` as secrets
  (env vars / secret manager), never in source.
- If you move to webhook mode later, set Telegram's `secret_token` on the webhook
  and verify it on incoming requests.
- Rate-limit Gemini/Jira calls per chat to avoid accidental spam (e.g. one
  ticket creation in flight at a time per chat).

---

## 9. Error handling

- Gemini call fails / times out → reply with a clear error, don't silently drop the command.
- Jira call fails (bad project key, invalid field) → surface Jira's `errorMessages` back into the chat so the reporter knows what to fix, rather than a generic failure.
- Log every ticket-creation attempt (success or failure) with the Telegram message ID for traceability.

---

## 10. Suggested build phases

1. **MVP (long polling, single group, single Jira project)**
   - Command parsing, Gemini call, Jira call, reply with link. No assignee mapping yet — always unassigned.
2. **Assignee support**
   - Static mapping file, soft-fail resolution.
3. **Quality pass**
   - Multi-message context window, issue-type/priority inference tuning, `createmeta` validation on startup so bad config fails fast.
4. **Hardening**
   - Move to webhook mode if you want push-based delivery, add allowlists, add structured logging, containerize with a small `Dockerfile`, add a `docker-compose.yml` for local dev.
5. **Optional nice-to-haves**
   - Edit/confirm step: bot posts the draft ticket as a message with inline buttons ("✅ Create" / "✏️ Edit title" / "❌ Cancel") before actually filing it in Jira — this is genuinely worth doing early, since Gemini will occasionally misjudge priority or title wording, and a confirm step avoids junk tickets.
   - Duplicate detection: search Jira (`/rest/api/3/search` with JQL) for similar existing tickets before creating a new one.

---

## 11. Reference docs

| Area | Doc | Notes |
|---|---|---|
| Telegram Bot API (full reference) | https://core.telegram.org/bots/api | The canonical spec — every method/field. Search-of-truth for command parsing, replies, webhook setup. |
| Telegram: creating a bot / BotFather | https://core.telegram.org/bots | Getting your token, setting commands via `/setcommands`. |
| Telegram: webhooks vs polling | https://core.telegram.org/bots/api#setwebhook | Read this before deciding webhook mode later. |
| go-telegram-bot-api (Go client) | https://pkg.go.dev/github.com/go-telegram-bot-api/telegram-bot-api/v5 | Go doc for the library itself. |
| go-telegram-bot-api source + examples | https://github.com/go-telegram-bot-api/telegram-bot-api | `README.md` has a polling example and a webhook example side by side. |
| Jira REST API v3 — intro | https://developer.atlassian.com/cloud/jira/platform/rest/v3/intro/ | Auth, pagination, expansion params, general conventions. |
| Jira REST API v3 — create issue endpoint | https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-post | Exact request/response shape for `POST /rest/api/3/issue`, incl. optional transition/properties. |
| Jira — basic auth for REST APIs | https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/ | How to build the `email:api_token` base64 header — needed for MVP auth. |
| Jira — Atlassian Document Format (ADF) | https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/ | Required for the `description` field on v3 — this is the part people usually get wrong first. |
| Jira — issue create metadata | `GET /rest/api/3/issue/createmeta` (documented alongside the issue endpoint above) | Use this at bot startup to validate your configured project key / issue type / priority names actually exist. |
| Gemini API — overview | https://ai.google.dev/gemini-api/docs | Entry point; access via API key (ai.google.dev) vs Vertex AI. |
| Gemini API — structured output | https://ai.google.dev/gemini-api/docs/structured-output | `responseSchema` / `responseMimeType: application/json` — this is what makes Gemini return validated ticket JSON instead of prose. |
| Gemini Go SDK | https://pkg.go.dev/google.golang.org/genai | Current, actively maintained SDK. **Do not** use `github.com/google/generative-ai-go` — it's deprecated (EOL Nov 2025). |
| Gemini — API keys & key restriction | https://ai.google.dev/gemini-api/docs/api-key | Get and restrict your key (Google's been tightening unrestricted-key enforcement through 2026). |

---

## 12. Starter templates to build from (instead of from scratch)

None of these do Telegram+Jira+Gemini together — that combination is specific enough that
you'll write the integration logic yourself regardless — but a couple of them save real
time on the boilerplate (config loading, command dispatch, webhook/polling toggle, graceful
shutdown) that has nothing to do with your actual bot logic.

| Repo | Why it's a good starting point | Watch out for |
|---|---|---|
| **carry0987/Telegram-Bot** — https://github.com/carry0987/Telegram-Bot | The most complete of the bunch: supports both polling and webhook out of the box (a config flip, not a rewrite), has `/healthz` and `/readyz` endpoints (useful if you containerize this), session storage with a Redis-or-in-memory fallback, and clean layered structure (`cmd/server`, `internal/config`, `internal/bot`, `internal/handler`). Closest thing to "production-shaped" on this list. | It ships more than you need (session store, inline query demos) — strip what you don't use rather than building around all of it. |
| **nskondratev/tg-bot-template** — https://github.com/nskondratev/tg-bot-template | Clean Standard-Go-Project-Layout structure, built-in middleware pattern for update handling (good fit for your allowlist-the-group-chat-ID check), structured logging with `zerolog`. Uses `go-telegram-bot-api` under the hood, same library recommended above. | Defaults to deploying as a Google Cloud Function — fine if you're on GCP, otherwise strip the Cloud Function bits and just use its local-polling entrypoint. |
| **ankogit/go-telegram-bot-template** — https://github.com/ankogit/go-telegram-bot-template | Simple, also built on `go-telegram-bot-api`, includes a small HTTP server alongside the bot (handy if you later want a `/webhook` endpoint for Jira→Telegram notifications going the other direction) and a boltdb-backed repository pattern if you want persistence later. | Less actively maintained; treat it as a reference for structure rather than a dependency you pull in directly. |

**Recommendation:** start from **carry0987/Telegram-Bot** if you want webhook support ready to flip on later with minimal rework, or **nskondratev/tg-bot-template** if you want the leanest possible base and are fine wiring webhook support yourself when you actually need it. Either way, delete everything not related to command handling + config + graceful shutdown before you start adding your Gemini/Jira logic — it's easier to add features to a stripped-down base than to untangle a template's assumptions later.

---

## 13. Open questions to settle before coding

- Single Jira project, or does the bot need to route to different projects based on topic/channel?
- Confirm-before-create, or fire-and-forget?
- Should non-mapped assignees fall back to a default (e.g. team lead) instead of unassigned?
- Long polling (simpler, no infra) vs webhook (needs a public HTTPS endpoint) — which fits your deployment environment?