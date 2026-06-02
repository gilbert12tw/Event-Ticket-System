#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
REPLICA_SERVICES=(gateway-1 gateway-2 gateway-3 frontend-1 frontend-2 frontend-3 backend-1 backend-2 backend-3)

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

prom_query() {
  curl -fsS --get --data-urlencode "query=$1" "$PROMETHEUS_URL/api/v1/query"
}

json_scalar_value() {
  sed -n \
    -e 's/.*"value":\[[^]]*,"\([0-9.][0-9.]*\)".*/\1/p' \
    -e 's/.*"result":\[[^]]*,"\([0-9.][0-9.]*\)".*/\1/p' |
    tail -n 1
}

prom_query_nonzero() {
  query=$1
  response=$(prom_query "$query" 2>/dev/null || true)
  printf '%s\n' "$response" | grep -Eq '"value":\[[^]]+,"[0-9.]*[1-9][0-9.]*"\]'
}

prom_query_at_least() {
  query=$1
  minimum=$2
  response=$(prom_query "$query" 2>/dev/null || true)
  value=$(printf '%s\n' "$response" | json_scalar_value)
  [ -n "$value" ] || return 1
  awk -v value="$value" -v minimum="$minimum" 'BEGIN { exit(value >= minimum ? 0 : 1) }'
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

check_k6_distribution() {
  log "checking k6 Phase 3 load and replica distribution"
  K6_PHASE3_PROFILE=${K6_PHASE3_VERIFY_PROFILE:-stress} \
    CETS_PHASE3_URL="$EDGE_URL" \
    CETS_PHASE3_ENV_FILE="$ENV_FILE" \
    "$ROOT_DIR/scripts/compose/phase3-k6.sh"
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

check_prometheus_red_metrics() {
  log "checking Prometheus RED metrics from k6 load"
  for _ in $(seq 1 24); do
    if prom_query_nonzero 'sum(increase(cets_http_requests_total[15m]))' &&
      prom_query_nonzero 'sum(increase(cets_http_requests_total{status_class=~"4xx|5xx"}[15m]))' &&
      prom_query_nonzero 'sum(increase(cets_http_request_seconds_count[15m]))' &&
      prom_query_at_least 'scalar(count(count by (route, method, status_class) (increase(cets_http_requests_total[15m]) > 0)))' 3 &&
      prom_query_at_least 'scalar(count(count by (instance) (increase(cets_http_requests_total[15m]) > 0)))' 3; then
      return
    fi
    sleep 5
  done
  die "Prometheus did not return complete RED evidence by route, status class, latency, and backend instance"
}

TEMPO_TRACE_ID=""

check_trace_ingest() {
  log "checking Tempo trace ingest"
  for _ in $(seq 1 24); do
    traces=$(curl -fsS "$TEMPO_URL/api/search?tags=service.name%3Dcets-backend&limit=1" 2>/dev/null || true)
    trace_id=$(printf '%s\n' "$traces" | sed -n 's/.*"traceID":"\([a-fA-F0-9][a-fA-F0-9]*\)".*/\1/p' | head -n 1)
    if [ -n "$trace_id" ]; then
      trace_detail=$(curl -fsS "$TEMPO_URL/api/traces/$trace_id" 2>/dev/null || true)
      if printf '%s\n' "$trace_detail" | grep -q "cets-backend" &&
        printf '%s\n' "$trace_detail" | grep -Eq "http.route|cets.route|rootTraceName"; then
        TEMPO_TRACE_ID=$trace_id
        return
      fi
    fi
    if printf '%s\n' "$traces" | grep -q '"traceID"'; then
      TEMPO_TRACE_ID=$(printf '%s\n' "$traces" | sed -n 's/.*"traceID":"\([a-fA-F0-9][a-fA-F0-9]*\)".*/\1/p' | head -n 1)
      return
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Tempo did not return cets-backend traces with route evidence"
}

check_service_graph() {
  log "checking Tempo service graph metrics"
  for _ in $(seq 1 12); do
    if prom_query_nonzero 'sum(increase(traces_service_graph_request_total[15m]))' &&
      prom_query_nonzero 'sum(increase(traces_service_graph_request_total{server="cets-backend"}[15m]))'; then
      return
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Prometheus did not return service graph metrics involving cets-backend"
}

check_profile_data() {
  log "checking Pyroscope profile data"
  for _ in $(seq 1 12); do
    profile=$(curl -fsS --get \
      --data-urlencode 'query=process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="cets-backend"}' \
      --data-urlencode 'from=now-1h' \
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
  [ -n "$TEMPO_TRACE_ID" ] || die "Tempo trace ID is required before Loki trace-log correlation"
  for _ in $(seq 1 12); do
    logs=$(curl -fsS --get \
      --data-urlencode "query={service_name=~\"backend-.*\"} |= \"$TEMPO_TRACE_ID\" |= \"otel_trace_id\"" \
      --data-urlencode "limit=1" \
      "$LOKI_URL/loki/api/v1/query_range" 2>/dev/null || true)
    if printf '%s\n' "$logs" | grep -q "otel_trace_id" &&
      printf '%s\n' "$logs" | grep -q "$TEMPO_TRACE_ID"; then
      break
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  printf '%s\n' "${logs:-}" | grep -q "$TEMPO_TRACE_ID" ||
    die "Loki did not return backend logs for Tempo trace $TEMPO_TRACE_ID"

  compose rm -sf redaction-canary >/dev/null 2>&1 || true
  docker rm -f cets-phase3-redaction-canary >/dev/null 2>&1 || true
  compose up -d --force-recreate redaction-canary >/dev/null
  for _ in $(seq 1 24); do
    redacted_logs=$(curl -fsS "$LOKI_URL/loki/api/v1/query_range?query=%7Bservice_name%3D%22redaction-canary%22%7D%20%7C%3D%20%22phase3-redaction-canary%22&limit=5" 2>/dev/null || true)
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

main() {
  have docker || die "docker is required"
  have curl || die "curl is required"
  check_replicas
  check_smoke
  check_k6_distribution
  check_lgtm_health
  check_prometheus_targets
  check_prometheus_red_metrics
  check_trace_ingest
  check_service_graph
  check_profile_data
  check_loki_logs_and_redaction
  log "Phase 3 Compose HA simulation verified"
}

main "$@"
