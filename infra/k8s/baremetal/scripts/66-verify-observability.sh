#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd jq
require_cmd curl
require_cmd base64
require_cmd openssl

PROMETHEUS_LOCAL_PORT=${PROMETHEUS_LOCAL_PORT:-19090}
LOKI_LOCAL_PORT=${LOKI_LOCAL_PORT:-19100}
TEMPO_LOCAL_PORT=${TEMPO_LOCAL_PORT:-19200}
PYROSCOPE_LOCAL_PORT=${PYROSCOPE_LOCAL_PORT:-19404}
GRAFANA_LOCAL_PORT=${GRAFANA_LOCAL_PORT:-19300}
BASE_URL=${CETS_OBSERVABILITY_BASE_URL:-http://$METALLB_INGRESS_IP}
HOST_HEADER=${CETS_OBSERVABILITY_HOST_HEADER:-$CETS_PUBLIC_HOSTNAME}

PROM_PID=""
LOKI_PID=""
TEMPO_PID=""
PYROSCOPE_PID=""
GRAFANA_PID=""
TEMPO_TRACE_ID=""

cleanup() {
  for pid in "$PROM_PID" "$LOKI_PID" "$TEMPO_PID" "$PYROSCOPE_PID" "$GRAFANA_PID"; do
    if [ -n "$pid" ]; then
      kill "$pid" >/dev/null 2>&1 || true
      wait "$pid" >/dev/null 2>&1 || true
    fi
  done
}

b64url() {
  base64 | tr '+/' '-_' | tr -d '=\n'
}

provider_secret() {
  kubectl_bm -n "$CETS_NAMESPACE" get secret cets-runtime-env -o jsonpath='{.data.PROVIDER_TOKEN_SECRET}' | base64 -d
}

sign_token() {
  local employee_id=$1
  local display_name=$2
  local role=$3
  local department=$4
  local site=$5
  local city=$6
  local grade=$7
  local secret
  local exp
  local payload
  local signature

  secret=$(provider_secret)
  exp=$(date -u -d '+1 hour' +%s)
  payload=$(jq -nc \
    --arg employee_id "$employee_id" \
    --arg display_name "$display_name" \
    --arg role "$role" \
    --arg department "$department" \
    --arg site "$site" \
    --arg city "$city" \
    --arg employment_status "active" \
    --argjson grade "$grade" \
    --argjson exp "$exp" \
    '{employee_id:$employee_id,display_name:$display_name,role_claims:[$role],department:$department,site:$site,city:$city,grade:$grade,employment_status:$employment_status,exp:$exp}' | b64url)
  signature=$(printf '%s' "$payload" | openssl dgst -sha256 -hmac "$secret" -binary | b64url)
  printf '%s.%s' "$payload" "$signature"
}

api_json() {
  local method=$1
  local path=$2
  local token=$3
  local body=${4:-}
  local output=$5
  local status

  if [ -n "$body" ]; then
    status=$(curl -sS --max-time 20 \
      -H "Host: $HOST_HEADER" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      -X "$method" \
      -d "$body" \
      -o "$output" \
      -w '%{http_code}' \
      "$BASE_URL$path" || true)
  else
    status=$(curl -sS --max-time 20 \
      -H "Host: $HOST_HEADER" \
      -H "Authorization: Bearer $token" \
      -X "$method" \
      -o "$output" \
      -w '%{http_code}' \
      "$BASE_URL$path" || true)
  fi

  case "$status" in
    2*) ;;
    *) die "$method $path returned HTTP $status" ;;
  esac
  jq -e '.success == true' "$output" >/dev/null || die "$method $path did not return success=true"
}
trap cleanup EXIT

start_port_forward() {
  local namespace=$1
  local service=$2
  local local_port=$3
  local remote_port=$4
  local log_file=$5
  kubectl_bm -n "$namespace" port-forward --address 127.0.0.1 "svc/$service" "$local_port:$remote_port" >"$log_file" 2>&1 &
  printf '%s\n' "$!"
}

