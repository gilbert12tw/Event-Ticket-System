#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env

out="$GENERATED_DIR/routeros-metallb-bgp.rsc"
cat >"$out" <<EOF
# RouterOS v7 BGP config for Event-Ticket-System MetalLB.
# Generated from infra/k8s/baremetal/scripts/35-render-routeros-bgp.sh.
# Apply on router $METALLB_PEER_ADDRESS after reviewing existing firewall/routing policy.
#
# Expected result:
#   /routing/bgp/session/print shows three established sessions.
#   /ip/route/print where dst-address=$METALLB_INGRESS_IP/32 shows BGP ECMP or equivalent active routes.

/routing/bgp/connection/remove [find where name~"^cets-metallb-"]
/routing/bgp/instance/remove [find where name="cets-metallb"]
/ip/firewall/filter/remove [find where comment~"^cets-metallb-bgp"]

/routing/bgp/instance/add name=cets-metallb as=$METALLB_PEER_ASN router-id=$METALLB_PEER_ADDRESS

/routing/bgp/connection/add name=cets-metallb-$BAREMETAL_NODE_1 remote.address=$BAREMETAL_NODE_1_IP instance=cets-metallb local.address=$METALLB_PEER_ADDRESS local.role=ebgp hold-time=$METALLB_BGP_HOLD_TIME keepalive-time=30s routing-table=main
/routing/bgp/connection/add name=cets-metallb-$BAREMETAL_NODE_2 remote.address=$BAREMETAL_NODE_2_IP instance=cets-metallb local.address=$METALLB_PEER_ADDRESS local.role=ebgp hold-time=$METALLB_BGP_HOLD_TIME keepalive-time=30s routing-table=main
/routing/bgp/connection/add name=cets-metallb-$BAREMETAL_NODE_3 remote.address=$BAREMETAL_NODE_3_IP instance=cets-metallb local.address=$METALLB_PEER_ADDRESS local.role=ebgp hold-time=$METALLB_BGP_HOLD_TIME keepalive-time=30s routing-table=main

# Enable BGP multipath for ECMP when supported by the installed RouterOS build.
:do { /routing/bgp/instance/set [find where name="cets-metallb"] multipath=yes } on-error={ :put "Review RouterOS BGP multipath syntax for this installed version" }

/ip/firewall/filter/add chain=input action=accept protocol=tcp dst-port=179 src-address=$BAREMETAL_NODE_1_IP comment="cets-metallb-bgp $BAREMETAL_NODE_1"
/ip/firewall/filter/add chain=input action=accept protocol=tcp dst-port=179 src-address=$BAREMETAL_NODE_2_IP comment="cets-metallb-bgp $BAREMETAL_NODE_2"
/ip/firewall/filter/add chain=input action=accept protocol=tcp dst-port=179 src-address=$BAREMETAL_NODE_3_IP comment="cets-metallb-bgp $BAREMETAL_NODE_3"

/routing/bgp/session/print
/ip/route/print where dst-address=$METALLB_INGRESS_IP/32
EOF

log "wrote RouterOS BGP script to $out"
log "apply it on the MikroTik router, then run infra/k8s/baremetal/scripts/65-verify-bgp.sh"
