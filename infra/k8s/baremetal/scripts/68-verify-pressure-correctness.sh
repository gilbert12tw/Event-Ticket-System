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

BASE_URL=${K8S_CORRECTNESS_BASE_URL:-http://$METALLB_INGRESS_IP}
HOST_HEADER=${K8S_CORRECTNESS_HOST_HEADER:-$CETS_PUBLIC_HOSTNAME}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.7.1-with-browser}
SCRIPT=/k6/k8s-correctness-pressure.js
ARTIFACT_DIR=${K8S_CORRECTNESS_ARTIFACT_DIR:-$ROOT_DIR/artifacts/k8s-correctness}
RUN_ID=${K8S_CORRECTNESS_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
DURATION=${K8S_CORRECTNESS_DURATION:-30s}
EMPLOYEE_PREFIX=${K8S_CORRECTNESS_EMPLOYEE_PREFIX:-KC}
EMPLOYEE_COUNT=${K8S_CORRECTNESS_EMPLOYEE_COUNT:-10000}
BOOKING_CAPACITY=${K8S_CORRECTNESS_BOOKING_CAPACITY:-100}
CHECKIN_TICKETS=${K8S_CORRECTNESS_CHECKIN_TICKETS:-24}
OFFLINE_BATCHES=${K8S_CORRECTNESS_OFFLINE_BATCHES:-8}
LOG_SINCE=${K8S_CORRECTNESS_LOG_SINCE:-30m}
BOOKING_RPS=${K8S_CORRECTNESS_BOOKING_RPS:-100}
ONLINE_RPS=${K8S_CORRECTNESS_ONLINE_RPS:-70}
OFFLINE_RPS=${K8S_CORRECTNESS_OFFLINE_RPS:-5}
MIXED_ONLINE_RPS=${K8S_CORRECTNESS_MIXED_ONLINE_RPS:-50}
MIXED_OFFLINE_RPS=${K8S_CORRECTNESS_MIXED_OFFLINE_RPS:-5}
INVALID_RPS=${K8S_CORRECTNESS_INVALID_RPS:-20}

SQL_REPORT="$ARTIFACT_DIR/sql-invariants-$RUN_ID.tsv"
LOG_REPLICAS="$ARTIFACT_DIR/log-replicas-$RUN_ID.txt"
EVENT_IDS_FILE="$ARTIFACT_DIR/event-ids-$RUN_ID.tsv"

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

seed_correctness_employees() {
  log "seeding $EMPLOYEE_COUNT correctness employees with prefix $EMPLOYEE_PREFIX"
  kubectl_bm -n "$CETS_NAMESPACE" delete pod cets-correctness-employee-seed --ignore-not-found >/dev/null 2>&1 || true
  kubectl_bm -n "$CETS_NAMESPACE" run cets-correctness-employee-seed \
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
                   '\''Correctness Employee '\'' || i,
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
  kubectl_bm -n "$CETS_NAMESPACE" delete pod cets-correctness-psql --ignore-not-found >/dev/null 2>&1 || true
  kubectl_bm -n "$CETS_NAMESPACE" run cets-correctness-psql \
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

run_k6_correctness() {
  local out="$ARTIFACT_DIR/k6-$RUN_ID.json"
  log "running k6 pressure correctness duration=$DURATION base=$BASE_URL host=$HOST_HEADER"
  docker_cmd run --rm --network host \
    -v "$ROOT_DIR/k6:/k6:ro" \
    -v "$ARTIFACT_DIR:/artifacts" \
    -e BASE_URL="$BASE_URL" \
    -e K6_HOST_HEADER="$HOST_HEADER" \
    -e K6_PROVIDER_TOKEN_SECRET="$PROVIDER_TOKEN_SECRET" \
    -e K6_RUN_ID="$RUN_ID" \
    -e K6_CORRECTNESS_DURATION="$DURATION" \
    -e K6_EMPLOYEE_PREFIX="$EMPLOYEE_PREFIX" \
    -e K6_CORRECTNESS_BOOKING_EMPLOYEES="$EMPLOYEE_COUNT" \
    -e K6_CORRECTNESS_BOOKING_CAPACITY="$BOOKING_CAPACITY" \
    -e K6_CORRECTNESS_CHECKIN_TICKETS="$CHECKIN_TICKETS" \
    -e K6_CORRECTNESS_OFFLINE_BATCHES="$OFFLINE_BATCHES" \
    -e K6_CORRECTNESS_BOOKING_RPS="$BOOKING_RPS" \
    -e K6_CORRECTNESS_ONLINE_RPS="$ONLINE_RPS" \
    -e K6_CORRECTNESS_OFFLINE_RPS="$OFFLINE_RPS" \
    -e K6_CORRECTNESS_MIXED_ONLINE_RPS="$MIXED_ONLINE_RPS" \
    -e K6_CORRECTNESS_MIXED_OFFLINE_RPS="$MIXED_OFFLINE_RPS" \
    -e K6_CORRECTNESS_INVALID_RPS="$INVALID_RPS" \
    "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$out")" "$SCRIPT"
}

load_event_ids() {
  log "loading exact event ids for run $RUN_ID"
  local sql
  sql="SELECT replace(title, 'k8s correctness $RUN_ID ', '') AS kind, event_id
       FROM events
       WHERE title IN (
         'k8s correctness $RUN_ID booking',
         'k8s correctness $RUN_ID online',
         'k8s correctness $RUN_ID offline',
         'k8s correctness $RUN_ID mixed'
       )
       ORDER BY kind"
  psql_once "$sql" >"$EVENT_IDS_FILE"
  local count
  count=$(wc -l <"$EVENT_IDS_FILE" | tr -d ' ')
  [ "$count" = "4" ] ||
    die "expected exactly 4 correctness events for run $RUN_ID, found $count; refusing title-scoped SQL"
  for kind in booking online offline mixed; do
    local kind_count
    kind_count=$(awk -F '|' -v kind="$kind" '$1 == kind { count += 1 } END { print count + 0 }' "$EVENT_IDS_FILE")
    [ "$kind_count" = "1" ] ||
      die "expected exactly one $kind event for run $RUN_ID, found $kind_count"
  done
}

event_id_for() {
  local kind=$1
  awk -F '|' -v kind="$kind" '$1 == kind { print $2 }' "$EVENT_IDS_FILE"
}

target_events_cte() {
  local booking online offline mixed
  booking=$(event_id_for booking)
  online=$(event_id_for online)
  offline=$(event_id_for offline)
  mixed=$(event_id_for mixed)
  printf "WITH target_event_ids(event_id, kind) AS (VALUES ('%s','booking'),('%s','online'),('%s','offline'),('%s','mixed')), target_events AS (SELECT e.event_id, e.capacity, e.title, te.kind FROM events e JOIN target_event_ids te ON te.event_id = e.event_id)" "$booking" "$online" "$offline" "$mixed"
}

append_result() {
  local name=$1
  local expected=$2
  local actual=$3
  local status=$4
  printf '%s\t%s\t%s\t%s\n' "$name" "$expected" "$actual" "$status" >>"$SQL_REPORT"
}

require_zero() {
  local name=$1
  local sql=$2
  local actual
  actual=$(psql_once "$sql" | tail -n 1)
  actual=${actual:-0}
  if [ "$actual" = "0" ]; then
    append_result "$name" "0" "$actual" "pass"
    return
  fi
  append_result "$name" "0" "$actual" "fail"
  die "correctness invariant failed: $name actual=$actual"
}

require_positive() {
  local name=$1
  local sql=$2
  local actual
  actual=$(psql_once "$sql" | tail -n 1)
  actual=${actual:-0}
  if awk -v value="$actual" 'BEGIN { exit(value > 0 ? 0 : 1) }'; then
    append_result "$name" ">0" "$actual" "pass"
    return
  fi
  append_result "$name" ">0" "$actual" "fail"
  die "correctness evidence missing: $name actual=$actual"
}

require_sql_invariants() {
  printf 'name\texpected\tactual\tstatus\n' >"$SQL_REPORT"
  local cte
  cte=$(target_events_cte)

  require_zero "confirmed_not_oversold" "$cte
    SELECT count(*) FROM (
      SELECT e.event_id, COALESCE(e.capacity, 0) AS capacity, count(r.registration_id) AS confirmed
      FROM target_events e
      LEFT JOIN registrations r ON r.event_id = e.event_id AND r.status = 'confirmed'
      GROUP BY e.event_id, e.capacity
      HAVING count(r.registration_id) > COALESCE(e.capacity, 0)
    ) violations"

  require_zero "duplicate_active_registration" "$cte
    SELECT count(*) FROM (
      SELECT r.event_id, r.employee_id
      FROM registrations r
      JOIN target_events e ON e.event_id = r.event_id
      WHERE r.status <> 'cancelled'
      GROUP BY r.event_id, r.employee_id
      HAVING count(*) > 1
    ) violations"

  require_zero "ticket_for_non_confirmed_registration" "$cte
    SELECT count(*)
    FROM tickets t
    JOIN registrations r ON r.registration_id = t.registration_id
    WHERE (t.event_id IN (SELECT event_id FROM target_events) OR r.event_id IN (SELECT event_id FROM target_events))
      AND r.status <> 'confirmed'"

  require_zero "ticket_registration_mismatch" "$cte
    SELECT count(*)
    FROM tickets t
    JOIN registrations r ON r.registration_id = t.registration_id
    WHERE (t.event_id IN (SELECT event_id FROM target_events) OR r.event_id IN (SELECT event_id FROM target_events))
      AND (t.event_id <> r.event_id OR t.employee_id <> r.employee_id)"

  require_zero "duplicate_accepted_checkin_records" "$cte
    SELECT count(*) FROM (
      SELECT c.ticket_id
      FROM checkin_records c
      JOIN tickets t ON t.ticket_id = c.ticket_id
      JOIN target_events e ON e.event_id = t.event_id
      GROUP BY c.ticket_id
      HAVING count(*) > 1
    ) violations"

  require_zero "duplicate_offline_accepted_scans" "$cte
    SELECT count(*) FROM (
      SELECT s.ticket_id
      FROM offline_checkin_scans s
      JOIN offline_checkin_batches b ON b.batch_id = s.batch_id
      JOIN target_events e ON e.event_id = b.event_id
      WHERE s.status = 'accepted' AND s.ticket_id IS NOT NULL
      GROUP BY s.ticket_id
      HAVING count(*) > 1
    ) violations"

  require_positive "booking_pressure_waitlisted" "$cte
    SELECT count(*)
    FROM registrations r
    JOIN target_events e ON e.event_id = r.event_id
    WHERE e.kind = 'booking' AND r.status = 'waitlisted'"

  require_positive "checkin_records_created" "$cte
    SELECT count(*)
    FROM checkin_records c
    JOIN tickets t ON t.ticket_id = c.ticket_id
    JOIN target_events e ON e.event_id = t.event_id"

  require_positive "online_duplicate_rejections" "$cte
    SELECT count(*)
    FROM checkin_rejections cr
    JOIN tickets t ON t.ticket_id = cr.ticket_id
    JOIN target_events e ON e.event_id = t.event_id
    WHERE e.kind = 'online' AND cr.reason = 'ticket_already_redeemed'"

  require_positive "offline_duplicate_scans" "$cte
    SELECT count(*)
    FROM offline_checkin_scans s
    JOIN offline_checkin_batches b ON b.batch_id = s.batch_id
    JOIN target_events e ON e.event_id = b.event_id
    WHERE e.kind = 'offline' AND s.status = 'duplicate'"

  require_positive "offline_duplicate_batches" "$cte
    SELECT count(*) FROM (
      SELECT DISTINCT s.batch_id
      FROM offline_checkin_scans s
      JOIN offline_checkin_batches b ON b.batch_id = s.batch_id
      JOIN target_events e ON e.event_id = b.event_id
      WHERE e.kind = 'offline' AND s.status = 'duplicate' AND b.device_id LIKE 'k6-$RUN_ID-offline-%'
    ) duplicate_batches
    HAVING count(*) > 1"

  require_positive "mixed_online_wins" "$cte
    SELECT count(*)
    FROM checkin_records c
    JOIN tickets t ON t.ticket_id = c.ticket_id
    JOIN target_events e ON e.event_id = t.event_id
    WHERE e.kind = 'mixed' AND c.device_id = 'k6-$RUN_ID-mixed-online'"

  require_positive "mixed_offline_lost_to_online" "$cte
    SELECT count(*)
    FROM offline_checkin_scans s
    JOIN offline_checkin_batches b ON b.batch_id = s.batch_id
    JOIN target_events e ON e.event_id = b.event_id
    JOIN checkin_records c ON c.ticket_id = s.ticket_id
    WHERE e.kind = 'mixed'
      AND s.status = 'duplicate'
      AND b.device_id LIKE 'k6-$RUN_ID-mixed-%'
      AND c.device_id = 'k6-$RUN_ID-mixed-online'"

  require_positive "online_invalid_ticket_rejections" "$cte
    SELECT count(*)
    FROM checkin_rejections cr
    JOIN target_events e ON e.event_id = cr.event_id
    WHERE e.kind = 'online' AND cr.device_id = 'k6-$RUN_ID-invalid-online' AND cr.reason = 'invalid_ticket_token'"

  require_positive "offline_invalid_ticket_conflicts" "$cte
    SELECT count(*)
    FROM offline_checkin_scans s
    JOIN offline_checkin_batches b ON b.batch_id = s.batch_id
    JOIN target_events e ON e.event_id = b.event_id
    WHERE e.kind = 'offline'
      AND b.device_id LIKE 'k6-$RUN_ID-invalid-offline-%'
      AND s.conflict_reason = 'invalid_ticket_token'"
}

require_replica_logs() {
  log "checking request logs include replica field"
  kubectl_bm -n "$CETS_NAMESPACE" logs -l app=backend --since="$LOG_SINCE" |
    jq -Rr 'fromjson? | select(.msg == "request handled" and (.replica // "") != "") | .replica' |
    sort -u >"$LOG_REPLICAS"
  local replica_count
  replica_count=$(wc -l <"$LOG_REPLICAS" | tr -d ' ')
  if awk -v value="${replica_count:-0}" 'BEGIN { exit(value >= 3 ? 0 : 1) }'; then
    return
  fi
  die "request logs did not include at least 3 backend replicas; observed $replica_count"
}

write_report() {
  local report="$ARTIFACT_DIR/correctness-report-$RUN_ID.md"
  local summary="$ARTIFACT_DIR/k6-$RUN_ID.json"
  {
    printf '# K8s Pressure Correctness Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Base URL | `%s` |\n' "$BASE_URL"
    printf '| Host header | `%s` |\n' "$HOST_HEADER"
    printf '| Duration | `%s` |\n' "$DURATION"
    printf '| Employee fixture | `%s` employees, prefix `%s` |\n' "$EMPLOYEE_COUNT" "$EMPLOYEE_PREFIX"
    printf '| Booking capacity | `%s` |\n' "$BOOKING_CAPACITY"
    printf '| Check-in tickets per fixture | `%s` |\n' "$CHECKIN_TICKETS"
    printf '| Offline sync batches per fixture | `%s` |\n' "$OFFLINE_BATCHES"
    printf '| Target RPS | `%s` total: booking `%s`, online `%s`, offline `%s`, mixed online `%s`, mixed offline `%s`, invalid `%s` |\n' "$((BOOKING_RPS + ONLINE_RPS + OFFLINE_RPS + MIXED_ONLINE_RPS + MIXED_OFFLINE_RPS + INVALID_RPS))" "$BOOKING_RPS" "$ONLINE_RPS" "$OFFLINE_RPS" "$MIXED_ONLINE_RPS" "$MIXED_OFFLINE_RPS" "$INVALID_RPS"
    printf '| Event IDs | `%s` |\n' "$EVENT_IDS_FILE"
    printf '| k6 summary | `%s` |\n' "$summary"
    printf '| SQL invariants | `%s` |\n' "$SQL_REPORT"
    printf '| Replica log evidence | `%s` |\n' "$LOG_REPLICAS"
    printf '\n## k6 Metrics\n\n'
    jq -r '
      def metric_value($name; $field):
        (.metrics[$name][$field] // .metrics[$name].percentiles[$field] // "n/a");
      [
        ["http_req_duration p95", metric_value("http_req_duration"; "p(95)")],
        ["correctness duration p95", metric_value("k8s_correctness_duration"; "p(95)")],
        ["booking attempts", (.metrics.k8s_correctness_booking_attempts.count // "n/a")],
        ["online accepted", (.metrics.k8s_correctness_online_accepted.count // "n/a")],
        ["online duplicate", (.metrics.k8s_correctness_online_duplicate.count // "n/a")],
        ["offline accepted", (.metrics.k8s_correctness_offline_accepted.count // "n/a")],
        ["offline duplicate", (.metrics.k8s_correctness_offline_duplicate.count // "n/a")],
        ["offline conflict", (.metrics.k8s_correctness_offline_conflict.count // "n/a")],
        ["mixed online accepted", (.metrics.k8s_correctness_mixed_online_accepted.count // "n/a")],
        ["mixed online duplicate", (.metrics.k8s_correctness_mixed_online_duplicate.count // "n/a")],
        ["mixed offline accepted", (.metrics.k8s_correctness_mixed_offline_accepted.count // "n/a")],
        ["mixed offline duplicate", (.metrics.k8s_correctness_mixed_offline_duplicate.count // "n/a")],
        ["mixed offline conflict", (.metrics.k8s_correctness_mixed_offline_conflict.count // "n/a")],
        ["invalid rejected", (.metrics.k8s_correctness_invalid_rejected.count // "n/a")],
        ["online invalid rejected", (.metrics.k8s_correctness_online_invalid_rejected.count // "n/a")],
        ["offline invalid rejected", (.metrics.k8s_correctness_offline_invalid_rejected.count // "n/a")]
      ] | .[] | "- \(.[0]): `\(.[1])`"
    ' "$summary"
    printf '\n## SQL Invariants\n\n'
    printf '| Name | Expected | Actual | Status |\n| --- | --- | --- | --- |\n'
    tail -n +2 "$SQL_REPORT" | awk -F '\t' '{ printf "| `%s` | `%s` | `%s` | `%s` |\n", $1, $2, $3, $4 }'
    printf '\n## Replica Logs\n\n'
    sed 's/^/- `/' "$LOG_REPLICAS" | sed 's/$/`/'
    printf '\n'
  } >"$report"
  log "wrote correctness report: $report"
}

main() {
  mkdir -p "$ARTIFACT_DIR"
  chmod 0777 "$ARTIFACT_DIR"
  PROVIDER_TOKEN_SECRET=$(provider_secret)
  export PROVIDER_TOKEN_SECRET

  curl -fsS -H "Host: $HOST_HEADER" "$BASE_URL/readyz" >/dev/null ||
    die "correctness target is not ready: $BASE_URL with Host=$HOST_HEADER"
  seed_correctness_employees
  run_k6_correctness
  load_event_ids
  require_sql_invariants
  require_replica_logs
  write_report
  log "pressure correctness verification passed"
}

main "$@"
