# Shared setup for the deploy scripts. Sourced, not executed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_FILE="${CONFIG_FILE:-${SCRIPT_DIR}/config.env}"

if [[ ! -f "${CONFIG_FILE}" ]]; then
  echo "error: ${CONFIG_FILE} not found. Copy deploy/config.env.example to deploy/config.env and fill it in." >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a; source "${CONFIG_FILE}"; set +a

require_vars() {
  local missing=()
  for name in "$@"; do
    [[ -n "${!name:-}" ]] || missing+=("${name}")
  done
  if (( ${#missing[@]} > 0 )); then
    echo "error: missing config values in ${CONFIG_FILE}: ${missing[*]}" >&2
    exit 1
  fi
}

require_vars PROJECT_ID REGION SERVICE_NAME AR_REPO RUNTIME_SA_NAME

RUNTIME_SA="${RUNTIME_SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/${AR_REPO}/${SERVICE_NAME}"

gc() { gcloud --project "${PROJECT_ID}" "$@"; }

# Secrets that must exist in Secret Manager, in Cloud Run env-var order.
SECRET_ENV_VARS=(TELEGRAM_BOT_TOKEN GEMINI_API_KEY JIRA_API_TOKEN TELEGRAM_WEBHOOK_SECRET)

secret_name_for() {
  # Secret Manager name == env var name, lowercased with dashes.
  echo "$1" | tr '[:upper:]_' '[:lower:]-'
}

service_url() {
  gc run services describe "${SERVICE_NAME}" --region "${REGION}" --format='value(status.url)'
}

read_secret_value() {
  gc secrets versions access latest --secret "$(secret_name_for "$1")"
}
