#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd kubectl

KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"
[ -f "$KUBECONFIG_AWS" ] || die "missing kubeconfig; run 40-bootstrap-k8s.sh first"
export KUBECONFIG="$KUBECONFIG_AWS"

PROXY_NAME=${QUICK_HTTPS_PROXY_NAME:-cets-https-preview-proxy}
TUNNEL_NAME=${QUICK_HTTPS_TUNNEL_NAME:-cets-quick-https-tunnel}

kubectl -n "$CETS_NAMESPACE" delete deployment "$TUNNEL_NAME" "$PROXY_NAME" --ignore-not-found
kubectl -n "$CETS_NAMESPACE" delete service "$PROXY_NAME" --ignore-not-found
kubectl -n "$CETS_NAMESPACE" delete configmap "$PROXY_NAME" --ignore-not-found
rm -f "$GENERATED_DIR/quick-https-url.txt"

log "temporary HTTPS quick tunnel removed"
