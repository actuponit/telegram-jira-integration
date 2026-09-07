#!/usr/bin/env bash
# One-time project setup: enable APIs, create the Artifact Registry repo and
# the runtime service account. Idempotent — safe to re-run.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

echo "==> Enabling APIs on ${PROJECT_ID}"
gc services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  secretmanager.googleapis.com

echo "==> Artifact Registry repo ${AR_REPO} (${REGION})"
if ! gc artifacts repositories describe "${AR_REPO}" --location "${REGION}" >/dev/null 2>&1; then
  gc artifacts repositories create "${AR_REPO}" \
    --repository-format=docker \
    --location "${REGION}" \
    --description "Container images for ${SERVICE_NAME}"
else
  echo "    already exists"
fi

echo "==> Runtime service account ${RUNTIME_SA}"
if ! gc iam service-accounts describe "${RUNTIME_SA}" >/dev/null 2>&1; then
  gc iam service-accounts create "${RUNTIME_SA_NAME}" \
    --display-name "Cloud Run runtime for ${SERVICE_NAME}"
else
  echo "    already exists"
fi

echo
echo "Bootstrap done. Next: deploy/10-secrets.sh"
