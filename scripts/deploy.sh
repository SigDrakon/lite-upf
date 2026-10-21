#!/usr/bin/env bash
#
# Runs on the deployment target host (executed over SSH by deploy.yml).
#
# Requires environment variables:
#   IMAGE           - fully qualified image reference, e.g. ghcr.io/org/repo:v0.1.0
#   DEPLOY_PATH     - directory on the host holding the compose project
#
# Optional:
#   REGISTRY_USER   - username used to authenticate against the registry
#   REGISTRY_TOKEN  - password/token used to authenticate against the registry
#
set -euo pipefail

IMAGE="${IMAGE:?IMAGE must be set}"
DEPLOY_PATH="${DEPLOY_PATH:?DEPLOY_PATH must be set}"
COMPOSE_FILE="${DEPLOY_PATH}/docker-compose.yml"
STATE_FILE="${DEPLOY_PATH}/.deployed-image"

cd "${DEPLOY_PATH}"

# --- authenticate to the registry -------------------------------------------
if [ -n "${REGISTRY_USER:-}" ] && [ -n "${REGISTRY_TOKEN:-}" ]; then
  echo ">> Logging in to the container registry"
  echo "$REGISTRY_TOKEN" | docker login ghcr.io -u "$REGISTRY_USER" --password-stdin
fi

# --- remember the currently running image so we can roll back ----------------
if [ -f "${STATE_FILE}" ]; then
  PREVIOUS_IMAGE="$(cat "${STATE_FILE}")"
else
  PREVIOUS_IMAGE=""
fi
echo "Previous image: ${PREVIOUS_IMAGE:-<none>}"

# --- pull the new image ------------------------------------------------------
echo ">> Pulling ${IMAGE}"
docker pull "${IMAGE}"

# --- make sure the compose file references the new image ---------------------
if [ -f "${COMPOSE_FILE}" ]; then
  echo ">> Updating ${COMPOSE_FILE}"
  if grep -q "image:" "${COMPOSE_FILE}"; then
    sed -i -E "s#^([[:space:]]*image:[[:space:]]*).*#\1${IMAGE}#" "${COMPOSE_FILE}"
  fi
fi

# --- restart -----------------------------------------------------------------
echo ">> Restarting the lite-upf service"
if command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  COMPOSE="docker compose"
fi

${COMPOSE} -f "${COMPOSE_FILE}" up -d --remove-orphans
${COMPOSE} -f "${COMPOSE_FILE}" ps

# --- persist the deployed image ----------------------------------------------
echo "${IMAGE}" > "${STATE_FILE}"

# --- verify the container is still alive -------------------------------------
sleep 3
CONTAINER_ID="$(${COMPOSE} -f "${COMPOSE_FILE}" ps -q lite-upf || true)"
if [ -z "${CONTAINER_ID}" ]; then
  echo "ERROR: the lite-upf container is not running" >&2
  exit 1
fi
if [ "$(docker inspect -f '{{.State.Running}}' "${CONTAINER_ID}")" != "true" ]; then
  echo "ERROR: the lite-upf container is not in the running state" >&2
  docker logs --tail 100 "${CONTAINER_ID}" >&2 || true
  exit 1
fi

echo ">> Successfully deployed ${IMAGE}"
