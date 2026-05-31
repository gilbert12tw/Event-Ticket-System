#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"
# shellcheck source=deploy-cets/platform.sh
. "$SCRIPT_DIR/deploy-cets/platform.sh"
# shellcheck source=deploy-cets/app.sh
. "$SCRIPT_DIR/deploy-cets/app.sh"
# shellcheck source=deploy-cets/observability.sh
. "$SCRIPT_DIR/deploy-cets/observability.sh"

load_env
require_apply
require_cmd kubectl
require_cmd helm
require_env CETS_API_IMAGE
require_env CETS_FRONTEND_IMAGE
require_env POSTGRES_PASSWORD
require_env TOKEN_SIGNING_SECRET
require_env PROVIDER_TOKEN_SECRET
require_env BOOKING_RESERVATION_HASH_SECRET
require_env OBJECT_STORAGE_SECRET_KEY

image_pull_block() {
  if [ -n "${CETS_IMAGE_PULL_SECRET:-}" ]; then
    cat <<PULL_SECRET
      imagePullSecrets:
      - name: $CETS_IMAGE_PULL_SECRET
PULL_SECRET
  fi
}

deploy_platform
deploy_app
deploy_observability

kubectl_bm rollout status deployment/backend -n "$CETS_NAMESPACE" --timeout=300s
kubectl_bm rollout status deployment/frontend -n "$CETS_NAMESPACE" --timeout=300s
if kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared >/dev/null 2>&1; then
  kubectl_bm rollout status deployment/cloudflared -n "$CETS_NAMESPACE" --timeout=300s
fi
log "Event Ticket System bare-metal deployment completed"
