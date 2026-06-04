#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl

TARGET_NODE=${TARGET_NODE:-${NODES[2]}}
ARTIFACT_DIR=${BAREMETAL_DRILL_ARTIFACT_DIR:-$ROOT_DIR/artifacts/k8s-failure-drill}
RUN_ID=${BAREMETAL_DRILL_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
RUN_NODE_DRAIN=${BAREMETAL_DRILL_NODE_DRAIN:-true}
ALLOW_DATABASE_NODE_DRAIN=${BAREMETAL_DRILL_DATABASE_NODE_DRAIN:-false}
PREFLIGHT_ONLY=${BAREMETAL_DRILL_PREFLIGHT_ONLY:-false}
DRILL_EVENTS="$ARTIFACT_DIR/baremetal-failure-drill-events-$RUN_ID.txt"
DRILL_REPORT="$ARTIFACT_DIR/baremetal-failure-drill-report-$RUN_ID.md"
DRILL_STATUS=failed
REPORT_WRITTEN=false
NODE_CORDONED=false
DATABASE_PODS_ON_TARGET=""
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
  printf '%s|%s|%s\n' "$timestamp" "$target" "$stage" >>"$DRILL_EVENTS"
}

write_report() {
  local report_status=$1
  {
    printf '# Bare-Metal Failure Drill Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Status | `%s` |\n' "$report_status"
    printf '| Namespace | `%s` |\n' "$CETS_NAMESPACE"
    printf '| Host header | `%s` |\n' "$CETS_PUBLIC_HOSTNAME"
    printf '| Ingress IP | `%s` |\n' "$METALLB_INGRESS_IP"
    printf '| Target node | `%s` |\n' "$TARGET_NODE"
    printf '| Node drain requested | `%s` |\n' "$RUN_NODE_DRAIN"
    printf '| Preflight only | `%s` |\n' "$PREFLIGHT_ONLY"
    printf '| Database node drain allowed | `%s` |\n' "$ALLOW_DATABASE_NODE_DRAIN"
    printf '| Database pods on target | `%s` |\n' "${DATABASE_PODS_ON_TARGET:-none}"
    printf '| Failure reason | `%s` |\n' "${FAILURE_REASON:-none}"
    printf '| Event evidence | `%s` |\n' "$DRILL_EVENTS"
  } >"$DRILL_REPORT"
  REPORT_WRITTEN=true
  log "wrote failure drill report: $DRILL_REPORT"
}

finalize() {
  local exit_status=$?
  set +e
  if [ "$exit_status" -ne 0 ]; then
    record_event "drill" "failed"
  fi
  if [ "$NODE_CORDONED" = "true" ]; then
    record_event "$TARGET_NODE" "cleanup_uncordon_attempted"
    if kubectl_bm uncordon "$TARGET_NODE" >/dev/null 2>&1; then
      NODE_CORDONED=false
      record_event "$TARGET_NODE" "cleanup_uncordon_succeeded"
    else
      record_event "$TARGET_NODE" "cleanup_uncordon_failed"
    fi
  fi
  if [ "$REPORT_WRITTEN" != "true" ] && [ -d "$ARTIFACT_DIR" ]; then
    write_report "$DRILL_STATUS"
  fi
  exit "$exit_status"
}

mkdir -p "$ARTIFACT_DIR"
chmod 0700 "$ARTIFACT_DIR"
: >"$DRILL_EVENTS"
trap finalize EXIT

database_pods_on_target_node() {
  kubectl_bm -n "$CETS_NAMESPACE" get pods \
    -l cnpg.io/cluster=cets-postgres \
    --field-selector "spec.nodeName=$TARGET_NODE" \
    -o jsonpath='{range .items[*]}{.metadata.name}{" "}{end}' 2>/dev/null
}

