#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd jq

log "checking observability pod readiness"
kubectl_bm -n observability wait --for=condition=Ready pod --all --timeout=300s

log "checking Loki push and query path"
check_pod="obs-check-$(date +%s)"
kubectl_bm -n observability run "$check_pod" \
  --rm -i \
  --restart=Never \
  --image=curlimages/curl:8.17.0 \
  --command -- sh -eu -c '
    ts="$(date +%s)000000000"
    payload="{\"streams\":[{\"stream\":{\"job\":\"cets-observability-check\"},\"values\":[[\"$ts\",\"cets observability check\"]]}]}"
    curl -fsS -H "Content-Type: application/json" -XPOST --data-raw "$payload" http://loki-gateway/loki/api/v1/push
    sleep 3
    curl -fsG --data-urlencode "query={job=\"cets-observability-check\"}" http://loki-gateway/loki/api/v1/query_range
    echo
    curl -fsS http://kube-prometheus-stack-prometheus:9090/-/ready
    echo
    curl -fsS "http://kube-prometheus-stack-prometheus:9090/api/v1/targets?state=active"
  ' >"$GENERATED_DIR/observability-check.json"

grep -q '"resultType":"streams"' "$GENERATED_DIR/observability-check.json" || die "Loki query did not return streams"
grep -q 'cets observability check' "$GENERATED_DIR/observability-check.json" || die "Loki query did not return the pushed canary line"
grep -q 'Prometheus Server is Ready' "$GENERATED_DIR/observability-check.json" || die "Prometheus readiness check failed"
grep -q 'cets/cets-backend/0' "$GENERATED_DIR/observability-check.json" || die "Prometheus target for cets-backend ServiceMonitor not found"

log "checking Loki backend pod log ingestion"
backend_log_pod="obs-backend-log-check-$(date +%s)"
kubectl_bm -n observability run "$backend_log_pod" \
  --rm -i \
  --restart=Never \
  --image=curlimages/curl:8.17.0 \
  --command -- sh -eu -c '
    for i in $(seq 1 24); do
      result=$(curl -fsG \
        --data-urlencode "query={namespace=\"cets\", app=\"backend\"} |= \"request handled\"" \
        --data-urlencode "limit=5" \
        http://loki-gateway/loki/api/v1/query_range)
      if printf "%s" "$result" | grep -q "\"result\":\\[" && ! printf "%s" "$result" | grep -q "\"result\":\\[\\]"; then
        printf "%s\n" "$result"
        exit 0
      fi
      sleep 5
    done
    printf "%s\n" "${result:-}"
    exit 1
  ' >"$GENERATED_DIR/backend-log-check.json" || die "Loki did not return backend logs"
grep -q '"namespace":"cets"' "$GENERATED_DIR/backend-log-check.json" || die "backend Loki log is missing namespace label"
grep -q '"app":"backend"' "$GENERATED_DIR/backend-log-check.json" || die "backend Loki log is missing app label"

log "checking Tempo and Pyroscope services"
kubectl_bm -n observability get svc tempo pyroscope >/dev/null

log "observability verification completed"
