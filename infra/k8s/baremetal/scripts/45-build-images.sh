#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd docker
require_cmd git

CETS_IMAGE_TAG=${CETS_IMAGE_TAG:-$(git_image_tag)}
[[ "$CETS_IMAGE_TAG" =~ ^[0-9a-f]{7,40}$ ]] || die "CETS_IMAGE_TAG must be a Git hash, got '$CETS_IMAGE_TAG'"
[ "$CETS_IMAGE_TAG" != "latest" ] || die "CETS_IMAGE_TAG must not be latest"

API_IMAGE_REPOSITORY=${CETS_API_IMAGE_REPOSITORY:-$(image_repository "${CETS_API_IMAGE:-cets-api}")}
FRONTEND_IMAGE_REPOSITORY=${CETS_FRONTEND_IMAGE_REPOSITORY:-$(image_repository "${CETS_FRONTEND_IMAGE:-cets-frontend}")}
API_IMAGE="$API_IMAGE_REPOSITORY:$CETS_IMAGE_TAG"
FRONTEND_IMAGE="$FRONTEND_IMAGE_REPOSITORY:$CETS_IMAGE_TAG"

require_hash_tagged_image CETS_API_IMAGE "$API_IMAGE"
require_hash_tagged_image CETS_FRONTEND_IMAGE "$FRONTEND_IMAGE"

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

if [ "${CETS_PUSH_IMAGES:-false}" = "true" ]; then
  log "pushing images to registry"
  docker_cmd push "$API_IMAGE"
  docker_cmd push "$FRONTEND_IMAGE"
else
  api_tar="$GENERATED_DIR/cets-api-image.tar"
  frontend_tar="$GENERATED_DIR/cets-frontend-image.tar"
  docker_cmd save "$API_IMAGE" -o "$api_tar"
  docker_cmd save "$FRONTEND_IMAGE" -o "$frontend_tar"
  if [ ! -O "$api_tar" ] || [ ! -O "$frontend_tar" ]; then
    if [ -n "${BAREMETAL_BECOME_PASSWORD:-}" ]; then
      printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | sudo -S chown "$(id -u):$(id -g)" "$api_tar" "$frontend_tar"
    else
      sudo -n chown "$(id -u):$(id -g)" "$api_tar" "$frontend_tar"
    fi
  fi

  for node in "${NODES[@]}"; do
    log "importing images into containerd on $node"
    copy_to_node "$api_tar" "$node" /tmp/cets-api-image.tar
    copy_to_node "$frontend_tar" "$node" /tmp/cets-frontend-image.tar
    sudo_remote "$node" "ctr -n k8s.io images import /tmp/cets-api-image.tar && ctr -n k8s.io images import /tmp/cets-frontend-image.tar && rm -f /tmp/cets-api-image.tar /tmp/cets-frontend-image.tar"
  done
fi

set_local_env_value CETS_API_IMAGE "$API_IMAGE"
set_local_env_value CETS_FRONTEND_IMAGE "$FRONTEND_IMAGE"

log "image references updated in .env.baremetal.local"
