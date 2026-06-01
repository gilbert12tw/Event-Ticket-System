#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
REPLICA_SERVICES=(gateway-1 gateway-2 gateway-3 frontend-1 frontend-2 frontend-3 backend-1 backend-2 backend-3)
VERIFY_RUN_ID=${CETS_PHASE3_VERIFY_RUN_ID:-phase3-$(date +%s)-$$}
VERIFY_START_SECONDS=${CETS_PHASE3_VERIFY_START_SECONDS:-$(date +%s)}
VERIFY_START_NS=${VERIFY_START_SECONDS}000000000
SENSITIVE_LOG_PATTERN='phase3-raw-(pii|signed|qr|provider|email-body|recipient-email|idempotency)-canary|signed[_ -]?token|qr_payload|auth_session|token_signing|object_storage_secret|session=|set-cookie|local_dev_.*secret|[A-Za-z0-9._%+-]+@cets\.local|Ariel Chen|Ben Lin|Carla Wu'

log() {
  printf '[phase3-compose-verify] %s\n' "$*"
}

die() {
  printf '[phase3-compose-verify] error: %s\n' "$*" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
}

env_value() {
  key=$1
  default=$2
  if [ -f "$ENV_FILE" ]; then
    value=$(awk -F= -v key="$key" '
      $0 !~ /^[[:space:]]*#/ && $1 == key {
        print substr($0, index($0, "=") + 1)
        exit
      }
    ' "$ENV_FILE")
    if [ -n "$value" ]; then
      printf '%s' "$value"
      return
    fi
  fi
  printf '%s' "$default"
}

PHASE3_EDGE_PORT=${PHASE3_EDGE_PORT:-$(env_value PHASE3_EDGE_PORT 18080)}
GRAFANA_PORT=${GRAFANA_PORT:-$(env_value GRAFANA_PORT 3000)}
PROMETHEUS_PORT=${PROMETHEUS_PORT:-$(env_value PROMETHEUS_PORT 9090)}
LOKI_PORT=${LOKI_PORT:-$(env_value LOKI_PORT 3100)}
TEMPO_PORT=${TEMPO_PORT:-$(env_value TEMPO_PORT 3200)}
PYROSCOPE_PORT=${PYROSCOPE_PORT:-$(env_value PYROSCOPE_PORT 4040)}
GRAFANA_ADMIN_USER=${GRAFANA_ADMIN_USER:-$(env_value GRAFANA_ADMIN_USER admin)}
GRAFANA_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD:-$(env_value GRAFANA_ADMIN_PASSWORD admin)}

