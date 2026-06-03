#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env

REQUIRE_BGP=${REQUIRE_BGP:-false}
REQUIRE_APPROVED_BROWSER=${REQUIRE_APPROVED_BROWSER:-true}
RUN_FAILURE_DRILLS=${RUN_FAILURE_DRILLS:-false}
RUN_POSTGRES_FAILOVER=${RUN_POSTGRES_FAILOVER:-false}
APPROVED_BROWSER_VERIFIED=${APPROVED_BROWSER_VERIFIED:-false}

run_check() {
  local name=$1
  shift
  log "verify-all: $name"
  "$@"
}

run_check "Kubernetes HA" "$SCRIPT_DIR/61-verify-k8s-ha.sh"
run_check "application and internal ingress" env VERIFY_CLOUDFLARE=false "$SCRIPT_DIR/60-verify.sh"
run_check "Cloudflare Tunnel and Access challenge" "$SCRIPT_DIR/62-verify-cloudflare.sh"
run_check "real application API smoke" "$SCRIPT_DIR/63-verify-app-smoke.sh"
run_check "observability" "$SCRIPT_DIR/66-verify-observability.sh"
run_check "telemetry redaction" "$SCRIPT_DIR/67-verify-telemetry-redaction.sh"

if [ "$RUN_FAILURE_DRILLS" = "true" ]; then
  run_check "stateless workload failure drill" env BAREMETAL_DRILL_NODE_DRAIN=false "$SCRIPT_DIR/70-failure-drill.sh"
  if [ "$RUN_POSTGRES_FAILOVER" = "true" ]; then
    run_check "PostgreSQL primary failover drill" "$SCRIPT_DIR/71-verify-postgres-failover.sh"
  else
    run_check "PostgreSQL failover preflight" env BAREMETAL_PG_FAILOVER_PREFLIGHT_ONLY=true "$SCRIPT_DIR/71-verify-postgres-failover.sh"
    log "verify-all: skipping PostgreSQL primary deletion; set RUN_POSTGRES_FAILOVER=true to execute it after an approved disruption window"
  fi
else
  log "verify-all: skipping failure drills; set RUN_FAILURE_DRILLS=true to execute disruptive drills"
fi

if [ "$REQUIRE_BGP" = "true" ]; then
  run_check "router BGP/ECMP" "$SCRIPT_DIR/65-verify-bgp.sh"
else
  log "verify-all: skipping BGP gate because REQUIRE_BGP=false"
fi

if [ "$REQUIRE_APPROVED_BROWSER" = "true" ]; then
  [ "$APPROVED_BROWSER_VERIFIED" = "true" ] ||
    die "approved browser verification is required; open https://$CETS_PUBLIC_HOSTNAME from a WARP-enrolled approved device, then rerun with APPROVED_BROWSER_VERIFIED=true after confirming the app loads"
else
  log "verify-all: skipping approved browser gate because REQUIRE_APPROVED_BROWSER=false"
fi

log "verify-all completed"
