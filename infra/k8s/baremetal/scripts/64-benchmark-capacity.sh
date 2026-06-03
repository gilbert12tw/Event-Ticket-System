#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd base64
require_cmd curl
require_cmd docker
require_cmd jq
require_cmd kubectl

BASE_URL=${K8S_BENCH_BASE_URL:-http://$METALLB_INGRESS_IP}
HOST_HEADER=${K8S_BENCH_HOST_HEADER:-$CETS_PUBLIC_HOSTNAME}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.7.1-with-browser}
SCRIPT=/k6/k8s-capacity-rps.js
ARTIFACT_DIR=${K8S_BENCH_ARTIFACT_DIR:-$ROOT_DIR/artifacts/k8s-capacity}
RUN_ID=${K8S_BENCH_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
DURATION=${K8S_BENCH_DURATION:-82s}
READ_RATIO=${K8S_BENCH_READ_RATIO:-0.8}
START_RPS=${K8S_BENCH_START_RPS:-100}
STEP_RPS=${K8S_BENCH_STEP_RPS:-100}
MAX_RPS=${K8S_BENCH_MAX_RPS:-2000}
RESOLUTION_RPS=${K8S_BENCH_RESOLUTION_RPS:-25}
EMPLOYEE_PREFIX=${K8S_BENCH_EMPLOYEE_PREFIX:-KB}
EMPLOYEE_COUNT=${K8S_BENCH_EMPLOYEE_COUNT:-10000}
HOT_EVENT_CAPACITY=${K8S_BENCH_HOT_EVENT_CAPACITY:-$EMPLOYEE_COUNT}
PROM_PORT=${K8S_BENCH_PROM_PORT:-19090}
REPORT_ONLY_RPS=${K8S_BENCH_REPORT_ONLY_RPS:-}

PROM_PID=""
K6_ENV_FILES=()

cleanup() {
  if [ -n "$PROM_PID" ]; then
    kill "$PROM_PID" >/dev/null 2>&1 || true
    wait "$PROM_PID" >/dev/null 2>&1 || true
  fi
  for env_file in "${K6_ENV_FILES[@]}"; do
    rm -f "$env_file"
  done
}
trap cleanup EXIT

provider_secret() {
  kubectl_bm -n "$CETS_NAMESPACE" get secret cets-runtime-env -o jsonpath='{.data.PROVIDER_TOKEN_SECRET}' | base64 -d
}

postgres_password() {
  kubectl_bm -n "$CETS_NAMESPACE" get secret cets-runtime-env -o jsonpath='{.data.DATABASE_URL}' |
    base64 -d |
    sed -n 's#postgresql://[^:]*:\([^@]*\)@.*#\1#p'
}

seed_benchmark_employees() {
  if [ "${K8S_BENCH_SEED_EMPLOYEES:-true}" != "true" ]; then
    log "skipping benchmark employee seed"
    return
  fi
  log "seeding $EMPLOYEE_COUNT benchmark employees with prefix $EMPLOYEE_PREFIX"
  kubectl_bm -n "$CETS_NAMESPACE" delete pod cets-benchmark-employee-seed --ignore-not-found >/dev/null 2>&1 || true
  kubectl_bm -n "$CETS_NAMESPACE" run cets-benchmark-employee-seed \
    --rm -i \
    --restart=Never \
    --image=postgres:16 \
    --env="PGPASSWORD=$(postgres_password)" \
    --env="POSTGRES_USER=${POSTGRES_USER:-cets}" \
    --env="POSTGRES_DB=${POSTGRES_DB:-cets}" \
    --env="EMPLOYEE_PREFIX=$EMPLOYEE_PREFIX" \
    --env="EMPLOYEE_COUNT=$EMPLOYEE_COUNT" \
    --command -- sh -eu -c '
      psql -h cets-postgres-rw -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
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
  local sql=$1
  kubectl_bm -n "$CETS_NAMESPACE" delete pod cets-benchmark-psql --ignore-not-found >/dev/null 2>&1 || true
  kubectl_bm -n "$CETS_NAMESPACE" run cets-benchmark-psql \
    --rm -i \
    --restart=Never \
    --image=postgres:16 \
    --env="PGPASSWORD=$(postgres_password)" \
    --env="POSTGRES_USER=${POSTGRES_USER:-cets}" \
    --env="POSTGRES_DB=${POSTGRES_DB:-cets}" \
    --env="SQL=$sql" \
    --command -- sh -eu -c '
      psql -h cets-postgres-rw -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "$SQL"
    '
}

run_k6_candidate() {
  local rps=$1
  local out="$ARTIFACT_DIR/k6-$RUN_ID-rps-$rps.json"
  local env_file
  local status
  log "running k6 candidate rps=$rps duration=$DURATION base=$BASE_URL host=$HOST_HEADER"
  env_file=$(mktemp "$ARTIFACT_DIR/k6-env-$RUN_ID-rps-$rps.XXXXXX")
  K6_ENV_FILES+=("$env_file")
  chmod 0600 "$env_file"
  {
    printf 'BASE_URL=%s\n' "$BASE_URL"
    printf 'K6_HOST_HEADER=%s\n' "$HOST_HEADER"
    printf 'K6_PROVIDER_TOKEN_SECRET=%s\n' "$PROVIDER_TOKEN_SECRET"
    printf 'K6_TARGET_RPS=%s\n' "$rps"
    printf 'K6_CAPACITY_DURATION=%s\n' "$DURATION"
    printf 'K6_READ_RATIO=%s\n' "$READ_RATIO"
    printf 'K6_EMPLOYEE_PREFIX=%s\n' "$EMPLOYEE_PREFIX"
    printf 'K6_EMPLOYEE_COUNT=%s\n' "$EMPLOYEE_COUNT"
    printf 'K6_HOT_EVENT_CAPACITY=%s\n' "$HOT_EVENT_CAPACITY"
    printf 'K6_RUN_ID=%s\n' "$RUN_ID-rps-$rps"
  } >"$env_file"

  set +e
  docker_cmd run --rm --network host \
    -v "$ROOT_DIR/k6:/k6:ro" \
    -v "$ARTIFACT_DIR:/artifacts" \
    --env-file "$env_file" \
    "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$out")" "$SCRIPT" >&2
  status=$?
  set -e
  rm -f "$env_file"
  return "$status"
}

