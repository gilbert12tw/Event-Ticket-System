#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl

require_env CETS_PUBLIC_HOSTNAME

check_cloudflare_endpoint() {
  local label=$1
  local url=$2
  local health_pattern=$3
  local headers="$GENERATED_DIR/cloudflare-$label-headers.txt"
  local body="$GENERATED_DIR/cloudflare-$label-body.txt"
  local status
  status=$(curl -k -sS -L --max-time 20 -D "$headers" -o "$body" -w '%{http_code}' "$url" || true)
  grep -qi '^server: cloudflare' "$headers" || die "$label response does not appear to come through Cloudflare"

  if grep -Eq "$health_pattern" "$body"; then
    log "$label approved-browser path appears to reach origin"
  elif [ "${CLOUDFLARE_ACCESS_ENABLED:-true}" = "false" ]; then
    die "$label public endpoint did not reach origin health; HTTP status $status"
  else
    grep -Eiq 'cloudflare access|cf-access|access' "$headers" "$body" ||
      die "$label endpoint did not return origin health and did not look like a Cloudflare Access challenge; HTTP status $status"
    log "$label unauthenticated curl was challenged by Cloudflare Access as expected"
  fi
}

log "checking cloudflared Kubernetes rollout"
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/cloudflared --timeout=300s
replicas=$(kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared -o jsonpath='{.status.readyReplicas}')
[ "${replicas:-0}" -ge 3 ] || die "expected 3 ready cloudflared replicas, got ${replicas:-0}"

log "checking Cloudflare HTTPS endpoint"
check_cloudflare_endpoint "app" "https://$CETS_PUBLIC_HOSTNAME/healthz" '"status"[[:space:]]*:[[:space:]]*"ok"'
if [ -n "${GRAFANA_PUBLIC_HOSTNAME:-}" ]; then
  check_cloudflare_endpoint "grafana" "https://$GRAFANA_PUBLIC_HOSTNAME/api/health" '"database"[[:space:]]*:[[:space:]]*"ok"'
fi

log "Cloudflare verification completed"