inspect_database_drain_guard() {
  if ! DATABASE_PODS_ON_TARGET=$(database_pods_on_target_node | xargs); then
    record_event "$TARGET_NODE" "node_drain_blocked_database_lookup_failed"
    DRILL_STATUS=blocked
    FAILURE_REASON="could not inspect CloudNativePG pods on target node before drain"
    write_report "$DRILL_STATUS"
    fail "$FAILURE_REASON"
  fi
  if [ -n "$DATABASE_PODS_ON_TARGET" ] && [ "$ALLOW_DATABASE_NODE_DRAIN" != "true" ]; then
    record_event "$TARGET_NODE" "node_drain_blocked_database_pods"
    DRILL_STATUS=blocked
    FAILURE_REASON="database pods on target node require BAREMETAL_DRILL_DATABASE_NODE_DRAIN=true: $DATABASE_PODS_ON_TARGET"
    write_report "$DRILL_STATUS"
    fail "$FAILURE_REASON"
  fi
}

if [ "$PREFLIGHT_ONLY" = "true" ]; then
  if [ "$RUN_NODE_DRAIN" = "true" ]; then
    inspect_database_drain_guard
  fi
  record_event "$TARGET_NODE" "preflight_only_passed"
  DRILL_STATUS=preflight-passed
  write_report "$DRILL_STATUS"
  log "failure drill preflight completed without pod deletion or node drain"
  exit 0
fi

log "deleting one backend pod and checking endpoint recovery"
pod=$(kubectl_bm -n "$CETS_NAMESPACE" get pod -l app=backend -o jsonpath='{.items[0].metadata.name}')
kubectl_bm -n "$CETS_NAMESPACE" delete pod "$pod" --wait=false
record_event "backend/$pod" "deleted"
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/backend --timeout=180s
record_event "deployment/backend" "rollout_recovered"
curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null
record_event "ingress" "readyz_after_backend_delete"

if kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared >/dev/null 2>&1; then
  log "deleting one cloudflared pod and checking tunnel connector recovery"
  cloudflared_pod=$(kubectl_bm -n "$CETS_NAMESPACE" get pod -l app=cloudflared -o jsonpath='{.items[0].metadata.name}')
  kubectl_bm -n "$CETS_NAMESPACE" delete pod "$cloudflared_pod" --wait=false
  record_event "cloudflared/$cloudflared_pod" "deleted"
  kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/cloudflared --timeout=180s
  record_event "deployment/cloudflared" "rollout_recovered"
else
  record_event "deployment/cloudflared" "not_installed"
fi

if [ "$RUN_NODE_DRAIN" = "true" ]; then
  inspect_database_drain_guard

  log "cordoning and draining $TARGET_NODE"
  kubectl_bm cordon "$TARGET_NODE"
  NODE_CORDONED=true
  record_event "$TARGET_NODE" "cordoned"
  kubectl_bm drain "$TARGET_NODE" --ignore-daemonsets --delete-emptydir-data --timeout=180s || {
    if kubectl_bm uncordon "$TARGET_NODE"; then
      NODE_CORDONED=false
      record_event "$TARGET_NODE" "drain_failed_uncordoned"
    else
      record_event "$TARGET_NODE" "drain_failed_uncordon_failed"
    fi
    fail "drain failed"
  }
  record_event "$TARGET_NODE" "drained"

  curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/healthz" >/dev/null
  record_event "ingress" "healthz_after_node_drain"
  curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null
  record_event "ingress" "readyz_after_node_drain"

  log "restoring $TARGET_NODE"
  kubectl_bm uncordon "$TARGET_NODE"
  NODE_CORDONED=false
  record_event "$TARGET_NODE" "uncordoned"
else
  record_event "$TARGET_NODE" "node_drain_skipped"
fi
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/backend --timeout=180s
record_event "deployment/backend" "final_rollout_ready"
kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/frontend --timeout=180s
record_event "deployment/frontend" "final_rollout_ready"
if kubectl_bm -n "$CETS_NAMESPACE" get deployment cloudflared >/dev/null 2>&1; then
  kubectl_bm -n "$CETS_NAMESPACE" rollout status deployment/cloudflared --timeout=180s
  record_event "deployment/cloudflared" "final_rollout_ready"
fi

DRILL_STATUS=passed
write_report "$DRILL_STATUS"
log "failure drill completed"
