#!/usr/bin/env bash
# Grants the Cloud Build service account what it needs to deploy, then
# creates the push-to-main trigger that runs cloudbuild.yaml.
#
# Prerequisite that cannot be scripted: the Cloud Build GitHub App must
# already be installed on the repository. Connect it once at
# https://console.cloud.google.com/cloud-build/triggers/connect
# then re-run this script.
#
# Re-running is safe: IAM bindings are idempotent and an existing trigger is
# updated in place rather than duplicated.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_vars JIRA_BASE_URL JIRA_EMAIL JIRA_PROJECT_KEY TELEGRAM_ALLOWED_CHATS

TRIGGER_NAME="${TRIGGER_NAME:-Github}"
BRANCH_PATTERN="${BRANCH_PATTERN:-^main$}"

# Repo coordinates come from the git remote so they cannot drift from the
# checkout this script is run in.
remote_url="$(git -C "${REPO_ROOT}" remote get-url origin)"
slug="${remote_url#*github.com[:/]}"
slug="${slug%.git}"
REPO_OWNER="${REPO_OWNER:-${slug%%/*}}"
REPO_NAME="${REPO_NAME:-${slug##*/}}"

PROJECT_NUMBER="$(gc projects describe "${PROJECT_ID}" --format='value(projectNumber)')"
BUILD_SA="$(gc builds get-default-service-account --region=global --format='value(serviceAccountEmail)')"
BUILD_SA="${BUILD_SA##*/}"
BUILD_SA="${BUILD_SA:-${PROJECT_NUMBER}-compute@developer.gserviceaccount.com}"

echo "==> Project ${PROJECT_ID} (${PROJECT_NUMBER})"
echo "==> Repo    ${REPO_OWNER}/${REPO_NAME} branch ${BRANCH_PATTERN}"
echo "==> Build SA ${BUILD_SA}"

echo
echo "==> Granting deploy permissions to the Cloud Build service account"
for role in roles/run.admin roles/artifactregistry.writer roles/logging.logWriter; do
  gc projects add-iam-policy-binding "${PROJECT_ID}" \
    --member "serviceAccount:${BUILD_SA}" \
    --role "${role}" \
    --condition=None \
    --quiet >/dev/null
  echo "    ${role}"
done

# Deploying a service that runs as RUNTIME_SA requires impersonating it.
# Scoped to that one account rather than granted project-wide.
gc iam service-accounts add-iam-policy-binding "${RUNTIME_SA}" \
  --member "serviceAccount:${BUILD_SA}" \
  --role roles/iam.serviceAccountUser \
  --quiet >/dev/null
echo "    roles/iam.serviceAccountUser on ${RUNTIME_SA}"

echo
# `gcloud builds triggers create github` is rejected with a bare
# INVALID_ARGUMENT on this project even when the GitHub App connection is
# healthy, and `triggers update github` cannot set substitutions. Importing a
# full trigger definition is the one path that works for both create and
# update, so the trigger is expressed as YAML and imported.
TRIGGER_FILE="$(mktemp -t cloudbuild-trigger)"
trap 'rm -f "${TRIGGER_FILE}"' EXIT

{
  echo "name: ${TRIGGER_NAME}"
  # Importing over an existing trigger needs its id, otherwise the import is
  # treated as a create and hits the same INVALID_ARGUMENT.
  existing_id="$(gc builds triggers describe "${TRIGGER_NAME}" --region=global --format='value(id)' 2>/dev/null || true)"
  [[ -n "${existing_id}" ]] && echo "id: ${existing_id}"
  echo "description: Build and deploy ${SERVICE_NAME} on push to main"
  echo "github:"
  echo "  owner: ${REPO_OWNER}"
  echo "  name: ${REPO_NAME}"
  echo "  push:"
  echo "    branch: ${BRANCH_PATTERN}"
  echo "filename: cloudbuild.yaml"
  echo "includeBuildLogs: INCLUDE_BUILD_LOGS_WITH_STATUS"
  # Runs as the Cloud Build service account granted above, not as the bot's
  # runtime identity — deploying requires roles the bot must never hold.
  echo "serviceAccount: projects/${PROJECT_ID}/serviceAccounts/${BUILD_SA}"
  echo "substitutions:"
  echo "  _REGION: ${REGION}"
  echo "  _SERVICE_NAME: ${SERVICE_NAME}"
  echo "  _AR_REPO: ${AR_REPO}"
  echo "  _RUNTIME_SA_NAME: ${RUNTIME_SA_NAME}"
  echo "  _JIRA_BASE_URL: ${JIRA_BASE_URL}"
  echo "  _JIRA_EMAIL: ${JIRA_EMAIL}"
  echo "  _JIRA_PROJECT_KEY: ${JIRA_PROJECT_KEY}"
  echo "  _TELEGRAM_ALLOWED_CHATS: '${TELEGRAM_ALLOWED_CHATS}'"
} > "${TRIGGER_FILE}"

echo "==> Importing trigger ${TRIGGER_NAME}"
if ! gc builds triggers import --source="${TRIGGER_FILE}" --region=global; then
  cat >&2 <<MSG

------------------------------------------------------------------
Trigger import failed.

If ${TRIGGER_NAME} does not exist yet, Cloud Build will only accept it
once the GitHub App is installed on ${REPO_OWNER}/${REPO_NAME}, and on
this project the API rejects CLI-created triggers outright. Create one
by hand in the console, then re-run this script to fill in the build
config, substitutions and service account:

  https://console.cloud.google.com/cloud-build/triggers?project=${PROJECT_ID}

Existing triggers:
  gcloud builds triggers list --region=global --project ${PROJECT_ID}
------------------------------------------------------------------
MSG
  exit 1
fi

echo
echo "Trigger ready. Push to main, or run a build now with:"
echo "  gcloud builds triggers run ${TRIGGER_NAME} --branch=main --region=global --project ${PROJECT_ID}"
