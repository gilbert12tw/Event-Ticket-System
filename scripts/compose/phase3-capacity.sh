#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
PHASE3_EDGE_PORT=${PHASE3_EDGE_PORT:-}
BASE_URL=${CETS_PHASE3_URL:-}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.7.1-with-browser}
SCRIPT=/k6/k8s-capacity-rps.js
K6_REPORT_FILTER="$ROOT_DIR/scripts/compose/phase3-capacity-k6-report.jq"
PROM_VECTOR_REPORT_FILTER="$ROOT_DIR/scripts/compose/phase3-prometheus-vector-report.jq"
PROM_VECTOR_TARGET_FILTER="$ROOT_DIR/scripts/compose/phase3-prometheus-backend-targets-required.jq"
CORRECTNESS_CHECK="$ROOT_DIR/scripts/compose/phase3-correctness-check.awk"
CORRECTNESS_SUMMARY_SQL="$ROOT_DIR/scripts/compose/phase3-correctness-summary.sql"
HEADER_REPLICAS_AWK="$ROOT_DIR/scripts/compose/phase3-header-replicas.awk"
REPLICA_SPREAD_CHECK="$ROOT_DIR/scripts/compose/phase3-replica-spread-check.awk"
PHASE3_BACKEND_PROMETHEUS_TARGETS="backend-1:8080 backend-2:8080 backend-3:8080"
ARTIFACT_DIR=${CETS_PHASE3_CAPACITY_ARTIFACT_DIR:-$ROOT_DIR/artifacts/phase3-capacity}
RUN_ID=${CETS_PHASE3_CAPACITY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
DURATION=${CETS_PHASE3_CAPACITY_DURATION:-82s}
READ_RATIO=${CETS_PHASE3_CAPACITY_READ_RATIO:-0.8}
START_RPS=${CETS_PHASE3_CAPACITY_START_RPS:-50}
STEP_RPS=${CETS_PHASE3_CAPACITY_STEP_RPS:-50}
MAX_RPS=${CETS_PHASE3_CAPACITY_MAX_RPS:-1000}
RESOLUTION_RPS=${CETS_PHASE3_CAPACITY_RESOLUTION_RPS:-25}
EMPLOYEE_PREFIX=${CETS_PHASE3_CAPACITY_EMPLOYEE_PREFIX:-PB}
EMPLOYEE_COUNT=${CETS_PHASE3_CAPACITY_EMPLOYEE_COUNT:-10000}
HOT_EVENT_CAPACITY=${CETS_PHASE3_CAPACITY_HOT_EVENT_CAPACITY:-$EMPLOYEE_COUNT}
PROMETHEUS_PORT=${PROMETHEUS_PORT:-}
PROMETHEUS_URL=${CETS_PROMETHEUS_URL:-}
REPORT_ONLY_RPS=${CETS_PHASE3_CAPACITY_REPORT_ONLY_RPS:-}
CAPACITY_VERIFY_REPORT=${CETS_PHASE3_CAPACITY_VERIFY_REPORT:-}
BOTTLENECK_NOTE=${CETS_PHASE3_CAPACITY_BOTTLENECK_NOTE:-not identified in this run}
OPTIMIZATION_RESULT=${CETS_PHASE3_CAPACITY_OPTIMIZATION_RESULT:-not yet optimized}
REPLICA_SAMPLES=${CETS_PHASE3_CAPACITY_REPLICA_SAMPLES:-90}

# shellcheck source=scripts/compose/phase3-capacity-report.sh
. "$ROOT_DIR/scripts/compose/phase3-capacity-report.sh"
# shellcheck source=scripts/compose/phase3-capacity-preflight.sh
. "$ROOT_DIR/scripts/compose/phase3-capacity-preflight.sh"
# shellcheck source=scripts/compose/phase3-capacity-prometheus-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-capacity-prometheus-evidence.sh"
# shellcheck source=scripts/compose/phase3-capacity-correctness-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-capacity-correctness-evidence.sh"
# shellcheck source=scripts/compose/phase3-capacity-replica-evidence.sh
. "$ROOT_DIR/scripts/compose/phase3-capacity-replica-evidence.sh"
# shellcheck source=scripts/compose/phase3-capacity-k6-search.sh
. "$ROOT_DIR/scripts/compose/phase3-capacity-k6-search.sh"

log() {
  printf '[phase3-capacity] %s\n' "$*"
}

die() {
  printf '[phase3-capacity] error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: scripts/compose/phase3-capacity.sh

Runs the Phase 3 Compose 80/20 read+booking capacity benchmark through edge-lb.

Important environment variables:
  CETS_PHASE3_URL                      Target URL, default from PHASE3_EDGE_PORT.
  CETS_PHASE3_CAPACITY_START_RPS       First RPS candidate, default 50.
  CETS_PHASE3_CAPACITY_STEP_RPS        Linear search step, default 50.
  CETS_PHASE3_CAPACITY_MAX_RPS         Search ceiling, default 1000.
  CETS_PHASE3_CAPACITY_RESOLUTION_RPS  Binary search resolution, default 25.
  CETS_PHASE3_CAPACITY_REPORT_ONLY_RPS Rebuild report for an existing candidate.
  CETS_PHASE3_CAPACITY_VERIFY_REPORT   Optional Phase 3 verify report path to link in reports.
EOF
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

compose() {
  docker compose \
    --env-file "$ENV_FILE" \
    -f "$ROOT_DIR/services/api/deploy/compose.yaml" \
    -f "$ROOT_DIR/services/api/deploy/compose.worker-isolation.yaml" \
    -f "$ROOT_DIR/services/api/deploy/compose.phase3-ha.yaml" \
    --profile phase3-ha \
    --profile worker-isolation \
    --profile observability \
    "$@"
}

seed_benchmark_employees() {
  if [ "${CETS_PHASE3_CAPACITY_SEED_EMPLOYEES:-true}" != "true" ]; then
    log "skipping benchmark employee seed"
    return
  fi
  log "seeding $EMPLOYEE_COUNT benchmark employees with prefix $EMPLOYEE_PREFIX"
  compose exec -T \
    -e EMPLOYEE_PREFIX="$EMPLOYEE_PREFIX" \
    -e EMPLOYEE_COUNT="$EMPLOYEE_COUNT" \
    postgres sh -eu -c '
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
        -c "INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
            SELECT '\''$EMPLOYEE_PREFIX'\'' || lpad(i::text, 6, '\''0'\''),
                   '\''Benchmark Employee '\'' || i,
                   '\''Engineering'\'',
                   '\''Taipei HQ'\'',
                   5,
                   '\''active'\''
            FROM generate_series(1, $EMPLOYEE_COUNT::int) AS i
            ON CONFLICT (employee_id) DO UPDATE SET
              department = EXCLUDED.department,
              site = EXCLUDED.site,
              job_grade = EXCLUDED.job_grade,
              employment_status = EXCLUDED.employment_status"
    '
}

psql_with_event_title() {
  event_title=$1
  sql_file=$2
  compose exec -T \
    -e EVENT_TITLE="$event_title" \
    postgres sh -eu -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -v event_title="$EVENT_TITLE"' <"$sql_file"
}

main() {
  case "${1:-}" in
    -h | --help)
      usage
      return
      ;;
    "")
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
  require_cmd curl
  require_cmd docker
  require_cmd jq
  validate_capacity_verify_report
  validate_capacity_inputs
  require_docker_daemon
  mkdir -p "$ARTIFACT_DIR"
  chmod 0777 "$ARTIFACT_DIR"

  PHASE3_EDGE_PORT=${PHASE3_EDGE_PORT:-$(env_value PHASE3_EDGE_PORT 18080)}
  BASE_URL=${BASE_URL:-http://127.0.0.1:${PHASE3_EDGE_PORT}}
  PROMETHEUS_PORT=${PROMETHEUS_PORT:-$(env_value PROMETHEUS_PORT 9090)}
  PROMETHEUS_URL=${PROMETHEUS_URL:-http://127.0.0.1:${PROMETHEUS_PORT}}
  PROVIDER_TOKEN_SECRET=${CETS_PHASE3_PROVIDER_TOKEN_SECRET:-$(env_value PROVIDER_TOKEN_SECRET local_dev_provider_token_secret_change_me)}
  export PROVIDER_TOKEN_SECRET

  if [ -n "$REPORT_ONLY_RPS" ]; then
    write_report "$REPORT_ONLY_RPS"
    return
  fi

  curl -fsS "$BASE_URL/readyz" >/dev/null || die "benchmark target is not ready: $BASE_URL"
  seed_benchmark_employees

  best_rps=$(find_max_rps)
  [ "$best_rps" -gt 0 ] || die "no passing RPS candidate found"
  printf '%s\n' "$best_rps" >"$ARTIFACT_DIR/max-rps-$RUN_ID.txt"
  write_report "$best_rps"
  log "highest passing RPS: $best_rps"
}

main "$@"
