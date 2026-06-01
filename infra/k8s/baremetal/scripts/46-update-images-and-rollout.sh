#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd docker
require_cmd git
require_cmd kubectl

tag=$(git_image_tag)
export CETS_IMAGE_TAG=${CETS_IMAGE_TAG:-$tag}
[[ "$CETS_IMAGE_TAG" =~ ^[0-9a-f]{7,40}$ ]] || die "CETS_IMAGE_TAG must be a Git hash, got '$CETS_IMAGE_TAG'"
[ "$CETS_IMAGE_TAG" = "$tag" ] || die "CETS_IMAGE_TAG must match current commit $tag"

log "building images for Git commit $CETS_IMAGE_TAG"
"$SCRIPT_DIR/45-build-images.sh"

load_env
require_hash_tagged_image CETS_API_IMAGE "$CETS_API_IMAGE"
require_hash_tagged_image CETS_FRONTEND_IMAGE "$CETS_FRONTEND_IMAGE"

log "rolling Kubernetes workloads to $CETS_API_IMAGE and $CETS_FRONTEND_IMAGE"
kubectl_bm -n "$CETS_NAMESPACE" set image deployment/backend backend="$CETS_API_IMAGE"
kubectl_bm -n "$CETS_NAMESPACE" set image deployment/frontend frontend="$CETS_FRONTEND_IMAGE"
for kind in notification projection compensation export; do
  kubectl_bm -n "$CETS_NAMESPACE" set image "deployment/worker-$kind" worker="$CETS_API_IMAGE"
done

kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/backend --timeout=300s
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/frontend --timeout=300s
for kind in notification projection compensation export; do
  kubectl_bm -n "$CETS_NAMESPACE" rollout status "deployment/worker-$kind" --timeout=300s
done

log "rollout completed with Git hash image tag $CETS_IMAGE_TAG"
