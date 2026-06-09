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

BASE_URL=${K8S_LGTM_BASE_URL:-http://$METALLB_INGRESS_IP}
HOST_HEADER=${K8S_LGTM_HOST_HEADER:-$CETS_PUBLIC_HOSTNAME}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.7.1-with-browser}
SCRIPT=/k6/k6-lgtm-debug-1000rps.js
ARTIFACT_DIR=${K8S_LGTM_ARTIFACT_DIR:-$ROOT_DIR/artifacts/k8s-lgtm-debug}
RUN_ID=${K8S_LGTM_RUN_ID:-lgtm-$(date -u +%Y%m%d%H%M%S)}
DURATION=${K8S_LGTM_DURATION:-120s}
READ_RPS=${K8S_LGTM_READ_RPS:-600}
BOOKING_RPS=${K8S_LGTM_BOOKING_RPS:-250}
INVALID_BOOKING_RPS=${K8S_LGTM_INVALID_BOOKING_RPS:-90}
UNAUTHORIZED_RPS=${K8S_LGTM_UNAUTHORIZED_RPS:-30}
MISSING_EVENT_RPS=${K8S_LGTM_MISSING_EVENT_RPS:-30}
EMPLOYEE_PREFIX=${K8S_LGTM_EMPLOYEE_PREFIX:-LD}
EMPLOYEE_COUNT=${K8S_LGTM_EMPLOYEE_COUNT:-10000}
HOT_EVENT_CAPACITY=${K8S_LGTM_HOT_EVENT_CAPACITY:-500}
WITH_FAILURE_DRILL=false
FAILURE_DRILL_DELAY=${K8S_LGTM_FAILURE_DRILL_DELAY:-30}
K6_ENV_FILE=""
K6_PID=""

for arg in "$@"; do
  case "$arg" in
    --with-failure-drill) WITH_FAILURE_DRILL=true ;;
    *) die "unknown argument: $arg" ;;
  esac
done

cleanup() {
  if [ -n "$K6_PID" ] && kill -0 "$K6_PID" 2>/dev/null; then
    wait "$K6_PID" 2>/dev/null || true
  fi
  if [ -n "$K6_ENV_FILE" ]; then
    rm -f "$K6_ENV_FILE"
  fi
}
trap cleanup EXIT

provider_secret() {
  kubectl_bm -n "$CETS_NAMESPACE" get secret cets-runtime-env \
    -o jsonpath='{.data.PROVIDER_TOKEN_SECRET}' | base64 -d
}

