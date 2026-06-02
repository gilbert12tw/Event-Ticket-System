#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
PHASE3_EDGE_PORT=${PHASE3_EDGE_PORT:-}
BASE_URL=${CETS_PHASE3_URL:-}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.7.1-with-browser}
SCRIPT=/k6/k8s-capacity-rps.js
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
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
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

psql_once() {
  sql=$1
  compose exec -T \
    -e SQL="$sql" \
    postgres sh -eu -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "$SQL"'
}

run_k6_candidate() {
  rps=$1
  out="$ARTIFACT_DIR/k6-$RUN_ID-rps-$rps.json"
  log "running k6 candidate rps=$rps duration=$DURATION base=$BASE_URL"
  docker run --rm --network host \
    -v "$ROOT_DIR/k6:/k6:ro" \
    -v "$ARTIFACT_DIR:/artifacts" \
    -e BASE_URL="$BASE_URL" \
    -e K6_PROVIDER_TOKEN_SECRET="$PROVIDER_TOKEN_SECRET" \
    -e K6_TARGET_RPS="$rps" \
    -e K6_CAPACITY_DURATION="$DURATION" \
    -e K6_READ_RATIO="$READ_RATIO" \
    -e K6_EMPLOYEE_PREFIX="$EMPLOYEE_PREFIX" \
    -e K6_EMPLOYEE_COUNT="$EMPLOYEE_COUNT" \
    -e K6_HOT_EVENT_CAPACITY="$HOT_EVENT_CAPACITY" \
    -e K6_RUN_ID="$RUN_ID-rps-$rps" \
    "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$out")" "$SCRIPT" >&2
}

candidate_passes() {
  rps=$1
  if run_k6_candidate "$rps"; then
    printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-pass-rps.txt"
    printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-pass-rps-$RUN_ID.txt"
    return 0
  fi
  printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-fail-rps.txt"
  printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt"
  return 1
}

find_max_rps() {
  low=0
  high=0
  current=$START_RPS

  while [ "$current" -le "$MAX_RPS" ]; do
    if candidate_passes "$current"; then
      low=$current
      current=$((current + STEP_RPS))
    else
      high=$current
      break
    fi
  done

  if [ "$high" -eq 0 ]; then
    printf '%s\n' "$low"
    return
  fi

  while [ $((high - low)) -gt "$RESOLUTION_RPS" ]; do
    mid=$(((low + high) / 2))
    if candidate_passes "$mid"; then
      low=$mid
    else
      high=$mid
    fi
  done
  printf '%s\n' "$low"
}

prom_query() {
  query=$1
  output=$2
  curl -fsS --get --data-urlencode "query=$query" "$PROMETHEUS_URL/api/v1/query" >"$output"
}

prom_scalar() {
  query=$1
  curl -fsS --get --data-urlencode "query=$query" "$PROMETHEUS_URL/api/v1/query" |
    jq -r 'if .data.resultType == "scalar" then .data.result[1] else (.data.result[0].value[1] // empty) end'
}

require_prometheus_evidence() {
  replicas=$(prom_scalar 'scalar(count(count by (instance) (increase(cets_http_requests_total[15m]) > 0)))')
  booking_samples=$(prom_scalar 'sum(increase(cets_booking_stage_seconds_count[15m]))')
  awk -v value="${replicas:-0}" 'BEGIN { exit(value >= 3 ? 0 : 1) }' ||
    die "benchmark did not produce traffic on all 3 backend instances; observed $replicas"
  awk -v value="${booking_samples:-0}" 'BEGIN { exit(value > 0 ? 0 : 1) }' ||
    die "benchmark did not produce booking stage metrics in Prometheus"
}

