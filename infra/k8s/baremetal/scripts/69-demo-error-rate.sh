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

BASE_URL=${K8S_ERROR_DEMO_BASE_URL:-http://$METALLB_INGRESS_IP}
HOST_HEADER=${K8S_ERROR_DEMO_HOST_HEADER:-$CETS_PUBLIC_HOSTNAME}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.7.1-with-browser}
SCRIPT=/k6/k8s-error-rate-demo.js
ARTIFACT_DIR=${K8S_ERROR_DEMO_ARTIFACT_DIR:-$ROOT_DIR/artifacts/k8s-error-demo}
RUN_ID=${K8S_ERROR_DEMO_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
DURATION=${K8S_ERROR_DEMO_DURATION:-120s}
TARGET_RPS=${K8S_ERROR_DEMO_TARGET_RPS:-350}
ERROR_RATIO=${K8S_ERROR_DEMO_ERROR_RATIO:-0.4}
EMPLOYEE_PREFIX=${K8S_ERROR_DEMO_EMPLOYEE_PREFIX:-KE}
EMPLOYEE_COUNT=${K8S_ERROR_DEMO_EMPLOYEE_COUNT:-10000}
HOT_EVENT_CAPACITY=${K8S_ERROR_DEMO_HOT_EVENT_CAPACITY:-$EMPLOYEE_COUNT}
K6_ENV_FILE=""

cleanup() {
  if [ -n "$K6_ENV_FILE" ]; then
    rm -f "$K6_ENV_FILE"
  fi
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
  if [ "${K8S_ERROR_DEMO_SEED_EMPLOYEES:-true}" != "true" ]; then
    log "skipping error demo employee seed"
    return
  fi
  log "seeding $EMPLOYEE_COUNT error demo employees with prefix $EMPLOYEE_PREFIX"
  kubectl_bm -n "$CETS_NAMESPACE" delete pod cets-error-demo-employee-seed --ignore-not-found >/dev/null 2>&1 || true
  kubectl_bm -n "$CETS_NAMESPACE" run cets-error-demo-employee-seed \
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
                   '\''Error Demo Employee '\'' || i,
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
    printf 'K6_RUN_ID=%s\n' "$RUN_ID"
    printf 'K6_ERROR_DEMO_DURATION=%s\n' "$DURATION"
    printf 'K6_ERROR_DEMO_RPS=%s\n' "$TARGET_RPS"
    printf 'K6_ERROR_DEMO_ERROR_RATIO=%s\n' "$ERROR_RATIO"
    printf 'K6_EMPLOYEE_PREFIX=%s\n' "$EMPLOYEE_PREFIX"
    printf 'K6_EMPLOYEE_COUNT=%s\n' "$EMPLOYEE_COUNT"
    printf 'K6_ERROR_DEMO_HOT_EVENT_CAPACITY=%s\n' "$HOT_EVENT_CAPACITY"
  } >"$K6_ENV_FILE"
}

run_k6_demo() {
  local out="$ARTIFACT_DIR/k6-$RUN_ID.json"
  log "running error-rate demo target_rps=$TARGET_RPS duration=$DURATION error_ratio=$ERROR_RATIO base=$BASE_URL host=$HOST_HEADER"
  docker_cmd run --rm --network host \
    -v "$ROOT_DIR/k6:/k6:ro" \
    -v "$ARTIFACT_DIR:/artifacts" \
    --env-file "$K6_ENV_FILE" \
    "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$out")" "$SCRIPT"
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
      "expected_errors=" + (($m.k8s_error_demo_expected_errors.count // 0) | tostring),
      "valid_responses=" + (($m.k8s_error_demo_valid_responses.count // 0) | tostring),
      "unexpected_responses=" + (($m.k8s_error_demo_unexpected_responses.count // 0) | tostring),
      "backend_replica_hits=" + (($m.k8s_error_demo_backend_replica_hits.count // 0) | tostring)
    ] | .[]
  ' "$summary"
}

main() {
  mkdir -p "$ARTIFACT_DIR"
  chmod 0777 "$ARTIFACT_DIR"
  curl -fsS -H "Host: $HOST_HEADER" "$BASE_URL/readyz" >/dev/null ||
    die "error demo target is not ready: $BASE_URL host=$HOST_HEADER"
  seed_demo_employees
  write_env_file
  run_k6_demo
  print_summary
}

main "$@"
