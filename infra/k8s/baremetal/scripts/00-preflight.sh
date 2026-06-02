#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd ssh
require_cmd ping

log "checking SSH, OS, sudo, network, and candidate VIPs"

for i in "${!NODES[@]}"; do
  node=${NODES[$i]}
  ip=${NODE_IPS[$i]}
  log "checking $node ($ip)"
  ping -c 1 -W 1 "$ip" >/dev/null || die "$node at $ip is not reachable"
  ssh_node "$node" "hostname; lsb_release -ds 2>/dev/null || true; nproc; free -h | sed -n '1,2p'; df -h / | tail -1"
  ssh_node "$node" "test \$(id -u) -ne 0" || die "do not run as root over SSH"
  if ssh_node "$node" "sudo -n true" >/dev/null 2>&1; then
    log "$node has passwordless sudo"
  else
    [ -n "${BAREMETAL_BECOME_PASSWORD:-}" ] || die "$node needs sudo password; set BAREMETAL_BECOME_PASSWORD"
    printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | ssh_node "$node" "sudo -S true" >/dev/null
  fi
done

for vip in "$K8S_API_VIP" "$METALLB_INGRESS_IP"; do
  if ping -c 1 -W 1 "$vip" >/dev/null 2>&1; then
    die "candidate VIP $vip already responds; reserve a free IP before continuing"
  fi
  log "candidate VIP $vip is not responding"
done

ping -c 1 -W 1 "$METALLB_PEER_ADDRESS" >/dev/null ||
  die "BGP peer/router $METALLB_PEER_ADDRESS is not reachable"

log "preflight passed"
