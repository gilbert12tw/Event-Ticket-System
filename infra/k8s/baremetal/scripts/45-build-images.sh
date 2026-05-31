#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd docker
require_env BAREMETAL_BECOME_PASSWORD

API_IMAGE=${CETS_API_IMAGE:-cets-api:baremetal}
FRONTEND_IMAGE=${CETS_FRONTEND_IMAGE:-cets-frontend:baremetal}

log "building local images $API_IMAGE and $FRONTEND_IMAGE"
docker_cmd() {
  if docker info >/dev/null 2>&1; then
    docker "$@"
  else
    printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | sudo -S docker "$@"
  fi
}

docker_cmd build -t "$API_IMAGE" -f services/api/Dockerfile .
docker_cmd build -t "$FRONTEND_IMAGE" -f apps/web/Dockerfile .

api_tar="$GENERATED_DIR/cets-api-image.tar"
frontend_tar="$GENERATED_DIR/cets-frontend-image.tar"
docker_cmd save "$API_IMAGE" -o "$api_tar"
docker_cmd save "$FRONTEND_IMAGE" -o "$frontend_tar"
printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | sudo -S chown "$(id -u):$(id -g)" "$api_tar" "$frontend_tar"

for node in "${NODES[@]}"; do
  log "importing images into containerd on $node"
  copy_to_node "$api_tar" "$node" /tmp/cets-api-image.tar
  copy_to_node "$frontend_tar" "$node" /tmp/cets-frontend-image.tar
  sudo_remote "$node" "ctr -n k8s.io images import /tmp/cets-api-image.tar && ctr -n k8s.io images import /tmp/cets-frontend-image.tar && rm -f /tmp/cets-api-image.tar /tmp/cets-frontend-image.tar"
done

set_local_env_value CETS_API_IMAGE "$API_IMAGE"
set_local_env_value CETS_FRONTEND_IMAGE "$FRONTEND_IMAGE"

log "images imported on all nodes and .env.baremetal.local updated"
