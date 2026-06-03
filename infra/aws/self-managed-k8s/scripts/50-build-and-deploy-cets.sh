#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"
# shellcheck source=deploy-cets/images.sh
. "$SCRIPT_DIR/deploy-cets/images.sh"
# shellcheck source=deploy-cets/platform.sh
. "$SCRIPT_DIR/deploy-cets/platform.sh"
# shellcheck source=deploy-cets/app.sh
. "$SCRIPT_DIR/deploy-cets/app.sh"
# shellcheck source=deploy-cets/cloudflare.sh
. "$SCRIPT_DIR/deploy-cets/cloudflare.sh"

load_env
require_apply
require_cmd aws
require_cmd jq
require_cmd kubectl
require_cmd helm
require_aws_identity
validate_node_count
validate_ingress_mode
validate_cets_app_env

KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"
[ -f "$KUBECONFIG_AWS" ] || die "missing kubeconfig; run 40-bootstrap-k8s.sh first"
export KUBECONFIG="$KUBECONFIG_AWS"

require_env POSTGRES_PASSWORD
require_env TOKEN_SIGNING_SECRET
require_env PROVIDER_TOKEN_SECRET
require_env BOOKING_RESERVATION_HASH_SECRET
require_env OBJECT_STORAGE_SECRET_KEY
if [ "$APP_INGRESS_MODE" = "cloudflare" ]; then
  require_env CLOUDFLARE_TUNNEL_TOKEN
fi

prepare_images
deploy_platform
deploy_app
deploy_cloudflare

log "CETS deployment completed"