EDGE_URL=${CETS_PHASE3_URL:-http://127.0.0.1:${PHASE3_EDGE_PORT}}
GRAFANA_URL=${CETS_GRAFANA_URL:-http://127.0.0.1:${GRAFANA_PORT}}
PROMETHEUS_URL=${CETS_PROMETHEUS_URL:-http://127.0.0.1:${PROMETHEUS_PORT}}
LOKI_URL=${CETS_LOKI_URL:-http://127.0.0.1:${LOKI_PORT}}
TEMPO_URL=${CETS_TEMPO_URL:-http://127.0.0.1:${TEMPO_PORT}}
PYROSCOPE_URL=${CETS_PYROSCOPE_URL:-http://127.0.0.1:${PYROSCOPE_PORT}}

compose() {
  docker compose \
    --env-file "$ENV_FILE" \
    -f "$ROOT_DIR/services/api/deploy/compose.yaml" \
    -f "$ROOT_DIR/services/api/deploy/compose.worker-isolation.yaml" \
    -f "$ROOT_DIR/services/api/deploy/compose.phase3-ha.yaml" \
    --profile phase3-ha \
    --profile worker-isolation \
    --profile observability \
    --profile phase3-canary \
    "$@"
}

http_get() {
  curl -fsS "$1" >/dev/null
}

fresh_url() {
  path=$1
  sequence=$2
  printf '%s%s?phase3_run_id=%s&sequence=%s' "$EDGE_URL" "$path" "$VERIFY_RUN_ID" "$sequence"
}

run_scoped_api_url() {
  sequence=$1
  printf '%s/api/v1/phase3-verify/%s?sequence=%s' "$EDGE_URL" "$VERIFY_RUN_ID" "$sequence"
}

generate_fresh_traffic() {
  log "generating fresh run-scoped traffic"
  for i in $(seq 1 20); do
    http_get "$(fresh_url /readyz "$i")"
    status=$(curl -sS -o /dev/null -w '%{http_code}' "$(run_scoped_api_url "$i")" || true)
    case "$status" in
      200 | 401 | 404)
        ;;
      *)
        die "run-scoped API probe returned unexpected status $status"
        ;;
    esac
    if [ "$((i % 5))" -eq 0 ]; then
      http_get "$(fresh_url /healthz "$i")"
      http_get "$(fresh_url / "$i")"
    fi
  done
}

tempo_search() {
  curl -fsS --get \
    --data-urlencode 'tags=service.name=cets-backend' \
    --data-urlencode 'limit=20' \
    --data-urlencode "start=$VERIFY_START_SECONDS" \
    --data-urlencode "end=$(date +%s)" \
    "$TEMPO_URL/api/search" 2>/dev/null || true
}

loki_query_range() {
  query=$1
  limit=${2:-20}
  start_ns=${3:-$VERIFY_START_NS}
  curl -fsS --get \
    --data-urlencode "query=$query" \
    --data-urlencode "limit=$limit" \
    --data-urlencode "start=$start_ns" \
    --data-urlencode "end=$(date +%s%N)" \
    "$LOKI_URL/loki/api/v1/query_range" 2>/dev/null || true
}

require_running() {
  service=$1
  container=$(compose ps -q "$service")
  [ -n "$container" ] || die "$service has no container"
  state=$(docker inspect --format '{{.State.Status}}' "$container")
  [ "$state" = "running" ] || die "$service is $state"
}

require_healthy_or_running() {
  service=$1
  container=$(compose ps -q "$service")
  [ -n "$container" ] || die "$service has no container"
  status=$(docker inspect --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container")
  case "$status" in
    "running healthy" | "running " | "running")
      return
      ;;
  esac
  die "$service is not healthy: $status"
}

check_replicas() {
  log "checking explicit 3x gateway/frontend/backend replicas"
  for service in "${REPLICA_SERVICES[@]}"; do
    require_healthy_or_running "$service"
  done
}

check_smoke() {
  log "checking external entrypoint smoke"
  http_get "$EDGE_URL/healthz"
  http_get "$EDGE_URL/readyz"
  http_get "$EDGE_URL/"
  for _ in $(seq 1 10); do
    http_get "$EDGE_URL/readyz"
  done
}

check_datasource() {
  uid=$1
  curl -fsS -u "$GRAFANA_ADMIN_USER:$GRAFANA_ADMIN_PASSWORD" "$GRAFANA_URL/api/datasources/uid/$uid" |
    grep -Eq "\"uid\"[[:space:]]*:[[:space:]]*\"$uid\"" ||
    die "Grafana datasource $uid is not provisioned"
}

wait_http_grep() {
  label=$1
  url=$2
  pattern=$3
  for _ in $(seq 1 24); do
    if curl -fsS "$url" 2>/dev/null | grep -Eqi "$pattern"; then
      return
    fi
    sleep 5
  done
  die "$label did not become ready"
}

check_lgtm_health() {
  log "checking LGTM services and datasource provisioning"
  for service in grafana prometheus loki tempo pyroscope alloy; do
    require_running "$service"
  done
  wait_http_grep "Grafana" "$GRAFANA_URL/api/health" '"database"[[:space:]]*:[[:space:]]*"ok"'
  wait_http_grep "Prometheus" "$PROMETHEUS_URL/-/ready" "Prometheus Server is Ready"
  wait_http_grep "Loki" "$LOKI_URL/ready" "^ready$"
  wait_http_grep "Tempo" "$TEMPO_URL/ready" "ready"
  for _ in $(seq 1 24); do
    if curl -fsS "$PYROSCOPE_URL/ready" >/dev/null 2>&1 ||
      curl -fsS "$PYROSCOPE_URL/-/ready" >/dev/null 2>&1; then
      break
    fi
    sleep 5
  done
  curl -fsS "$PYROSCOPE_URL/ready" >/dev/null 2>&1 ||
    curl -fsS "$PYROSCOPE_URL/-/ready" >/dev/null 2>&1 ||
    die "Pyroscope did not report ready"
  for uid in Prometheus Loki Tempo Pyroscope; do
    check_datasource "$uid"
  done
}

check_prometheus_targets() {
  log "checking Prometheus backend targets"
  targets=$(curl -fsS "$PROMETHEUS_URL/api/v1/targets?state=active")
  printf '%s\n' "$targets" | grep -q '"job":"cets-backend"' || die "Prometheus cets-backend target missing"
  printf '%s\n' "$targets" | grep -q '"health":"up"' || die "Prometheus has no healthy active targets"
}

check_trace_ingest() {
  log "checking Tempo trace ingest"
  for _ in $(seq 1 12); do
    traces=$(tempo_search)
    if printf '%s\n' "$traces" | grep -q '"traceID"'; then
      return
    fi
    generate_fresh_traffic
    sleep 5
  done
  die "Tempo did not return cets-backend traces"
}

check_run_scoped_backend_logs() {
  log "checking run-scoped backend logs"
  for _ in $(seq 1 12); do
    generate_fresh_traffic
    run_logs=$(loki_query_range "{service_name=~\"backend-.*\"} |= \"$VERIFY_RUN_ID\"" 5)
    if printf '%s\n' "$run_logs" | grep -q "$VERIFY_RUN_ID"; then
      return
    fi
    sleep 5
  done
  die "Loki did not return run-scoped backend logs"
}

check_service_graph() {
  log "checking Tempo service graph metrics"
  for _ in $(seq 1 12); do
    graph=$(curl -fsS --get \
      --data-urlencode 'query=sum(increase(traces_service_graph_request_total[2m]))' \
      "$PROMETHEUS_URL/api/v1/query" 2>/dev/null || true)
    if printf '%s\n' "$graph" | grep -Eq '"value":\[[^]]+,"[0-9.]*[1-9][0-9.]*"\]'; then
      return
    fi
    generate_fresh_traffic
    sleep 5
  done
  die "Prometheus did not return non-zero service graph metrics"
}

check_profile_data() {
  log "checking Pyroscope profile data"
  for _ in $(seq 1 12); do
    profile=$(curl -fsS --get \
      --data-urlencode 'query=process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="cets-backend"}' \
      --data-urlencode 'from=now-5m' \
      --data-urlencode 'until=now' \
      --data-urlencode 'maxNodes=64' \
      "$PYROSCOPE_URL/pyroscope/render" 2>/dev/null || true)
    if printf '%s\n' "$profile" | grep -Eq '"numTicks":[0-9]*[1-9][0-9]*'; then
      return
    fi
    sleep 5
  done
  die "Pyroscope did not return cets-backend profile samples"
}

check_loki_logs_and_redaction() {
  log "checking Loki trace logs and redaction"
  for _ in $(seq 1 12); do
    logs=$(loki_query_range '{service_name=~"backend-.*"} |= "otel_trace_id"' 5)
    if printf '%s\n' "$logs" | grep -q "otel_trace_id"; then
      break
    fi
    generate_fresh_traffic
    sleep 5
  done
  printf '%s\n' "${logs:-}" | grep -q "otel_trace_id" ||
    die "Loki did not return trace-correlated backend logs"

  compose rm -sf redaction-canary >/dev/null 2>&1 || true
  docker rm -f cets-phase3-redaction-canary >/dev/null 2>&1 || true
  canary_start_ns=$(date +%s%N)
  compose up -d --force-recreate redaction-canary >/dev/null
  for _ in $(seq 1 12); do
    redacted_logs=$(loki_query_range '{service_name="redaction-canary"} |= "phase3-redaction-canary"' 5 "$canary_start_ns")
    if printf '%s\n' "$redacted_logs" | grep -q "phase3-redaction-canary"; then
      printf '%s\n' "$redacted_logs" | grep -Eq "phase3-raw-(pii|signed|qr|provider|email-body|recipient-email|idempotency)-canary" &&
        die "Loki contains a raw redaction canary secret"
      printf '%s\n' "$redacted_logs" | grep -q "\[REDACTED\]" ||
        die "Loki canary log was found but sensitive fields were not redacted"
      compose rm -sf redaction-canary >/dev/null 2>&1 || true
      return
    fi
    sleep 5
  done
  compose rm -sf redaction-canary >/dev/null 2>&1 || true
  die "Loki did not return the redaction canary log"
}

check_app_worker_log_redaction() {
  log "checking backend and worker Loki streams for sensitive leakage"
  service_logs=$(loki_query_range '{service_name=~"backend-.*|worker-.*"}' 100)
  if printf '%s\n' "$service_logs" | grep -Eiq "$SENSITIVE_LOG_PATTERN"; then
    die "Loki backend/worker logs contain sensitive app output"
  fi
}

main() {
  have docker || die "docker is required"
  have curl || die "curl is required"
  check_replicas
  check_smoke
  check_lgtm_health
  check_prometheus_targets
  generate_fresh_traffic
  check_run_scoped_backend_logs
  check_trace_ingest
  check_service_graph
  check_profile_data
  check_loki_logs_and_redaction
  check_app_worker_log_redaction
  log "Phase 3 Compose HA simulation verified"
}

main "$@"