wait_http() {
  local label=$1
  local url=$2
  for _ in $(seq 1 30); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  die "$label did not become ready at $url"
}

wait_any_http() {
  local label=$1
  shift
  for _ in $(seq 1 30); do
    for url in "$@"; do
      if curl -fsS "$url" >/dev/null 2>&1; then
        return
      fi
    done
    sleep 1
  done
  die "$label did not become ready"
}

start_lgtm_port_forwards() {
  log "opening local port-forwards for Prometheus, Loki, Tempo, and Pyroscope"
  PROM_PID=$(start_port_forward observability kube-prometheus-stack-prometheus "$PROMETHEUS_LOCAL_PORT" 9090 "$GENERATED_DIR/prometheus-port-forward.log")
  LOKI_PID=$(start_port_forward observability loki-gateway "$LOKI_LOCAL_PORT" 80 "$GENERATED_DIR/loki-port-forward.log")
  TEMPO_PID=$(start_port_forward observability tempo "$TEMPO_LOCAL_PORT" 3200 "$GENERATED_DIR/tempo-port-forward.log")
  PYROSCOPE_PID=$(start_port_forward observability pyroscope "$PYROSCOPE_LOCAL_PORT" 4040 "$GENERATED_DIR/pyroscope-port-forward.log")
  wait_http "Prometheus" "http://127.0.0.1:$PROMETHEUS_LOCAL_PORT/-/ready"
  wait_http "Loki" "http://127.0.0.1:$LOKI_LOCAL_PORT/loki/api/v1/status/buildinfo"
  wait_http "Tempo" "http://127.0.0.1:$TEMPO_LOCAL_PORT/ready"
  wait_any_http "Pyroscope" "http://127.0.0.1:$PYROSCOPE_LOCAL_PORT/ready" "http://127.0.0.1:$PYROSCOPE_LOCAL_PORT/-/ready"
}

prom_query() {
  curl -fsS --get --data-urlencode "query=$1" "http://127.0.0.1:$PROMETHEUS_LOCAL_PORT/api/v1/query"
}

prom_query_nonzero() {
  local query=$1
  local response
  response=$(prom_query "$query" 2>/dev/null || true)
  printf '%s\n' "$response" | grep -Eq '"value":\[[^]]+,"[0-9.]*[1-9][0-9.]*"\]|"result":\[[^]]+,"[0-9.]*[1-9][0-9.]*"\]'
}

generate_backend_trace() {
  for _ in $(seq 1 12); do
    curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/healthz" >/dev/null || true
    curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$METALLB_INGRESS_IP/readyz" >/dev/null || true
    sleep 2
  done
}

