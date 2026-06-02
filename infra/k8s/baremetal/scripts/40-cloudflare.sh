#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd kubectl
require_cmd jq
require_env CLOUDFLARE_API_TOKEN
require_env CLOUDFLARE_ACCOUNT_ID
require_env CLOUDFLARE_ZONE_ID
require_env CLOUDFLARE_ZONE_NAME
require_env CLOUDFLARE_ACCESS_ALLOWED_EMAILS

TF=$(tf_bin)
CF_DIR="$BM_DIR/cloudflare"
TFVARS="$CF_DIR/terraform.tfvars.json"

log "checking Cloudflare zone status"
zone_json=$(curl -fsS -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones/$CLOUDFLARE_ZONE_ID")
active=$(printf '%s' "$zone_json" | jq -r '.result.status')
[ "$active" = "active" ] || die "Cloudflare zone is '$active', not active. Add the domain to Cloudflare and update registrar nameservers first."

emails_json=$(printf '%s' "$CLOUDFLARE_ACCESS_ALLOWED_EMAILS" | jq -R 'split(",") | map(gsub("^\\s+|\\s+$"; "")) | map(select(length > 0))')
if [ -n "${CLOUDFLARE_ACCESS_DEVICE_POSTURE_RULE_IDS:-}" ]; then
  posture_json=$(printf '%s' "$CLOUDFLARE_ACCESS_DEVICE_POSTURE_RULE_IDS" | jq -R 'split(",") | map(gsub("^\\s+|\\s+$"; "")) | map(select(length > 0))')
else
  posture_json='[]'
fi

jq -n \
  --arg cloudflare_api_token "$CLOUDFLARE_API_TOKEN" \
  --arg cloudflare_account_id "$CLOUDFLARE_ACCOUNT_ID" \
  --arg cloudflare_zone_id "$CLOUDFLARE_ZONE_ID" \
  --arg cloudflare_zone_name "$CLOUDFLARE_ZONE_NAME" \
  --arg hostname "$CETS_PUBLIC_HOSTNAME" \
  --arg tunnel_name "$CLOUDFLARE_TUNNEL_NAME" \
  --argjson allowed_emails "$emails_json" \
  --argjson device_posture_rule_ids "$posture_json" \
  '{
    cloudflare_api_token: $cloudflare_api_token,
    cloudflare_account_id: $cloudflare_account_id,
    cloudflare_zone_id: $cloudflare_zone_id,
    cloudflare_zone_name: $cloudflare_zone_name,
    hostname: $hostname,
    tunnel_name: $tunnel_name,
    allowed_emails: $allowed_emails,
    device_posture_rule_ids: $device_posture_rule_ids
  }' >"$TFVARS"
chmod 0600 "$TFVARS"

log "applying Cloudflare Tunnel, DNS, and Access resources"
(cd "$CF_DIR" && "$TF" init && "$TF" apply -auto-approve)

tunnel_token=$(cd "$CF_DIR" && "$TF" output -raw tunnel_token)
require_env CETS_NAMESPACE
kubectl_bm create namespace "$CETS_NAMESPACE" --dry-run=client -o yaml | kubectl_bm apply -f -
kubectl_bm -n "$CETS_NAMESPACE" create secret generic cloudflared-token \
  --from-literal=tunnel-token="$tunnel_token" \
  --dry-run=client -o yaml | kubectl_bm apply -f -

log "Cloudflare resources applied and cloudflared token secret written"
