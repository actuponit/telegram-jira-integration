#!/usr/bin/env bash
# Points the Telegram webhook at the deployed Cloud Run URL, with the
# secret_token Telegram will send back in X-Telegram-Bot-Api-Secret-Token.
# Both values are pulled from Secret Manager — never typed here.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

URL="$(service_url)"
[[ -n "${URL}" ]] || { echo "error: service ${SERVICE_NAME} not deployed yet" >&2; exit 1; }
WEBHOOK_URL="${URL}/webhook/telegram"

BOT_TOKEN="$(read_secret_value TELEGRAM_BOT_TOKEN)"
WEBHOOK_SECRET="$(read_secret_value TELEGRAM_WEBHOOK_SECRET)"

echo "==> Registering webhook ${WEBHOOK_URL}"
response="$(curl -sS -X POST "https://api.telegram.org/bot${BOT_TOKEN}/setWebhook" \
  --data-urlencode "url=${WEBHOOK_URL}" \
  --data-urlencode "secret_token=${WEBHOOK_SECRET}" \
  --data-urlencode "allowed_updates=[\"message\"]" \
  --data-urlencode "drop_pending_updates=true")"
echo "${response}"

case "${response}" in
  *'"ok":true'*) ;;
  *) echo "error: setWebhook failed" >&2; exit 1 ;;
esac

echo
echo "Webhook set. Next: deploy/40-verify.sh"