generate_dependency_trace() {
  local run_id
  local admin_token
  local employee_token
  local starts_at
  local registration_start
  local registration_close
  local event_body
  local event_id
  local poster_file
  local status
  local booking_body

  run_id=$(date -u +%Y%m%d%H%M%S)
  admin_token=$(sign_token "admin-1" "Admin One" "activity_admin" "Welfare Committee" "Taipei HQ" "Taipei" 7)
  employee_token=$(sign_token "E1001" "Ariel Chen" "employee" "Engineering" "Taipei HQ" "Taipei" 6)
  starts_at=$(date -u -d '+7 days' +%Y-%m-%dT%H:%M:%SZ)
  registration_start=$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)
  registration_close=$(date -u -d '+6 days' +%Y-%m-%dT%H:%M:%SZ)
  event_body=$(jq -nc \
    --arg title "observability-canary-$run_id" \
    --arg starts_at "$starts_at" \
    --arg registration_start "$registration_start" \
    --arg registration_close "$registration_close" \
    '{title:$title,description:"Observability canary",location:"Taipei HQ",event_city:"Taipei",event_site:"Taipei HQ",starts_at:$starts_at,registration_start:$registration_start,registration_close:$registration_close,capacity_type:"limited",capacity:5,allows_family:false,status:"published",category:"ops-smoke",tags:["baremetal","observability"],entry_method:"qr",visibility:"eligible",rule:{department:"Engineering",site:"Taipei HQ",min_grade:1,employment_status:"active"}}')
  api_json POST /api/v1/admin/events "$admin_token" "$event_body" "$GENERATED_DIR/observability-event.json"
  event_id=$(jq -er '.data.event_id' "$GENERATED_DIR/observability-event.json")

  poster_file="$GENERATED_DIR/observability-poster.png"
  printf '\211PNG\r\n\032\n\000\000\000\rIHDR\000\000\000\001\000\000\000\001\010\006\000\000\000\037\025\304\211\000\000\000\nIDATx\234c\000\001\000\000\005\000\001\r\n-\264\000\000\000\000IEND\256B\140\202' >"$poster_file"
  status=$(curl -sS --max-time 20 \
    -H "Host: $HOST_HEADER" \
    -H "Authorization: Bearer $admin_token" \
    -F "poster=@$poster_file;type=image/png" \
    -o "$GENERATED_DIR/observability-poster-upload.json" \
    -w '%{http_code}' \
    "$BASE_URL/api/v1/admin/events/$event_id/poster" || true)
  case "$status" in
    2*) ;;
    *) die "POST /api/v1/admin/events/$event_id/poster returned HTTP $status" ;;
  esac

  booking_body=$(jq -nc --arg key "observability-canary-$run_id" '{idempotency_key:$key,family_count:0}')
  api_json POST "/api/v1/events/$event_id/bookings" "$employee_token" "$booking_body" "$GENERATED_DIR/observability-booking.json"
  curl -fsS -H "Host: $HOST_HEADER" -H "Authorization: Bearer $employee_token" \
    "$BASE_URL/api/v1/events/$event_id/poster" >/dev/null || true
}

check_backend_red_metrics() {
  log "checking backend RED metrics by route/status/instance"
  for _ in $(seq 1 24); do
    if prom_query_nonzero 'sum(increase(cets_http_requests_total[15m]))' &&
      prom_query_nonzero 'sum(increase(cets_http_request_seconds_count[15m]))' &&
      prom_query_nonzero 'scalar(count(count by (instance) (increase(cets_http_requests_total[15m]) > 0)))'; then
      return
    fi
    generate_backend_trace
  done
  die "Prometheus did not return backend RED evidence"
}

check_tempo_trace_ingest() {
  log "checking Tempo backend trace ingest"
  for _ in $(seq 1 30); do
    traces=$(curl -fsS "$TEMPO_LOCAL_URL/api/search?tags=service.name%3Dcets-backend&limit=1" 2>/dev/null || true)
    trace_id=$(printf '%s\n' "$traces" | sed -n 's/.*"traceID":"\([a-fA-F0-9][a-fA-F0-9]*\)".*/\1/p' | head -n 1)
    if [ -n "$trace_id" ]; then
      detail=$(curl -fsS "$TEMPO_LOCAL_URL/api/traces/$trace_id" 2>/dev/null || true)
      if printf '%s\n' "$detail" | grep -q "cets-backend" &&
        printf '%s\n' "$detail" | grep -Eq "http.route|cets.route|rootTraceName"; then
        TEMPO_TRACE_ID=$trace_id
        return
      fi
    fi
    generate_backend_trace
    sleep 3
  done
  die "Tempo did not return cets-backend traces with route evidence"
}

check_loki_trace_logs() {
  log "checking Loki logs for Tempo trace $TEMPO_TRACE_ID"
  [ -n "$TEMPO_TRACE_ID" ] || die "Tempo trace ID is required"
  for _ in $(seq 1 24); do
    logs=$(curl -fsS --get \
      --data-urlencode "query={namespace=\"cets\", app=\"backend\"} |= \"$TEMPO_TRACE_ID\" |= \"otel_trace_id\"" \
      --data-urlencode "limit=5" \
      "$LOKI_LOCAL_URL/loki/api/v1/query_range" 2>/dev/null || true)
    if printf '%s\n' "$logs" | grep -q "$TEMPO_TRACE_ID" &&
      printf '%s\n' "$logs" | grep -q "otel_trace_id"; then
      printf '%s\n' "$logs" >"$GENERATED_DIR/backend-trace-log-check.json"
      return
    fi
    generate_backend_trace
    sleep 3
  done
  die "Loki did not return backend logs for Tempo trace $TEMPO_TRACE_ID"
}