postgres_password() {
  kubectl_bm -n "$CETS_NAMESPACE" get secret cets-runtime-env \
    -o jsonpath='{.data.DATABASE_URL}' |
    base64 -d |
    sed -n 's#postgresql://[^:]*:\([^@]*\)@.*#\1#p'
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

seed_demo_employees() {
  if [ "${K8S_LGTM_SEED_EMPLOYEES:-true}" != "true" ]; then
    log "skipping LGTM demo employee seed"
    return
  fi
  log "seeding $EMPLOYEE_COUNT LGTM demo employees with prefix $EMPLOYEE_PREFIX"
  kubectl_bm -n "$CETS_NAMESPACE" delete pod cets-lgtm-demo-employee-seed \
    --ignore-not-found >/dev/null 2>&1 || true
  kubectl_bm -n "$CETS_NAMESPACE" run cets-lgtm-demo-employee-seed \
    --rm -i \
    --quiet \
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
                   '\''LGTM Demo Employee '\'' || i,
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

write_env_file() {
  K6_ENV_FILE=$(mktemp "$ARTIFACT_DIR/k6-env-$RUN_ID.XXXXXX")
  chmod 0600 "$K6_ENV_FILE"
  {
    printf 'BASE_URL=%s\n' "$BASE_URL"
    printf 'K6_HOST_HEADER=%s\n' "$HOST_HEADER"
    printf 'K6_PROVIDER_TOKEN_SECRET=%s\n' "$(provider_secret)"
    printf 'K6_USE_MOCK_PROVIDER=false\n'
    printf 'K6_RUN_ID=%s\n' "$RUN_ID"
    printf 'LGTM_DURATION=%s\n' "$DURATION"
    printf 'K6_READ_RPS=%s\n' "$READ_RPS"
    printf 'K6_BOOKING_RPS=%s\n' "$BOOKING_RPS"
    printf 'K6_INVALID_BOOKING_RPS=%s\n' "$INVALID_BOOKING_RPS"
    printf 'K6_UNAUTHORIZED_RPS=%s\n' "$UNAUTHORIZED_RPS"
    printf 'K6_MISSING_EVENT_RPS=%s\n' "$MISSING_EVENT_RPS"
    printf 'K6_HOT_EVENT_CAPACITY=%s\n' "$HOT_EVENT_CAPACITY"
    printf 'K6_EMPLOYEE_COUNT=%s\n' "$EMPLOYEE_COUNT"
    printf 'K6_EMPLOYEE_PREFIX=%s\n' "$EMPLOYEE_PREFIX"
  } >"$K6_ENV_FILE"
}

run_k6() {
  local out="$ARTIFACT_DIR/k6-$RUN_ID.json"
  log "running LGTM debug demo rps=$((READ_RPS + BOOKING_RPS + INVALID_BOOKING_RPS + UNAUTHORIZED_RPS + MISSING_EVENT_RPS)) duration=$DURATION base=$BASE_URL host=$HOST_HEADER"
  docker_cmd run --rm --network host \
    -v "$ROOT_DIR/k6:/k6:ro" \
    -v "$ARTIFACT_DIR:/artifacts" \
    --env-file "$K6_ENV_FILE" \
    "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$out")" "$SCRIPT"
}

run_failure_drill() {
  log "failure drill: waiting ${FAILURE_DRILL_DELAY}s for traffic to warm up"
  sleep "$FAILURE_DRILL_DELAY"

  local target_pod
  target_pod=$(kubectl_bm -n "$CETS_NAMESPACE" get pods -l app=backend \
    --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)

  if [ -z "$target_pod" ]; then
    log "failure drill: no running backend pod found, skipping"
    return
  fi

  log "failure drill: killing pod $target_pod to generate 5xx"
  kubectl_bm -n "$CETS_NAMESPACE" delete pod "$target_pod" --wait=false
  log "failure drill: pod $target_pod deleted, ingress will return 502 until replacement is ready"
}

print_summary() {
  local summary="$ARTIFACT_DIR/k6-$RUN_ID.json"
  [ -f "$summary" ] || die "k6 summary was not written: $summary"
  jq -r --arg summary "$summary" '
    .metrics as $m |
    [
      "summary=" + $summary,
      "http_reqs=" + (($m.http_reqs.count // 0) | tostring),
      "http_req_failed=" + (($m.http_req_failed.value // 0) | tostring),
      "expected_errors=" + (($m.lgtm_expected_errors.count // 0) | tostring),
      "valid_responses=" + (($m.lgtm_valid_responses.count // 0) | tostring),
      "unexpected_responses=" + (($m.lgtm_unexpected_responses.count // 0) | tostring),
      "backend_replica_hits=" + (($m.lgtm_backend_replica_hits.count // 0) | tostring)
    ] | .[]
  ' "$summary"
}

main() {
  mkdir -p "$ARTIFACT_DIR"
  chmod 0777 "$ARTIFACT_DIR"
  curl -fsS -H "Host: $HOST_HEADER" "$BASE_URL/readyz" >/dev/null ||
    die "LGTM demo target is not ready: $BASE_URL host=$HOST_HEADER"
  seed_demo_employees
  write_env_file

  if [ "$WITH_FAILURE_DRILL" = "true" ]; then
    run_k6 &
    K6_PID=$!
    run_failure_drill
    wait "$K6_PID"
    K6_PID=""
  else
    run_k6
  fi

  print_summary
}

main "$@"
