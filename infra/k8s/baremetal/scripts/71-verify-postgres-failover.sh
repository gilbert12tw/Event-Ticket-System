#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl
require_env POSTGRES_DB
require_env POSTGRES_USER

cluster_jsonpath='{.status.currentPrimary}'

current_primary() {
  kubectl_bm -n "$CETS_NAMESPACE" get cluster cets-postgres -o jsonpath="$cluster_jsonpath"
}

psql_on_primary() {
  local primary=$1
  local sql=$2
  kubectl_bm -n "$CETS_NAMESPACE" exec "$primary" -c postgres -- \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc "$sql"
}

assert_sync_replication() {
  local primary=$1
  local sync_names
  local sync_count

  sync_names=$(psql_on_primary "$primary" 'SHOW synchronous_standby_names;')
  [ -n "$sync_names" ] || die "synchronous_standby_names is empty on $primary"

  sync_count=$(psql_on_primary "$primary" "SELECT count(*) FROM pg_stat_replication WHERE sync_state = 'sync';")
  [ "${sync_count:-0}" -ge 1 ] ||
    die "expected at least one synchronous standby on $primary, got ${sync_count:-0}"
}

wait_cluster_healthy() {
  local deadline=$((SECONDS + 360))
  while [ "$SECONDS" -lt "$deadline" ]; do
    ready=$(kubectl_bm -n "$CETS_NAMESPACE" get cluster cets-postgres -o jsonpath='{.status.readyInstances}' 2>/dev/null || true)
    phase=$(kubectl_bm -n "$CETS_NAMESPACE" get cluster cets-postgres -o jsonpath='{.status.phase}' 2>/dev/null || true)
    if [ "$ready" = "3" ] && [ "$phase" = "Cluster in healthy state" ]; then
      return
    fi
    sleep 5
  done
  kubectl_bm -n "$CETS_NAMESPACE" get cluster cets-postgres
  die "CloudNativePG cluster did not return to 3 ready healthy instances"
}

old_primary=$(current_primary)
[ -n "$old_primary" ] || die "could not determine current PostgreSQL primary"

log "checking synchronous replication before PostgreSQL failover"
assert_sync_replication "$old_primary"

probe_id="ha-failover-probe-$(date +%s)"
log "writing committed failover probe $probe_id"
psql_on_primary "$old_primary" "INSERT INTO audit_logs (audit_id, actor_id, role, action, entity_type, entity_id, metadata) VALUES ('$probe_id', 'ha-drill', 'system_admin', 'ha.failover_probe', 'system', '$probe_id', '{\"probe\":\"postgres_failover\"}'::jsonb);"

log "deleting current PostgreSQL primary pod $old_primary"
kubectl_bm -n "$CETS_NAMESPACE" delete pod "$old_primary" --grace-period=0 --force --wait=false

log "waiting for PostgreSQL primary failover away from $old_primary"
deadline=$((SECONDS + 180))
new_primary=""
while [ "$SECONDS" -lt "$deadline" ]; do
  candidate=$(current_primary 2>/dev/null || true)
  if [ -n "$candidate" ] && [ "$candidate" != "$old_primary" ]; then
    new_primary=$candidate
    break
  fi
  sleep 3
done

[ -n "$new_primary" ] || die "PostgreSQL primary did not fail over away from $old_primary"
log "PostgreSQL primary moved from $old_primary to $new_primary"

log "waiting for CloudNativePG cluster to recover"
wait_cluster_healthy
kubectl_bm -n "$CETS_NAMESPACE" get cluster cets-postgres

log "checking synchronous replication after PostgreSQL failover"
assert_sync_replication "$new_primary"

survived=$(psql_on_primary "$new_primary" "SELECT count(*) FROM audit_logs WHERE audit_id = '$probe_id';")
[ "$survived" = "1" ] || die "committed failover probe $probe_id was not found after failover"

log "checking application database readiness after PostgreSQL failover"
curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null
"$SCRIPT_DIR/63-verify-app-smoke.sh" >/dev/null

log "PostgreSQL failover verification completed"
