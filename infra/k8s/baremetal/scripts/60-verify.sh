#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl

log "checking Kubernetes nodes"
kubectl_bm get nodes -o wide
ready_nodes=$(kubectl_bm get nodes --no-headers | awk '$2 == "Ready" { count++ } END { print count+0 }')
[ "$ready_nodes" -ge 3 ] || die "expected 3 Ready nodes, got $ready_nodes"

log "checking ingress LoadBalancer IP"
ingress_ip=$(kubectl_bm -n ingress-nginx get svc ingress-nginx-controller -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
[ "$ingress_ip" = "$METALLB_INGRESS_IP" ] || die "expected ingress IP $METALLB_INGRESS_IP, got $ingress_ip"

log "checking workload rollouts"
for deploy in backend frontend redis minio mailhog worker-notification worker-projection worker-compensation worker-export; do
  kubectl_bm -n "$CETS_NAMESPACE" rollout status "deployment/$deploy" --timeout=120s
done
if kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared >/dev/null 2>&1; then
  kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/cloudflared --timeout=120s
else
  log "cloudflared deployment not found; skipping until 40-cloudflare.sh is applied"
fi

check_three_way_spread() {
  app=$1
  nodes=$(kubectl_bm -n "$CETS_NAMESPACE" get pods -l "app=$app" -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}' | sort)
  pod_count=$(printf '%s\n' "$nodes" | sed '/^$/d' | wc -l | tr -d ' ')
  node_count=$(printf '%s\n' "$nodes" | sed '/^$/d' | sort -u | wc -l | tr -d ' ')
  [ "$pod_count" = "3" ] || die "expected app=$app to have 3 pods, got $pod_count"
  [ "$node_count" = "3" ] || die "expected app=$app to be spread across 3 nodes, got $node_count: $(printf '%s' "$nodes" | paste -sd ',' -)"
}

log "checking HA replica spread across work1/work2/work3"
check_three_way_spread backend
check_three_way_spread frontend
if kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared >/dev/null 2>&1; then
  check_three_way_spread cloudflared
fi

log "checking CloudNativePG state"
kubectl_bm -n "$CETS_NAMESPACE" get cluster cets-postgres

log "checking internal HTTP endpoint through ingress-nginx"
curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/healthz" >/dev/null
curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null

if [ "${VERIFY_CLOUDFLARE:-true}" = "true" ]; then
  log "checking Cloudflare HTTPS endpoint; Access may return login HTML if browser auth is required"
  curl -fsSIL "https://$CETS_PUBLIC_HOSTNAME" | sed -n '1,12p'
fi

log "verification completed"
