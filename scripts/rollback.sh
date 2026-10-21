#!/usr/bin/env bash
#
# Rolls the deployment back to the previously recorded image.
# Runs on the deployment target host (executed over SSH by deploy.yml).
#
# Requires environment variables:
#   DEPLOY_PATH - directory on the host holding the compose project
#
set -euo pipefail

DEPLOY_PATH="${DEPLOY_PATH:?DEPLOY_PATH must be set}"
COMPOSE_FILE="${DEPLOY_PATH}/docker-compose.yml"
STATE_FILE="${DEPLOY_PATH}/.deployed-image"

cd "${DEPLOY_PATH}"

if [ ! -f "${STATE_FILE}" ]; then
  echo "No previous deployment recorded - nothing to roll back to." >&2
  exit 0
fi

PREVIOUS_IMAGE="$(cat "${STATE_FILE}")"
if [ -z "${PREVIOUS_IMAGE}" ]; then
  echo "Previous image reference is empty - nothing to roll back to." >&2
  exit 0
fi

echo ">> Rolling back to ${PREVIOUS_IMAGE}"
docker pull "${PREVIOUS_IMAGE}" || {
  echo "ERROR: could not pull the previous image ${PREVIOUS_IMAGE}" >&2
  exit 1
}

if [ -f "${COMPOSE_FILE}" ]; then
  sed -i -E "s#^([[:space:]]*image:[[:space:]]*).*#\1${PREVIOUS_IMAGE}#" "${COMPOSE_FILE}"
fi

if command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  COMPOSE="docker compose"
fi

${COMPOSE} -f "${COMPOSE_FILE}" up -d --remove-orphans
${COMPOSE} -f "${COMPOSE_FILE}" ps

echo ">> Rolled back to ${PREVIOUS_IMAGE}"
