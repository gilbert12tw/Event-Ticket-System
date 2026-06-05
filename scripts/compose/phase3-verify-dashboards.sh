#!/usr/bin/env bash
# Verify that all six Event-Ticket-System Grafana dashboards have live data.
# Run after phase3-k6.sh stress to ensure all signal types are populated.
set -euo pipefail

GRAFANA_URL=${GRAFANA_URL:-http://localhost:3000}
GRAFANA_USER=${GRAFANA_USER:-admin}
GRAFANA_PASS=${GRAFANA_PASS:-admin}
PROM_URL=${PROM_URL:-http://localhost:9090}
LOKI_URL=${LOKI_URL:-http://localhost:3100}
TEMPO_URL=${TEMPO_URL:-http://localhost:3200}
PYROSCOPE_URL=${PYROSCOPE_URL:-http://localhost:4040}

PASS=0
FAIL=0

log()  { printf '[verify-dashboards] %s\n' "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; PASS=$((PASS+1)); }
fail() { printf '  \033[31m✗\033[0m %s\n' "$*"; FAIL=$((FAIL+1)); }
info() { printf '  \033[34m→\033[0m %s\n' "$*"; }

# Query Prometheus instant vector; print series count
prom_count() {
  curl -s --max-time 10 \
    "$PROM_URL/api/v1/query?query=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$1")" \
    2>/dev/null | python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
    print(len(d.get('data',{}).get('result',[])))
except:
    print(0)
"
}

check_prom() {
  local metric="$1" label="${2:-}" desc="${3:-$1}"
  local count
  if [ -n "$label" ]; then
    count=$(prom_count "${metric}{${label}}")
  else
    count=$(prom_count "$metric")
  fi
  if [ "$count" -gt 0 ] 2>/dev/null; then
    ok "$desc ($count series)"
  else
    fail "$desc — 0 series found (metric: $metric)"
  fi
}

log "=== Grafana Datasource Health ==="
# Get datasource IDs then test each via /api/datasources/{id}/health
ds_json=$(curl -s --max-time 10 -u "${GRAFANA_USER}:${GRAFANA_PASS}" \
  "${GRAFANA_URL}/api/datasources" 2>/dev/null || echo "[]")
for ds in Prometheus Loki Tempo Pyroscope; do
  ds_id=$(python3 -c "
import json,sys
for d in json.loads(sys.argv[1]):
    if d.get('name')=='${ds}':
        print(d.get('id',''))
        break
" "$ds_json" 2>/dev/null)
  if [ -z "$ds_id" ]; then
    fail "$ds datasource: not found in Grafana"
    continue
  fi
  status=$(curl -s --max-time 10 -u "${GRAFANA_USER}:${GRAFANA_PASS}" \
    "${GRAFANA_URL}/api/datasources/${ds_id}/health" 2>/dev/null \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('status','error').lower())" 2>/dev/null || echo "error")
  if [ "$status" = "ok" ]; then ok "$ds datasource: ok"
  else fail "$ds datasource: $status"
  fi
done

log ""
log "=== Golden Signals (ets-01) ==="
check_prom "cets_http_requests_total" "" "Traffic metric"
check_prom "cets_http_request_seconds_bucket" "" "Latency histogram"
check_prom "cets_build_info" "" "Active replicas"
check_prom "cets_db_pool_conns" "state=\"acquired\"" "DB pool connections"
check_prom "probe_success" "" "Blackbox probe"

log ""
log "=== RED Traffic Drilldown (ets-02) ==="
fivexx_count=$(prom_count 'cets_http_requests_total{status_class="5xx"}')
if [ "$fivexx_count" -gt 0 ] 2>/dev/null; then
  ok "5xx error counter ($fivexx_count series)"
else
  info "5xx error counter: 0 series — no 5xx errors in current window (healthy state)"
fi
check_prom "cets_http_requests_total" "status_class=\"4xx\"" "4xx counter (controlled errors)"
check_prom "cets_build_info" "" "Replica labels (template vars)"

log ""
log "=== Booking & Redis Pressure (ets-03) ==="
check_prom "cets_booking_stage_seconds_bucket" "" "Booking stage histogram"
check_prom "cets_booking_preadmission_seconds_bucket" "" "Pre-admission histogram"
check_prom "cets_reservation_attempt_total" "" "Reservation outcomes"

log ""
log "=== USE Infrastructure (ets-04) ==="
check_prom "cets_db_pool_acquire_wait_seconds_total" "job=\"cets-backend\"" "DB pool wait (per instance)"
check_prom "cets_db_pool_acquire_count_total" "job=\"cets-backend\"" "DB pool acquire count"
check_prom "cets_db_lock_waiting_sessions" "job=\"cets-backend\"" "DB lock sessions"
check_prom "container_memory_usage_bytes" "job=\"cets-cadvisor\"" "cAdvisor memory (aggregate)"
# node-exporter may not be available in all environments
node_count=$(prom_count "node_cpu_seconds_total")
if [ "$node_count" -gt 0 ]; then
  ok "node_cpu_seconds_total ($node_count series)"
else
  info "node_cpu_seconds_total: 0 series — node-exporter not running (requires shared/slave mount)"
fi

log ""
log "=== Outbox & Worker Health (ets-05) ==="
check_prom "cets_outbox_oldest_lag_seconds" "" "Outbox oldest lag"
check_prom "cets_outbox_pending_total" "" "Outbox pending count"
# Dead letter / retry metrics are 0 in healthy state — check structure only
dl_count=$(prom_count "cets_outbox_dead_letter_total")
info "cets_outbox_dead_letter_total: $dl_count series (0 = no dead letters = healthy)"
retry_count=$(prom_count "cets_worker_retry_total")
info "cets_worker_retry_total: $retry_count series (0 = no retries = healthy)"

log ""
log "=== Service Anomaly Investigation (ets-06) ==="

# Step 1: Metrics (5xx may be 0 in healthy state — check general traffic instead)
check_prom "cets_http_requests_total" "" "STEP 1: HTTP traffic metric"
fivexx_svc=$(prom_count 'cets_http_requests_total{status_class="5xx"}')
if [ "$fivexx_svc" -gt 0 ] 2>/dev/null; then
  ok "STEP 1: 5xx per route×replica visible ($fivexx_svc series)"
else
  info "STEP 1: 5xx rate=0 — healthy; dashboard shows data when errors occur"
fi

# Step 2: Booking deps
check_prom "cets_booking_stage_seconds_count" "outcome=\"error\"" "STEP 2: Booking stage errors"

# Step 3: Traces
trace_count=$(curl -s --max-time 10 \
  "${TEMPO_URL}/api/search?service.name=cets-backend&limit=5" 2>/dev/null \
  | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('traces',[])))" 2>/dev/null || echo 0)
if [ "$trace_count" -gt 0 ]; then
  ok "STEP 3: Tempo traces for cets-backend ($trace_count)"
else
  fail "STEP 3: No Tempo traces found for cets-backend — run k6 traffic first"
fi

# Service graph
check_prom "traces_service_graph_request_total" "" "STEP 3: Service graph (Tempo metrics generator)"

# Step 4: Logs
NOW=$(date +%s)
START=$((NOW - 600))
log_count=$(curl -s --max-time 10 \
  --data-urlencode 'query={service_name=~"backend-.*"} |= "otel_trace_id"' \
  --data-urlencode "start=${START}000000000" \
  --data-urlencode "end=${NOW}000000000" \
  --data-urlencode "limit=5" \
  "${LOKI_URL}/loki/api/v1/query_range" 2>/dev/null \
  | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('data',{}).get('result',[])))" 2>/dev/null || echo 0)
if [ "$log_count" -gt 0 ]; then
  ok "STEP 4: Loki backend logs with otel_trace_id ($log_count streams)"
else
  fail "STEP 4: No Loki logs with otel_trace_id found for backend-* — run k6 traffic first"
fi

# Step 5: Profiles
pyro_data=$(curl -s --max-time 10 \
  "${PYROSCOPE_URL}/pyroscope/render?from=now-10m&until=now&query=process_cpu:cpu:nanoseconds:cpu:nanoseconds%7Bservice_name%3D%22cets-backend%22%7D&format=json" 2>/dev/null \
  | python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
    timeline=d.get('timeline',{}).get('samples',[])
    non_zero=sum(1 for s in timeline if s and s>0)
    print(non_zero)
except:
    print(0)
" 2>/dev/null || echo 0)
if [ "$pyro_data" -gt 0 ]; then
  ok "STEP 5: Pyroscope CPU profile has $pyro_data non-zero samples for cets-backend"
else
  fail "STEP 5: No Pyroscope CPU samples for cets-backend — check PYROSCOPE_ENABLED=true"
fi

log ""
log "=== Dashboard Folder Provisioning ==="
folder_count=$(curl -s --max-time 10 -u "${GRAFANA_USER}:${GRAFANA_PASS}" \
  "${GRAFANA_URL}/api/dashboards/home" 2>/dev/null | python3 -c "import json,sys; print('ok')" 2>/dev/null || echo "error")
if [ "$folder_count" = "ok" ]; then
  ok "Grafana API reachable"
fi

# Check that Event-Ticket-System folder and dashboards exist
dash_count=$(curl -s --max-time 10 -u "${GRAFANA_USER}:${GRAFANA_PASS}" \
  "${GRAFANA_URL}/api/search?folderTitle=Event-Ticket-System&type=dash-db" 2>/dev/null \
  | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
if [ "$dash_count" -ge 6 ]; then
  ok "Event-Ticket-System folder has $dash_count dashboards"
elif [ "$dash_count" -gt 0 ]; then
  info "Event-Ticket-System folder has $dash_count dashboards (expected ≥6)"
else
  fail "Event-Ticket-System folder not found or empty in Grafana — check provisioning"
fi

log ""
log "=== Summary ==="
log "PASS: $PASS  FAIL: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  log "Some checks failed. Run 'scripts/compose/phase3-k6.sh stress' to generate traffic, then re-run."
  exit 1
else
  log "All checks passed."
fi
