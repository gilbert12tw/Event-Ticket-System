#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl

TARGET_NODE=${TARGET_NODE:-${NODES[2]}}

log "deleting one backend pod and checking endpoint recovery"
pod=$(kubectl_bm -n "$CETS_NAMESPACE" get pod -l app=backend -o jsonpath='{.items[0].metadata.name}')
kubectl_bm -n "$CETS_NAMESPACE" delete pod "$pod" --wait=false
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/backend --timeout=180s
curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null

if kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared >/dev/null 2>&1; then
  log "deleting one cloudflared pod and checking tunnel connector recovery"
  cloudflared_pod=$(kubectl_bm -n "$CETS_NAMESPACE" get pod -l app=cloudflared -o jsonpath='{.items[0].metadata.name}')
  kubectl_bm -n "$CETS_NAMESPACE" delete pod "$cloudflared_pod" --wait=false
  kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/cloudflared --timeout=180s
fi

log "cordoning and draining $TARGET_NODE"
kubectl_bm cordon "$TARGET_NODE"
kubectl_bm drain "$TARGET_NODE" --ignore-daemonsets --delete-emptydir-data --timeout=180s || {
  kubectl_bm uncordon "$TARGET_NODE" || true
  die "drain failed"
}

curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/healthz" >/dev/null
curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null

log "restoring $TARGET_NODE"
kubectl_bm uncordon "$TARGET_NODE"
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/backend --timeout=180s
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/frontend --timeout=180s
if kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared >/dev/null 2>&1; then
  kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/cloudflared --timeout=180s
fi

log "failure drill completed"
