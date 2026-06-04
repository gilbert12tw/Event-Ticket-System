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

require_docker_daemon() {
  docker info >/dev/null 2>&1 || die "docker daemon access is required"
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
ARTIFACT_DIR=${CETS_PHASE3_VERIFY_ARTIFACT_DIR:-$ROOT_DIR/artifacts/phase3-verify}
RUN_ID=${CETS_PHASE3_VERIFY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
VERIFY_K6_PROFILE=${K6_PHASE3_VERIFY_PROFILE:-stress}
VERIFY_REPORT="$ARTIFACT_DIR/phase3-verify-report-$RUN_ID.md"
K6_ARTIFACT_DIR="$ARTIFACT_DIR/k6"
K6_SUMMARY_EVIDENCE="$K6_ARTIFACT_DIR/phase3-$VERIFY_K6_PROFILE-summary-$RUN_ID.json"
K6_REPLICA_SPREAD_EVIDENCE="$K6_ARTIFACT_DIR/phase3-$VERIFY_K6_PROFILE-replica-spread-$RUN_ID.txt"
PROM_TARGETS_EVIDENCE="$ARTIFACT_DIR/prometheus-targets-$RUN_ID.txt"
PROM_RED_EVIDENCE="$ARTIFACT_DIR/prometheus-red-$RUN_ID.txt"
PROM_CONTROLLED_ERROR_EVIDENCE="$ARTIFACT_DIR/prometheus-controlled-error-$RUN_ID.txt"
PROM_CONTROLLED_ERROR_QUERY='sum by (route,method,status_class) (cets_http_requests_total{route="/api/v1/auth/mock-provider-token",method="POST",status_class="4xx"})'
PROM_CONTROLLED_ERROR_BASELINE=0
TEMPO_SEARCH_EVIDENCE="$ARTIFACT_DIR/tempo-search-$RUN_ID.json"
TEMPO_TRACE_EVIDENCE="$ARTIFACT_DIR/tempo-trace-$RUN_ID.json"
TEMPO_ERROR_TRACE_EVIDENCE="$ARTIFACT_DIR/tempo-error-trace-$RUN_ID.json"
SERVICE_GRAPH_EVIDENCE="$ARTIFACT_DIR/prometheus-service-graph-$RUN_ID.json"
SERVICE_GRAPH_BACKEND_DEPENDENCY_EVIDENCE="$ARTIFACT_DIR/prometheus-service-graph-backend-dependency-$RUN_ID.json"
SERVICE_GRAPH_INBOUND_QUERY='sum(traces_service_graph_request_total{server="cets-backend"})'
SERVICE_GRAPH_BACKEND_DEPENDENCY_QUERY='sum(traces_service_graph_request_total{client="cets-backend"})'
SERVICE_GRAPH_INBOUND_BASELINE=0
SERVICE_GRAPH_BACKEND_DEPENDENCY_BASELINE=0
PYROSCOPE_PROFILE_EVIDENCE="$ARTIFACT_DIR/pyroscope-profile-$RUN_ID.json"
LOKI_TRACE_LOG_EVIDENCE="$ARTIFACT_DIR/loki-trace-logs-$RUN_ID.json"
LOKI_ERROR_TRACE_LOG_EVIDENCE="$ARTIFACT_DIR/loki-error-trace-logs-$RUN_ID.json"
LOKI_REDACTION_EVIDENCE="$ARTIFACT_DIR/loki-redaction-$RUN_ID.json"
TEMPO_TRACE_IDS_FILTER="$ROOT_DIR/scripts/compose/phase3-tempo-trace-ids.jq"
CONTROLLED_ERROR_TRACE_ID=$(printf '%032s' "$(printf '%s' "$RUN_ID" | tr -cd '0-9a-fA-F' | tail -c 32)" | tr ' ' '0')
CONTROLLED_ERROR_SPAN_ID=0000000000000001
if printf '%s\n' "$CONTROLLED_ERROR_TRACE_ID" | grep -Eq '^0+$'; then
  CONTROLLED_ERROR_TRACE_ID=00000000000000000000000000000001
fi

# shellcheck source=scripts/compose/phase3-verify-report.sh
. "$ROOT_DIR/scripts/compose/phase3-verify-report.sh"
# shellcheck source=scripts/compose/phase3-verify-trace-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-verify-trace-evidence.sh"
# shellcheck source=scripts/compose/phase3-verify-loki-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-verify-loki-evidence.sh"
# shellcheck source=scripts/compose/phase3-verify-prometheus-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-verify-prometheus-evidence.sh"
# shellcheck source=scripts/compose/phase3-verify-service-graph-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-verify-service-graph-evidence.sh"
# shellcheck source=scripts/compose/phase3-verify-profile-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-verify-profile-evidence.sh"
# shellcheck source=scripts/compose/phase3-verify-lgtm-health.sh
. "$ROOT_DIR/scripts/compose/phase3-verify-lgtm-health.sh"

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
  K6_PHASE3_PROFILE="$VERIFY_K6_PROFILE" \
    CETS_PHASE3_URL="$EDGE_URL" \
    CETS_PHASE3_ENV_FILE="$ENV_FILE" \
    CETS_PHASE3_K6_ARTIFACT_DIR="$K6_ARTIFACT_DIR" \
    CETS_PHASE3_K6_SUMMARY="$K6_SUMMARY_EVIDENCE" \
    CETS_PHASE3_K6_REPLICA_SPREAD="$K6_REPLICA_SPREAD_EVIDENCE" \
    "$ROOT_DIR/scripts/compose/phase3-k6.sh"
}

TEMPO_TRACE_ID=""
TEMPO_ERROR_TRACE_ID=""

main() {
  have docker || die "docker is required"
  have curl || die "curl is required"
  have jq || die "jq is required"
  require_docker_daemon
  mkdir -p "$ARTIFACT_DIR"
  chmod 0700 "$ARTIFACT_DIR"
  check_replicas
  check_smoke
  check_lgtm_health
  check_prometheus_targets
  capture_service_graph_baseline
  check_k6_distribution
  check_prometheus_red_metrics
  check_trace_ingest
  capture_prometheus_controlled_error_baseline
  check_error_trace_ingest
  check_prometheus_controlled_error_metrics
  check_service_graph
  check_profile_data
  check_loki_logs_and_redaction
  write_verify_report
  log "Phase 3 Compose HA simulation verified"
}

main "$@"