docker_cmd() {
  if docker info >/dev/null 2>&1; then
    docker "$@"
  elif [ -n "${BAREMETAL_BECOME_PASSWORD:-}" ]; then
    printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | sudo -S docker "$@"
  else
    sudo -n docker "$@"
  fi
}

candidate_passes() {
  local rps=$1
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
  local low=0
  local high=0
  local current=$START_RPS

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

start_prometheus_port_forward() {
  log "opening Prometheus port-forward on 127.0.0.1:$PROM_PORT"
  kubectl_bm -n observability port-forward --address 127.0.0.1 svc/kube-prometheus-stack-prometheus "$PROM_PORT:9090" \
    >"$ARTIFACT_DIR/prometheus-port-forward.log" 2>&1 &
  PROM_PID=$!
  for _ in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:$PROM_PORT/-/ready" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  die "Prometheus port-forward did not become ready"
}

prom_query() {
  local query=$1
  local output=$2
  curl -fsS --get --data-urlencode "query=$query" "http://127.0.0.1:$PROM_PORT/api/v1/query" >"$output"
}

require_prometheus_vector_sample() {
  local output=$1
  local label=$2
  jq -e '(.data.result // []) | length > 0' "$output" >/dev/null ||
    die "benchmark did not produce $label samples in Prometheus"
}

prom_scalar() {
  local query=$1
  curl -fsS --get --data-urlencode "query=$query" "http://127.0.0.1:$PROM_PORT/api/v1/query" |
    jq -r 'if .data.resultType == "scalar" then .data.result[1] else (.data.result[0].value[1] // empty) end'
}

require_prometheus_evidence() {
  local replicas
  local cpu_samples
  replicas=$(prom_scalar 'scalar(count(count by (instance) (increase(cets_http_requests_total[15m]) > 0)))')
  cpu_samples=$(prom_scalar 'sum(rate(container_cpu_usage_seconds_total{namespace="cets",pod=~"backend-.*",container!="POD",container!=""}[5m]))')
  awk -v value="${replicas:-0}" 'BEGIN { exit(value >= 3 ? 0 : 1) }' ||
    die "benchmark did not produce traffic on all 3 backend instances; observed $replicas"
  awk -v value="${cpu_samples:-0}" 'BEGIN { exit(value > 0 ? 0 : 1) }' ||
    die "benchmark did not produce backend CPU samples in Prometheus"
}

require_capacity_invariant() {
  local best_rps=$1
  local event_title="k8s capacity $RUN_ID-rps-$best_rps"
  local result
  result=$(psql_once "SELECT COALESCE(e.capacity, 0) || ',' || count(r.registration_id) FROM events e LEFT JOIN registrations r ON r.event_id = e.event_id AND r.status = 'confirmed' WHERE e.title = '$event_title' GROUP BY e.capacity" | tail -n 1)
  [ -n "$result" ] || die "could not find benchmark event '$event_title' for capacity check"
  capacity=${result%,*}
  confirmed=${result#*,}
  awk -v confirmed="$confirmed" -v capacity="$capacity" 'BEGIN { exit(confirmed <= capacity ? 0 : 1) }' ||
    die "confirmed bookings exceeded event capacity for '$event_title': confirmed=$confirmed capacity=$capacity"
}

write_report() {
  local best_rps=$1
  local summary="$ARTIFACT_DIR/k6-$RUN_ID-rps-$best_rps.json"
  local report="$ARTIFACT_DIR/capacity-report-$RUN_ID.md"
  local prom_cpu="$ARTIFACT_DIR/prometheus-cpu-$RUN_ID.json"
  local prom_memory="$ARTIFACT_DIR/prometheus-memory-$RUN_ID.json"
  local prom_restarts="$ARTIFACT_DIR/prometheus-restarts-$RUN_ID.json"
  local prom_red="$ARTIFACT_DIR/prometheus-red-$RUN_ID.json"
  local prom_throttling="$ARTIFACT_DIR/prometheus-throttling-$RUN_ID.json"

  require_capacity_invariant "$best_rps"
  start_prometheus_port_forward
  require_prometheus_evidence
  prom_query 'sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="cets",pod=~"backend-.*",container!="POD",container!=""}[5m]))' "$prom_cpu" || true
  require_prometheus_vector_sample "$prom_cpu" "backend CPU"
  prom_query 'sum by (pod) (container_memory_working_set_bytes{namespace="cets",pod=~"backend-.*",container!="POD",container!=""})' "$prom_memory"
  require_prometheus_vector_sample "$prom_memory" "backend memory"
  prom_query 'sum by (pod) (kube_pod_container_status_restarts_total{namespace="cets",pod=~"backend-.*"})' "$prom_restarts"
  require_prometheus_vector_sample "$prom_restarts" "backend restart"
  prom_query 'sum by (route,method,status_class) (rate(cets_http_requests_total[5m]))' "$prom_red" || true
  prom_query 'sum by (pod) (rate(container_cpu_cfs_throttled_seconds_total{namespace="cets",pod=~"backend-.*",container!="POD",container!=""}[5m]))' "$prom_throttling" || true

  {
    printf '# K8s Capacity Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Base URL | `%s` |\n' "$BASE_URL"
    printf '| Host header | `%s` |\n' "$HOST_HEADER"
    printf '| Highest passing RPS | `%s` |\n' "$best_rps"
    printf '| Duration | `%s` |\n' "$DURATION"
    printf '| Traffic mix | `%s read / %s booking` |\n' "$READ_RATIO" "$(awk -v r="$READ_RATIO" 'BEGIN { printf "%.2f", 1-r }')"
    printf '| Employee fixture | `%s%s` employees, prefix `%s` |\n' "$EMPLOYEE_COUNT" "" "$EMPLOYEE_PREFIX"
    printf '| k6 summary | `%s` |\n' "$summary"
    printf '| Prometheus CPU sample | `%s` |\n' "$prom_cpu"
    printf '| Prometheus memory sample | `%s` |\n' "$prom_memory"
    printf '| Prometheus restart sample | `%s` |\n' "$prom_restarts"
    printf '| Prometheus RED sample | `%s` |\n' "$prom_red"
    printf '| Prometheus throttling sample | `%s` |\n' "$prom_throttling"
    if [ -f "$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt" ]; then
      printf '| Highest failing RPS tested | `%s` |\n' "$(cat "$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt")"
    fi
    printf '\n'
    printf '## k6 Metrics\n\n'
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
    printf '\n## Backend CPU Samples\n\n'
    jq -r '(.data.result // [])[] | "- \(.metric.pod // .metric.instance // "unknown"): `\(.value[1])`"' "$prom_cpu"
    printf '\n## Backend Memory Samples\n\n'
    jq -r '(.data.result // [])[] | "- \(.metric.pod // .metric.instance // "unknown"): `\(.value[1])` bytes"' "$prom_memory"
    printf '\n## Backend Restart Samples\n\n'
    jq -r '(.data.result // [])[] | "- \(.metric.pod // .metric.instance // "unknown"): `\(.value[1])` restarts"' "$prom_restarts"
    printf '\n## Backend CPU Throttling Samples\n\n'
    if jq -e '.status == "success" and ((.data.result // []) | length > 0)' "$prom_throttling" >/dev/null; then
      jq -r '(.data.result // [])[] | "- \(.metric.pod // .metric.instance // "unknown"): `\(.value[1])` throttled CPU seconds/sec"' "$prom_throttling"
    elif jq -e '.status == "success" and ((.data.result // []) | length == 0)' "$prom_throttling" >/dev/null; then
      printf 'No backend throttling samples were returned by Prometheus for this query.\n'
    else
      printf 'Prometheus throttling query did not return a successful response; inspect `%s`.\n' "$prom_throttling"
    fi
    printf '\n\n'
    printf '## Follow-Up Evidence\n\n'
    printf 'Run `infra/k8s/baremetal/scripts/66-verify-observability.sh` after this benchmark to validate trace-to-log, trace-to-profile, and service graph data for the load window.\n'
  } >"$report"
  log "wrote capacity report: $report"
}

main() {
  mkdir -p "$ARTIFACT_DIR"
  chmod 0777 "$ARTIFACT_DIR"
  PROVIDER_TOKEN_SECRET=$(provider_secret)

  if [ -n "$REPORT_ONLY_RPS" ]; then
    write_report "$REPORT_ONLY_RPS"
    log "rewrote capacity report for rps=$REPORT_ONLY_RPS"
    return
  fi

  curl -fsS -H "Host: $HOST_HEADER" "$BASE_URL/readyz" >/dev/null ||
    die "benchmark target is not ready: $BASE_URL with Host=$HOST_HEADER"
  seed_benchmark_employees

  best_rps=$(find_max_rps)
  [ "$best_rps" -gt 0 ] || die "no passing RPS candidate found"
  printf '%s\n' "$best_rps" >"$ARTIFACT_DIR/max-rps-$RUN_ID.txt"
  write_report "$best_rps"
  log "highest passing RPS: $best_rps"
}

main "$@"