require_capacity_invariant() {
  best_rps=$1
  event_title="k8s capacity $RUN_ID-rps-$best_rps"
  result=$(psql_once "SELECT COALESCE(e.capacity, 0) || ',' || count(r.registration_id) FROM events e LEFT JOIN registrations r ON r.event_id = e.event_id AND r.status = 'confirmed' WHERE e.title = '$event_title' GROUP BY e.capacity" | tail -n 1)
  [ -n "$result" ] || die "could not find benchmark event '$event_title' for capacity check"
  capacity=${result%,*}
  confirmed=${result#*,}
  awk -v confirmed="$confirmed" -v capacity="$capacity" 'BEGIN { exit(confirmed <= capacity ? 0 : 1) }' ||
    die "confirmed bookings exceeded event capacity for '$event_title': confirmed=$confirmed capacity=$capacity"
}

write_report() {
  best_rps=$1
  summary="$ARTIFACT_DIR/k6-$RUN_ID-rps-$best_rps.json"
  report="$ARTIFACT_DIR/capacity-report-$RUN_ID.md"
  prom_red="$ARTIFACT_DIR/prometheus-red-$RUN_ID.json"
  prom_booking="$ARTIFACT_DIR/prometheus-booking-stages-$RUN_ID.json"
  prom_reservation="$ARTIFACT_DIR/prometheus-reservation-$RUN_ID.json"

  require_capacity_invariant "$best_rps"
  require_prometheus_evidence
  prom_query 'sum by (route,method,status_class,instance) (rate(cets_http_requests_total[5m]))' "$prom_red" || true
  prom_query 'histogram_quantile(0.95, sum by (stage,outcome,le) (rate(cets_booking_stage_seconds_bucket[5m])))' "$prom_booking" || true
  prom_query 'sum by (outcome,capacity_type,outage_mode) (increase(cets_reservation_attempt_total[15m]))' "$prom_reservation" || true

  {
    printf '# Phase3 Capacity Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Base URL | `%s` |\n' "$BASE_URL"
    printf '| Highest passing RPS | `%s` |\n' "$best_rps"
    printf '| Duration | `%s` |\n' "$DURATION"
    printf '| Traffic mix | `%s read / %s booking` |\n' "$READ_RATIO" "$(awk -v r="$READ_RATIO" 'BEGIN { printf "%.2f", 1-r }')"
    printf '| Employee fixture | `%s` employees, prefix `%s` |\n' "$EMPLOYEE_COUNT" "$EMPLOYEE_PREFIX"
    printf '| Hot event capacity | `%s` |\n' "$HOT_EVENT_CAPACITY"
    printf '| k6 summary | `%s` |\n' "$summary"
    printf '| Prometheus RED sample | `%s` |\n' "$prom_red"
    printf '| Prometheus booking stage sample | `%s` |\n' "$prom_booking"
    printf '| Prometheus reservation sample | `%s` |\n' "$prom_reservation"
    if [ -f "$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt" ]; then
      printf '| Highest failing RPS tested | `%s` |\n' "$(cat "$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt")"
    fi
    printf '\n## k6 Metrics\n\n'
    jq -r '
      def metric_value($name; $field):
        (.metrics[$name][$field] // .metrics[$name].percentiles[$field] // "n/a");
      [
        ["http_req_duration p95", metric_value("http_req_duration"; "p(95)")],
        ["read flow p95", metric_value("http_req_duration{flow:read}"; "p(95)")],
        ["read flow p99", metric_value("http_req_duration{flow:read}"; "p(99)")],
        ["booking p95", metric_value("k8s_booking_duration"; "p(95)")],
        ["booking p99", metric_value("k8s_booking_duration"; "p(99)")],
        ["booking attempts", (.metrics.k8s_booking_attempts.count // "n/a")],
        ["booking confirmed", (.metrics.k8s_booking_confirmed.count // "n/a")],
        ["booking waitlisted", (.metrics.k8s_booking_waitlisted.count // "n/a")]
      ] | .[] | "- \(.[0]): `\(.[1])`"
    ' "$summary"
  } >"$report"
  log "wrote capacity report: $report"
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
