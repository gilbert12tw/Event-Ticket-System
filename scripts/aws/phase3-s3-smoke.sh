#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
AWS_BIN=${AWS_BIN:-aws}
EVIDENCE_DIR=${PHASE3_S3_SMOKE_EVIDENCE_DIR:-}
SMOKE_PREFIX=${PHASE3_S3_SMOKE_PREFIX:-exports/phase3-smoke}
SMOKE_KEEP_OBJECT=${PHASE3_S3_SMOKE_KEEP_OBJECT:-false}
uploaded=false
smoke_file=""
put_output=""
head_output=""
delete_output=""
temp_outputs=()
bucket=""
key=""

log() {
  printf '[phase3-s3-smoke] %s\n' "$*"
}

die() {
  printf '[phase3-s3-smoke] error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

have() {
  command -v "$1" >/dev/null 2>&1
}

cleanup() {
  if [ -n "$smoke_file" ]; then
    rm -f "$smoke_file"
  fi
  if [ "${#temp_outputs[@]}" -gt 0 ]; then
    rm -f "${temp_outputs[@]}"
  fi
  if [ "$uploaded" = "true" ] && [ "$SMOKE_KEEP_OBJECT" != "true" ] && [ -n "$bucket" ] && [ -n "$key" ]; then
    "$AWS_BIN" s3api delete-object \
      --region "$region" \
      --bucket "$bucket" \
      --key "$key" \
      --output json >"${delete_output:-/dev/null}" 2>/dev/null || true
  fi
}

tf_output_raw() {
  (cd "$TF_DIR" && "$TF_BIN" output -raw "$1")
}

write_json() {
  path=$1
  jq . >"$path"
}

if [ -z "${TF_BIN:-}" ]; then
  if have terraform; then
    TF_BIN=terraform
  elif have tofu; then
    TF_BIN=tofu
  else
    TF_BIN=terraform
  fi
fi

need "$TF_BIN"
need "$AWS_BIN"
need jq

trap cleanup EXIT

region=$(tf_output_raw selected_region)
mode=$(tf_output_raw deployment_mode)
bucket=$(tf_output_raw s3_export_bucket)
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
key="${SMOKE_PREFIX%/}/$mode-$timestamp-$$.txt"
smoke_file=$(mktemp)
put_output=$(mktemp)
head_output=$(mktemp)
delete_output=$(mktemp)
temp_outputs=("$put_output" "$head_output" "$delete_output")

if [ -n "$EVIDENCE_DIR" ]; then
  mkdir -p "$EVIDENCE_DIR"
  put_output="$EVIDENCE_DIR/s3-smoke-put-object.json"
  head_output="$EVIDENCE_DIR/s3-smoke-head-object.json"
  delete_output="$EVIDENCE_DIR/s3-smoke-delete-object.json"
  temp_outputs=()
fi

cat >"$smoke_file" <<EOF
phase3 aws export bucket smoke
mode=$mode
timestamp=$timestamp
EOF

log "writing canary object s3://$bucket/$key"
"$AWS_BIN" s3api put-object \
  --region "$region" \
  --bucket "$bucket" \
  --key "$key" \
  --body "$smoke_file" \
  --content-type "text/plain; charset=utf-8" \
  --server-side-encryption AES256 \
  --output json | write_json "$put_output"
uploaded=true

"$AWS_BIN" s3api head-object \
  --region "$region" \
  --bucket "$bucket" \
  --key "$key" \
  --output json | write_json "$head_output"

content_length=$(jq -r '.ContentLength // 0' "$head_output")
encryption=$(jq -r '.ServerSideEncryption // ""' "$head_output")
[ "$content_length" -gt 0 ] || die "S3 smoke object has empty ContentLength"
[ "$encryption" = "AES256" ] || die "S3 smoke object missing AES256 server-side encryption"

if [ "$SMOKE_KEEP_OBJECT" = "true" ]; then
  log "keeping canary object for manual inspection because PHASE3_S3_SMOKE_KEEP_OBJECT=true"
else
  "$AWS_BIN" s3api delete-object \
    --region "$region" \
    --bucket "$bucket" \
    --key "$key" \
    --output json | write_json "$delete_output"
  uploaded=false
fi

cat <<EOF
[phase3-s3-smoke] ok
mode=$mode
bucket=$bucket
key=$key
content_length=$content_length
server_side_encryption=$encryption
kept_object=$SMOKE_KEEP_OBJECT
EOF
