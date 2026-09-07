# 05 — Cloud Run deploy + Secret Manager

**What to build:** the bot runs live on Cloud Run with webhook delivery and all secrets pulled from Google Secret Manager — nothing sensitive in env vars or source.

**Blocked by:** 02 — Core ticket creation, 03 — Chat allowlist

**Status:** ready-for-agent

- [ ] Bot deployed to Cloud Run, scales to zero
- [ ] `TELEGRAM_BOT_TOKEN`, `GEMINI_API_KEY`, `JIRA_API_TOKEN`, and the webhook `secret_token` are all sourced from Google Secret Manager via Cloud Run's native integration
- [ ] Telegram webhook registered against the deployed Cloud Run URL with the `secret_token` set
- [ ] A real `/to-ticket` sent in the allowlisted chat against the deployed instance produces a real Jira Issue and chat reply
- [ ] Health-check endpoint responds on the deployed instance