check_service_graph_metrics() {
  log "checking service graph metrics involving cets-backend"
  for _ in $(seq 1 24); do
    if prom_query_nonzero 'sum(increase(traces_service_graph_request_total[15m]))' &&
      prom_query_nonzero 'sum(increase(traces_service_graph_request_total{server="cets-backend"}[15m]))'; then
      return
    fi
    generate_backend_trace
    sleep 5
  done
  die "Prometheus did not return service graph metrics for cets-backend"
}

check_required_service_graph_edges() {
  log "checking required service graph edges"
  for _ in $(seq 1 24); do
    if prom_query_nonzero 'sum(increase(traces_service_graph_request_total{client="user",server="ingress-nginx"}[15m]))' &&
      prom_query_nonzero 'sum(increase(traces_service_graph_request_total{client="user",server="cets-backend"}[15m]))' &&
      prom_query_nonzero 'sum(increase(traces_service_graph_request_total{client="cets-backend",server="postgres"}[15m]))' &&
      prom_query_nonzero 'sum(increase(traces_service_graph_request_total{client="cets-backend",server="redis"}[15m]))' &&
      prom_query_nonzero 'sum(increase(traces_service_graph_request_total{client="cets-backend",server="minio"}[15m]))'; then
      return
    fi
    generate_dependency_trace
    sleep 5
  done
  die "Prometheus did not return all required service graph edges"
}

check_pyroscope_profile_data() {
  log "checking Pyroscope backend CPU profile data"
  for _ in $(seq 1 24); do
    profile=$(curl -fsS --get \
      --data-urlencode 'query=process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="cets-backend"}' \
      --data-urlencode 'from=now-1h' \
      --data-urlencode 'until=now' \
      --data-urlencode 'maxNodes=64' \
      "$PYROSCOPE_LOCAL_URL/pyroscope/render" 2>/dev/null || true)
    if printf '%s\n' "$profile" | grep -Eq '"numTicks":[0-9]*[1-9][0-9]*'; then
      printf '%s\n' "$profile" >"$GENERATED_DIR/backend-profile-check.json"
      return
    fi
    generate_backend_trace
    sleep 5
  done
  die "Pyroscope did not return cets-backend CPU samples"
}

check_grafana_cets_folder_dashboards() {
  log "checking Grafana Event-Ticket-System folder dashboards"
  GRAFANA_PID=$(start_port_forward observability kube-prometheus-stack-grafana "$GRAFANA_LOCAL_PORT" 80 "$GENERATED_DIR/grafana-port-forward.log")
  wait_http "Grafana" "http://127.0.0.1:$GRAFANA_LOCAL_PORT/api/health"
  password=$(kubectl_bm -n observability get secret kube-prometheus-stack-grafana -o jsonpath='{.data.admin-password}' | base64 -d)
  curl -fsS -u "admin:$password" "http://127.0.0.1:$GRAFANA_LOCAL_PORT/api/search?folderIds=0" >/dev/null
  dashboards=$(curl -fsS -u "admin:$password" "http://127.0.0.1:$GRAFANA_LOCAL_PORT/api/search?query=ETS")
  for title in "ETS 01 — Golden Signals" "ETS 02 — RED Traffic Drilldown" "ETS 03 — Booking & Redis Pressure" "ETS 04 — USE Infrastructure" "ETS 05 — Outbox & Worker Health" "ETS 06 — Service Anomaly Investigation"; do
    printf '%s\n' "$dashboards" | jq -e --arg title "$title" 'any(.[]; .title == $title and .folderTitle == "Event-Ticket-System")' >/dev/null ||
      die "Grafana dashboard '$title' was not found in Event-Ticket-System folder"
  done
}

