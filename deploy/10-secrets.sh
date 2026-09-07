#!/usr/bin/env bash
# Creates (or adds a new version to) each Secret Manager secret and grants
# the runtime service account read access to it. Values are read from stdin
# so they never land in shell history, a file, or the process table.
#
# Non-interactive alternative, one secret at a time:
#   printf %s "$VALUE" | gcloud secrets versions add telegram-bot-token \
#     --project PROJECT_ID --data-file=-
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

for env_var in "${SECRET_ENV_VARS[@]}"; do
  secret="$(secret_name_for "${env_var}")"

  if ! gc secrets describe "${secret}" >/dev/null 2>&1; then
    echo "==> Creating secret ${secret}"
    gc secrets create "${secret}" --replication-policy=automatic
  else
    echo "==> Secret ${secret} exists"
  fi

  if gc secrets versions describe latest --secret "${secret}" >/dev/null 2>&1; then
    read -r -p "    ${secret} already has a value. Add a new version? [y/N] " answer
    [[ "${answer}" =~ ^[Yy]$ ]] || { echo "    skipped"; continue; }
  fi

  read -r -s -p "    Value for ${env_var}: " value; echo
  if [[ -z "${value}" ]]; then
    echo "    error: empty value refused for ${secret}" >&2
    exit 1
  fi
  printf %s "${value}" | gc secrets versions add "${secret}" --data-file=-
  unset value

  gc secrets add-iam-policy-binding "${secret}" \
    --member "serviceAccount:${RUNTIME_SA}" \
    --role roles/secretmanager.secretAccessor >/dev/null
  echo "    ${RUNTIME_SA} granted secretAccessor"
done

echo
echo "Secrets done. Next: deploy/20-deploy.sh"
