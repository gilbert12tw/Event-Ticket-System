#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd nc

log "checking router BGP TCP listener $METALLB_PEER_ADDRESS:179"
nc -vz -w 3 "$METALLB_PEER_ADDRESS" 179 >/dev/null

log "checking MetalLB FRR BGP sessions"
pods=$(kubectl_bm -n metallb-system get pod -l app.kubernetes.io/name=frr-k8s -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
[ -n "$pods" ] || die "no FRR-K8s pods found"

established=0
total=0
while IFS= read -r pod; do
  [ -n "$pod" ] || continue
  total=$((total + 1))
  summary=$(kubectl_bm -n metallb-system exec "$pod" -c frr -- vtysh -c 'show bgp summary')
  printf '%s\n' "$summary" | grep -q "$METALLB_PEER_ADDRESS" || die "$pod has no BGP neighbor $METALLB_PEER_ADDRESS"
  if printf '%s\n' "$summary" | awk -v peer="$METALLB_PEER_ADDRESS" '$1 == peer && $10 != "Active" && $10 != "Idle" && $10 != "Connect" && $10 != "OpenSent" && $10 != "OpenConfirm" { found=1 } END { exit found ? 0 : 1 }'; then
    established=$((established + 1))
  else
    printf '%s\n' "$summary" >&2
  fi
done <<<"$pods"

[ "$total" -ge 3 ] || die "expected 3 FRR pods, got $total"
[ "$established" -eq "$total" ] || die "expected all $total BGP sessions established, got $established"

log "checking advertised ingress prefix in FRR config"
for node in "${NODES[@]}"; do
  kubectl_bm -n metallb-system get frrconfiguration "metallb-$node" -o yaml |
    grep -q "$METALLB_INGRESS_IP/32" || die "metallb-$node does not advertise $METALLB_INGRESS_IP/32"
done

log "BGP verification completed"
