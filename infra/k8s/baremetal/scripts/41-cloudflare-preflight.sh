#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd jq
require_cmd curl

require_env CETS_PUBLIC_HOSTNAME
require_env CLOUDFLARE_API_TOKEN
require_env CLOUDFLARE_ACCOUNT_ID
require_env CLOUDFLARE_ZONE_ID
require_env CLOUDFLARE_ZONE_NAME
require_env CLOUDFLARE_ACCESS_ALLOWED_EMAILS

[ "$CETS_PUBLIC_HOSTNAME" != "tickets.example.com" ] || die "set CETS_PUBLIC_HOSTNAME to the real Cloudflare hostname"
[ "$CLOUDFLARE_ZONE_NAME" != "example.com" ] || die "set CLOUDFLARE_ZONE_NAME to the real Cloudflare zone"

TF=$(tf_bin)
log "validating Cloudflare OpenTofu/Terraform configuration"
(cd "$BM_DIR/cloudflare" && "$TF" init -backend=false >/dev/null && "$TF" validate >/dev/null)

log "checking Cloudflare zone status"
zone_json=$(curl -fsS -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones/$CLOUDFLARE_ZONE_ID")
active=$(printf '%s' "$zone_json" | jq -r '.result.status')
[ "$active" = "active" ] || die "Cloudflare zone is '$active', not active"

log "checking hostname belongs to zone"
case "$CETS_PUBLIC_HOSTNAME" in
  *."$CLOUDFLARE_ZONE_NAME"|"$CLOUDFLARE_ZONE_NAME") ;;
  *) die "$CETS_PUBLIC_HOSTNAME is not under zone $CLOUDFLARE_ZONE_NAME" ;;
esac

log "checking Cloudflare Access allow-list inputs"
email_count=$(printf '%s' "$CLOUDFLARE_ACCESS_ALLOWED_EMAILS" | jq -R 'split(",") | map(gsub("^\\s+|\\s+$"; "")) | map(select(length > 0)) | length')
if [ -n "${CLOUDFLARE_ACCESS_DEVICE_POSTURE_RULE_IDS:-}" ]; then
  posture_count=$(printf '%s' "$CLOUDFLARE_ACCESS_DEVICE_POSTURE_RULE_IDS" | jq -R 'split(",") | map(gsub("^\\s+|\\s+$"; "")) | map(select(length > 0)) | length')
else
  posture_count=0
fi
[ "$email_count" -gt 0 ] || die "CLOUDFLARE_ACCESS_ALLOWED_EMAILS must include at least one approved identity"
if [ "$posture_count" -eq 0 ]; then
  log "CLOUDFLARE_ACCESS_DEVICE_POSTURE_RULE_IDS is empty; Terraform will create a WARP-required posture rule"
fi

log "Cloudflare preflight completed"
