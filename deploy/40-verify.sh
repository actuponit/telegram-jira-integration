#!/usr/bin/env bash
# Post-deploy checks: health endpoint, webhook registration, recent logs.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

URL="$(service_url)"
[[ -n "${URL}" ]] || { echo "error: service ${SERVICE_NAME} not deployed" >&2; exit 1; }

echo "==> Health check ${URL}/healthz"
code="$(curl -sS -o /dev/null -w '%{http_code}' "${URL}/healthz")"
echo "    HTTP ${code}"
[[ "${code}" == "200" ]] || { echo "error: health check failed" >&2; exit 1; }

echo "==> Webhook info"
BOT_TOKEN="$(read_secret_value TELEGRAM_BOT_TOKEN)"
info="$(curl -sS "https://api.telegram.org/bot${BOT_TOKEN}/getWebhookInfo")"
echo "${info}"
case "${info}" in
  *"${URL}/webhook/telegram"*) echo "    webhook points at this service" ;;
  *) echo "warning: webhook URL does not match ${URL}/webhook/telegram" >&2 ;;
esac

echo "==> Last 20 log lines"
gc run services logs read "${SERVICE_NAME}" --region "${REGION}" --limit 20 || true

echo
echo "Remaining manual check: send /to-ticket as a reply in the allowlisted"
echo "chat and confirm a real Jira Issue is created and linked back in chat."
