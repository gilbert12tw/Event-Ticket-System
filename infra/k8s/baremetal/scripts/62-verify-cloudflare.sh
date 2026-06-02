#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl

require_env CETS_PUBLIC_HOSTNAME

log "checking cloudflared Kubernetes rollout"
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/cloudflared --timeout=300s
replicas=$(kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared -o jsonpath='{.status.readyReplicas}')
[ "${replicas:-0}" -ge 3 ] || die "expected 3 ready cloudflared replicas, got ${replicas:-0}"

log "checking Cloudflare HTTPS endpoint"
headers="$GENERATED_DIR/cloudflare-headers.txt"
body="$GENERATED_DIR/cloudflare-body.txt"
status=$(curl -k -sS -L --max-time 20 -D "$headers" -o "$body" -w '%{http_code}' "https://$CETS_PUBLIC_HOSTNAME/healthz" || true)
grep -qi '^server: cloudflare' "$headers" || die "response does not appear to come through Cloudflare"

if grep -q '"status":"ok"' "$body"; then
  log "approved-browser path appears to reach application /healthz"
else
  grep -Eiq 'cloudflare access|cf-access|access' "$headers" "$body" ||
    die "endpoint did not return app health and did not look like a Cloudflare Access challenge; HTTP status $status"
  log "unauthenticated curl was challenged by Cloudflare Access as expected; verify approved browser manually"
fi

log "Cloudflare verification completed"
