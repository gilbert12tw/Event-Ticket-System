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
RUN_ID=${CETS_SMOKE_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}

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
    *) die "$method $path returned HTTP $status: $(jq -cr '.error // .error_code // .message // .' "$output" 2>/dev/null || cat "$output")" ;;
  esac
  jq -e '.success == true' "$output" >/dev/null || die "$method $path did not return success=true"
}

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

admin_token=$(sign_token "admin-1" "Admin One" "activity_admin" "Welfare Committee" "Taipei HQ" "Taipei" 7)
employee_token=$(sign_token "E1001" "Ariel Chen" "employee" "Engineering" "Taipei HQ" "Taipei" 6)
staff_token=$(sign_token "staff-1" "Staff One" "checkin_staff" "Operations" "Taipei HQ" "Taipei" 5)
hr_token=$(sign_token "hr-1" "HR One" "hr_admin" "Human Resources" "Taipei HQ" "Taipei" 6)

log "checking auth/me through internal ingress"
api GET /api/v1/auth/me "$employee_token" "" "$tmpdir/me.json"
jq -e '.data.employee_id == "E1001" and (.data.mapped_roles | index("employee")) and .data.claims_status == "complete"' "$tmpdir/me.json" >/dev/null

log "creating smoke event"
starts_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
registration_start=$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)
registration_close=$(date -u -d '+1 day' +%Y-%m-%dT%H:%M:%SZ)
event_body=$(jq -nc \
  --arg title "baremetal-smoke-$RUN_ID" \
  --arg starts_at "$starts_at" \
  --arg registration_start "$registration_start" \
  --arg registration_close "$registration_close" \
  '{title:$title,description:"Bare-metal HA smoke event",location:"Taipei HQ",event_city:"Taipei",event_site:"Taipei HQ",starts_at:$starts_at,registration_start:$registration_start,registration_close:$registration_close,capacity_type:"limited",capacity:5,allows_family:false,status:"published",category:"ops-smoke",tags:["baremetal","ha"],entry_method:"qr",visibility:"eligible",rule:{department:"Engineering",site:"Taipei HQ",min_grade:1,employment_status:"active"}}')
api POST /api/v1/admin/events "$admin_token" "$event_body" "$tmpdir/event.json"
event_id=$(jq -er '.data.event_id' "$tmpdir/event.json")

log "booking smoke event as employee"
booking_body=$(jq -nc --arg key "baremetal-smoke-$RUN_ID" '{idempotency_key:$key,family_count:0}')
api POST "/api/v1/events/$event_id/bookings" "$employee_token" "$booking_body" "$tmpdir/booking.json"
jq -e '.data.registration.status == "confirmed"' "$tmpdir/booking.json" >/dev/null
registration_id=$(jq -er '.data.registration.registration_id' "$tmpdir/booking.json")
ticket_id=$(jq -er '.data.ticket.ticket_id' "$tmpdir/booking.json")
signed_token=$(jq -er '.data.ticket.signed_token' "$tmpdir/booking.json")
[ -n "$signed_token" ] || die "booking did not return signed ticket token"

log "checking employee ticket read path"
api GET /api/v1/me/tickets "$employee_token" "" "$tmpdir/tickets.json"
jq -e --arg ticket_id "$ticket_id" 'any(.data[]; .ticket_id == $ticket_id and .status == "active")' "$tmpdir/tickets.json" >/dev/null

log "checking check-in path"
checkin_body=$(jq -nc --arg signed_token "$signed_token" --arg event_id "$event_id" '{signed_token:$signed_token,event_id:$event_id,device_id:"baremetal-smoke"}')
api POST /api/v1/checkins "$staff_token" "$checkin_body" "$tmpdir/checkin.json"
jq -e '.data.status == "accepted"' "$tmpdir/checkin.json" >/dev/null

log "checking audit log path"
api GET "/api/v1/admin/audit-logs?action=booking.confirmed&entity_id=$registration_id&limit=5" "$hr_token" "" "$tmpdir/audit.json"
jq -e --arg registration_id "$registration_id" 'any(.data[]; .entity_id == $registration_id and .action == "booking.confirmed")' "$tmpdir/audit.json" >/dev/null

log "application smoke verification completed for event $event_id"