log "checking observability pod readiness"
kubectl_bm -n observability wait --for=condition=Ready pod --all --timeout=300s

log "checking Loki push and query path"
check_pod="obs-check-$(date +%s)"
kubectl_bm -n observability run "$check_pod" \
  --rm -i \
  --restart=Never \
  --image=curlimages/curl:8.17.0 \
  --command -- sh -eu -c '
    ts="$(date +%s)000000000"
    payload="{\"streams\":[{\"stream\":{\"job\":\"cets-observability-check\"},\"values\":[[\"$ts\",\"cets observability check\"]]}]}"
    curl -fsS -H "Content-Type: application/json" -XPOST --data-raw "$payload" http://loki-gateway/loki/api/v1/push
    sleep 3
    curl -fsG --data-urlencode "query={job=\"cets-observability-check\"}" http://loki-gateway/loki/api/v1/query_range
    echo
    curl -fsS http://kube-prometheus-stack-prometheus:9090/-/ready
    echo
    curl -fsS "http://kube-prometheus-stack-prometheus:9090/api/v1/targets?state=active"
  ' >"$GENERATED_DIR/observability-check.json"

grep -q '"resultType":"streams"' "$GENERATED_DIR/observability-check.json" || die "Loki query did not return streams"
grep -q 'cets observability check' "$GENERATED_DIR/observability-check.json" || die "Loki query did not return the pushed canary line"
grep -q 'Prometheus Server is Ready' "$GENERATED_DIR/observability-check.json" || die "Prometheus readiness check failed"
grep -q '"job":"cets-backend"' "$GENERATED_DIR/observability-check.json" || die "Prometheus target for cets-backend ServiceMonitor not found"

log "checking Loki backend pod log ingestion"
backend_log_pod="obs-backend-log-check-$(date +%s)"
kubectl_bm -n observability run "$backend_log_pod" \
  --rm -i \
  --restart=Never \
  --image=curlimages/curl:8.17.0 \
  --command -- sh -eu -c '
    for i in $(seq 1 24); do
      result=$(curl -fsG \
        --data-urlencode "query={namespace=\"cets\", app=\"backend\"} |= \"request handled\"" \
        --data-urlencode "limit=5" \
        http://loki-gateway/loki/api/v1/query_range)
      if printf "%s" "$result" | grep -q "\"result\":\\[" && ! printf "%s" "$result" | grep -q "\"result\":\\[\\]"; then
        printf "%s\n" "$result"
        exit 0
      fi
      sleep 5
    done
    printf "%s\n" "${result:-}"
    exit 1
  ' >"$GENERATED_DIR/backend-log-check.json" || die "Loki did not return backend logs"
grep -q '"namespace":"cets"' "$GENERATED_DIR/backend-log-check.json" || die "backend Loki log is missing namespace label"
grep -q '"app":"backend"' "$GENERATED_DIR/backend-log-check.json" || die "backend Loki log is missing app label"

log "checking Tempo and Pyroscope services"
kubectl_bm -n observability get svc tempo pyroscope >/dev/null

TEMPO_LOCAL_URL="http://127.0.0.1:$TEMPO_LOCAL_PORT"
LOKI_LOCAL_URL="http://127.0.0.1:$LOKI_LOCAL_PORT"
PYROSCOPE_LOCAL_URL="http://127.0.0.1:$PYROSCOPE_LOCAL_PORT"

check_worker_metrics_targets() {
  log "checking worker metrics targets in Prometheus"
  for _ in $(seq 1 24); do
    if prom_query_nonzero 'count(cets_build_info{service=~"cets-worker-.*"})'; then
      return
    fi
    sleep 5
  done
  die "Prometheus did not return cets_build_info for any cets-worker-* service"
}

start_lgtm_port_forwards
generate_backend_trace
generate_dependency_trace
check_backend_red_metrics
check_worker_metrics_targets
check_tempo_trace_ingest
check_loki_trace_logs
check_service_graph_metrics
check_required_service_graph_edges
check_pyroscope_profile_data
check_grafana_cets_folder_dashboards

log "observability verification completed"
