#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd base64
require_cmd curl
require_cmd jq
require_cmd kubectl
require_cmd openssl

BASE_URL=${CETS_SMOKE_BASE_URL:-http://$METALLB_INGRESS_IP}
HOST_HEADER=${CETS_SMOKE_HOST_HEADER:-$CETS_PUBLIC_HOSTNAME}
RUN_ID=${CETS_REDACTION_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
LOKI_LOCAL_PORT=${CETS_LOKI_LOCAL_PORT:-31100}

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

api() {
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

start_loki_port_forward() {
  kubectl_bm -n observability port-forward --address 127.0.0.1 svc/loki-gateway "$LOKI_LOCAL_PORT:80" >"$tmpdir/loki-port-forward.log" 2>&1 &
  pf_pid=$!
  for _ in $(seq 1 20); do
    if curl -fsS "http://127.0.0.1:$LOKI_LOCAL_PORT/loki/api/v1/status/buildinfo" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  die "Loki port-forward did not become ready"
}

loki_query() {
  local query=$1
  local output=$2
  curl -fsG \
    --data-urlencode "query=$query" \
    --data-urlencode "start=$loki_start_ns" \
    --data-urlencode "limit=20" \
    "http://127.0.0.1:$LOKI_LOCAL_PORT/loki/api/v1/query_range" >"$output"
}

require_loki_match() {
  local query=$1
  local label=$2
  local output="$tmpdir/loki-positive.json"
  for _ in $(seq 1 24); do
    loki_query "$query" "$output"
    if jq -e '(.data.result | length) > 0' "$output" >/dev/null; then
      return
    fi
    sleep 5
  done
  die "Loki did not return expected $label logs"
}

reject_loki_match() {
  local query=$1
  local label=$2
  local output="$tmpdir/loki-negative.json"
  loki_query "$query" "$output"
  if jq -e '(.data.result | length) > 0' "$output" >/dev/null; then
    die "Loki contains raw $label canary"
  fi
}

tmpdir=$(mktemp -d)
pf_pid=""
trap 'if [ -n "${pf_pid:-}" ]; then kill "$pf_pid" >/dev/null 2>&1 || true; fi; rm -rf "$tmpdir"' EXIT

loki_start_ns=$(date +%s%N)
admin_token=$(sign_token "admin-1" "Admin One" "activity_admin" "Welfare Committee" "Taipei HQ" "Taipei" 7)
employee_token=$(sign_token "E1001" "Ariel Chen" "employee" "Engineering" "Taipei HQ" "Taipei" 6)
staff_token=$(sign_token "staff-1" "Staff One" "checkin_staff" "Operations" "Taipei HQ" "Taipei" 5)

log "running redaction canary API flow"
starts_at=$(date -u -d '+7 days' +%Y-%m-%dT%H:%M:%SZ)
registration_start=$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)
registration_close=$(date -u -d '+6 days' +%Y-%m-%dT%H:%M:%SZ)
raw_idempotency_key="phase3-raw-idempotency-canary-$RUN_ID"
event_body=$(jq -nc \
  --arg title "redaction-canary-$RUN_ID" \
  --arg starts_at "$starts_at" \
  --arg registration_start "$registration_start" \
  --arg registration_close "$registration_close" \
  '{title:$title,description:"Telemetry redaction canary",location:"Taipei HQ",event_city:"Taipei",event_site:"Taipei HQ",starts_at:$starts_at,registration_start:$registration_start,registration_close:$registration_close,capacity_type:"limited",capacity:5,allows_family:false,status:"published",category:"ops-smoke",tags:["baremetal","redaction"],entry_method:"qr",visibility:"eligible",rule:{department:"Engineering",site:"Taipei HQ",min_grade:1,employment_status:"active"}}')
api POST /api/v1/admin/events "$admin_token" "$event_body" "$tmpdir/event.json"
event_id=$(jq -er '.data.event_id' "$tmpdir/event.json")

booking_body=$(jq -nc --arg key "$raw_idempotency_key" '{idempotency_key:$key,family_count:0}')
api POST "/api/v1/events/$event_id/bookings" "$employee_token" "$booking_body" "$tmpdir/booking.json"
signed_token=$(jq -er '.data.ticket.signed_token' "$tmpdir/booking.json")
[ -n "$signed_token" ] || die "booking did not return signed ticket token"

checkin_body=$(jq -nc --arg signed_token "$signed_token" --arg event_id "$event_id" '{signed_token:$signed_token,event_id:$event_id,device_id:"phase3-redaction-device"}')
api POST /api/v1/checkins "$staff_token" "$checkin_body" "$tmpdir/checkin.json"

log "checking Loki backend log ingestion and raw-sensitive-value absence"
start_loki_port_forward
require_loki_match '{namespace="cets", app="backend"} |= "request handled"' "backend request"
reject_loki_match "{namespace=\"cets\"} |= \"$admin_token\"" "provider token"
reject_loki_match "{namespace=\"cets\"} |= \"$employee_token\"" "provider token"
reject_loki_match "{namespace=\"cets\"} |= \"$staff_token\"" "provider token"
reject_loki_match "{namespace=\"cets\"} |= \"$signed_token\"" "signed ticket token"
reject_loki_match "{namespace=\"cets\"} |= \"$raw_idempotency_key\"" "idempotency key"
reject_loki_match '{namespace="cets"} |= "Ariel Chen"' "employee display name"
reject_loki_match '{namespace="cets"} |= "E1001"' "employee id"

log "telemetry redaction verification completed"
