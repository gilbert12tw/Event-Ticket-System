#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl
POSTGRES_DB=${POSTGRES_DB:-cets}
POSTGRES_USER=${POSTGRES_USER:-postgres}

cluster_jsonpath='{.status.currentPrimary}'
ARTIFACT_DIR=${BAREMETAL_PG_FAILOVER_ARTIFACT_DIR:-$ROOT_DIR/artifacts/k8s-postgres-failover}
RUN_ID=${BAREMETAL_PG_FAILOVER_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
SMOKE_SCRIPT=${BAREMETAL_PG_FAILOVER_SMOKE_SCRIPT:-$SCRIPT_DIR/63-verify-app-smoke.sh}
PREFLIGHT_ONLY=${BAREMETAL_PG_FAILOVER_PREFLIGHT_ONLY:-false}
[[ "$RUN_ID" =~ ^[A-Za-z0-9._-]+$ ]] || die "BAREMETAL_PG_FAILOVER_RUN_ID contains unsupported characters"
FAILOVER_EVENTS="$ARTIFACT_DIR/postgres-failover-events-$RUN_ID.txt"
FAILOVER_REPORT="$ARTIFACT_DIR/postgres-failover-report-$RUN_ID.md"
FAILOVER_STATUS=failed
REPORT_WRITTEN=false
old_primary=""
new_primary=""
probe_id=""
FAILURE_REASON=""

fail() {
  FAILURE_REASON=$1
  die "$1"
}

record_event() {
  local target=$1
  local stage=$2
  local timestamp
  timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  printf '%s|%s|%s\n' "$timestamp" "$target" "$stage" >>"$FAILOVER_EVENTS"
}

write_report() {
  local report_status=$1
  {
    printf '# PostgreSQL Failover Drill Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Status | `%s` |\n' "$report_status"
    printf '| Namespace | `%s` |\n' "$CETS_NAMESPACE"
    printf '| Old primary | `%s` |\n' "${old_primary:-unknown}"
    printf '| New primary | `%s` |\n' "${new_primary:-unknown}"
    printf '| Probe audit ID | `%s` |\n' "${probe_id:-unknown}"
    printf '| Failure reason | `%s` |\n' "${FAILURE_REASON:-none}"
    printf '| Smoke script | `%s` |\n' "$SMOKE_SCRIPT"
    printf '| Event evidence | `%s` |\n' "$FAILOVER_EVENTS"
  } >"$FAILOVER_REPORT"
  REPORT_WRITTEN=true
  log "wrote PostgreSQL failover report: $FAILOVER_REPORT"
}

finalize() {
  local exit_status=$?
  set +e
  if [ "$exit_status" -ne 0 ]; then
    record_event "drill" "failed"
  fi
  if [ "$REPORT_WRITTEN" != "true" ] && [ -d "$ARTIFACT_DIR" ]; then
    write_report "$FAILOVER_STATUS"
  fi
  exit "$exit_status"
}

mkdir -p "$ARTIFACT_DIR"
chmod 0700 "$ARTIFACT_DIR"
: >"$FAILOVER_EVENTS"
trap finalize EXIT

[ -x "$SMOKE_SCRIPT" ] || fail "PostgreSQL failover smoke script is not executable: $SMOKE_SCRIPT"

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

  sync_names=$(psql_on_primary "$primary" 'SHOW synchronous_standby_names;') ||
    fail "could not read synchronous_standby_names on $primary"
  [ -n "$sync_names" ] || fail "synchronous_standby_names is empty on $primary"

  sync_count=$(psql_on_primary "$primary" "SELECT count(*) FROM pg_stat_replication WHERE sync_state IN ('sync', 'quorum');") ||
    fail "could not read synchronous standby count on $primary"
  [ "${sync_count:-0}" -ge 1 ] ||
    fail "expected at least one sync or quorum standby on $primary, got ${sync_count:-0}"
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
  fail "CloudNativePG cluster did not return to 3 ready healthy instances"
}

old_primary=$(current_primary) || fail "could not determine current PostgreSQL primary"
[ -n "$old_primary" ] || fail "could not determine current PostgreSQL primary"
record_event "$old_primary" "old_primary_detected"

log "checking synchronous replication before PostgreSQL failover"
assert_sync_replication "$old_primary"
record_event "$old_primary" "sync_replication_before_ok"

if [ "$PREFLIGHT_ONLY" = "true" ]; then
  new_primary="not-run"
  probe_id="not-run"
  record_event "$old_primary" "preflight_only_passed"
  FAILOVER_STATUS=preflight-passed
  write_report "$FAILOVER_STATUS"
  log "PostgreSQL failover preflight completed without primary deletion"
  exit 0
fi

probe_id="ha-failover-probe-$RUN_ID"
log "writing committed failover probe $probe_id"
psql_on_primary "$old_primary" "INSERT INTO audit_logs (audit_id, actor_id, role, action, entity_type, entity_id, metadata) VALUES ('$probe_id', 'ha-drill', 'system_admin', 'ha.failover_probe', 'system', '$probe_id', '{\"probe\":\"postgres_failover\"}'::jsonb);" ||
  fail "could not write failover probe on $old_primary"
record_event "$old_primary" "probe_committed"

log "deleting current PostgreSQL primary pod $old_primary"
kubectl_bm -n "$CETS_NAMESPACE" delete pod "$old_primary" --grace-period=0 --force --wait=false ||
  fail "could not delete PostgreSQL primary pod $old_primary"
record_event "$old_primary" "primary_deleted"

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

[ -n "$new_primary" ] || fail "PostgreSQL primary did not fail over away from $old_primary"
log "PostgreSQL primary moved from $old_primary to $new_primary"
record_event "$new_primary" "new_primary_detected"

log "waiting for CloudNativePG cluster to recover"
wait_cluster_healthy
kubectl_bm -n "$CETS_NAMESPACE" get cluster cets-postgres
record_event "cets-postgres" "cluster_healthy"

log "checking synchronous replication after PostgreSQL failover"
assert_sync_replication "$new_primary"
record_event "$new_primary" "sync_replication_after_ok"

survived=$(psql_on_primary "$new_primary" "SELECT count(*) FROM audit_logs WHERE audit_id = '$probe_id';") ||
  fail "could not verify committed failover probe $probe_id after failover"
[ "$survived" = "1" ] || fail "committed failover probe $probe_id was not found after failover"
record_event "$new_primary" "probe_survived"

log "checking application database readiness after PostgreSQL failover"
curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null ||
  fail "application readiness check failed after PostgreSQL failover"
record_event "ingress" "readyz_after_failover"
"$SMOKE_SCRIPT" >/dev/null ||
  fail "application smoke script failed after PostgreSQL failover"
record_event "app-smoke" "passed"

FAILOVER_STATUS=passed
write_report "$FAILOVER_STATUS"
log "PostgreSQL failover verification completed"
