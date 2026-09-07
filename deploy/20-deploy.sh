#!/usr/bin/env bash
# Builds the container with Cloud Build and deploys it to Cloud Run.
#
# Secrets come from Secret Manager via Cloud Run's native integration
# (--set-secrets), so nothing sensitive is passed on this command line or
# stored on the service. TELEGRAM_ALLOWED_CHATS is a plain env var by
# design — it is config, not a credential (ticket 03/05).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_vars JIRA_BASE_URL JIRA_EMAIL JIRA_PROJECT_KEY TELEGRAM_ALLOWED_CHATS

TAG="${TAG:-$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || date +%Y%m%d-%H%M%S)}"

secret_flags=""
for env_var in "${SECRET_ENV_VARS[@]}"; do
  secret_flags+="${env_var}=$(secret_name_for "${env_var}"):latest,"
done
secret_flags="${secret_flags%,}"

echo "==> Building ${IMAGE}:${TAG}"
gc builds submit "${REPO_ROOT}" --tag "${IMAGE}:${TAG}"

echo "==> Deploying ${SERVICE_NAME} to ${REGION}"
# --allow-unauthenticated is required: Telegram calls the webhook
# anonymously. Authenticity is enforced in-process by verifying the
# X-Telegram-Bot-Api-Secret-Token header against TELEGRAM_WEBHOOK_SECRET.
gc run deploy "${SERVICE_NAME}" \
  --image "${IMAGE}:${TAG}" \
  --region "${REGION}" \
  --platform managed \
  --service-account "${RUNTIME_SA}" \
  --allow-unauthenticated \
  --port 8080 \
  --min-instances 0 \
  --max-instances 4 \
  --timeout 120 \
  --set-env-vars "JIRA_BASE_URL=${JIRA_BASE_URL},JIRA_EMAIL=${JIRA_EMAIL},JIRA_PROJECT_KEY=${JIRA_PROJECT_KEY},TELEGRAM_ALLOWED_CHATS=${TELEGRAM_ALLOWED_CHATS}" \
  --set-secrets "${secret_flags}"

echo
echo "Deployed: $(service_url)"
echo "Next: deploy/30-set-webhook.sh"
