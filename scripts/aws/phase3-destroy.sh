#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
SELECTED_REGION_ENV=${SELECTED_REGION_ENV:-$TF_DIR/selected-region.env}
AWS_BIN=${AWS_BIN:-aws}
CONFIRM=${PHASE3_CONFIRM_DESTROY:-}
REASON=${PHASE3_DESTROY_REASON:-}
PHASE3_COLLECT_DESTROY_EVIDENCE=${PHASE3_COLLECT_DESTROY_EVIDENCE:-true}
PHASE3_EVIDENCE_ROOT=${PHASE3_EVIDENCE_ROOT:-$TF_DIR/cost-reports/evidence}
EVIDENCE_DIR=

log() {
  printf '[phase3-aws-destroy] %s\n' "$*"
}

die() {
  printf '[phase3-aws-destroy] error: %s\n' "$*" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
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

usage() {
  cat <<'EOF'
Usage:
  PHASE3_CONFIRM_DESTROY=destroy-cets-phase3 \
  PHASE3_DESTROY_REASON='post-demo cleanup' \
  scripts/aws/phase3-destroy.sh

This creates and applies a Terraform/OpenTofu destroy plan for the Phase 3 AWS stack. It still
refuses root deployment credentials and verifies terraform.tfvars matches the cost-gate selected
region before touching AWS.

Optional env:
  PHASE3_COLLECT_DESTROY_EVIDENCE=false  skip ignored destroy evidence files
  PHASE3_EVIDENCE_RUN_ID=...             force the evidence directory name
  TF_BIN=tofu                            use OpenTofu instead of Terraform
EOF
}

tfvars_value() {
  key=$1
  awk -F= -v key="$key" '
    $1 ~ "^[[:space:]]*" key "[[:space:]]*$" {
      value=$2
      sub(/^[[:space:]]*/, "", value)
      sub(/[[:space:]]*$/, "", value)
      gsub(/^"|"$/, "", value)
      print value
      exit
    }
  ' "$TF_DIR/terraform.tfvars"
}

configure_destroy_evidence() {
  [ "$PHASE3_COLLECT_DESTROY_EVIDENCE" = "true" ] || return 0
  if [ -z "${PHASE3_EVIDENCE_RUN_ID:-}" ]; then
    PHASE3_EVIDENCE_RUN_ID="destroy-$(date -u +%Y%m%dT%H%M%SZ)"
    export PHASE3_EVIDENCE_RUN_ID
  fi
  EVIDENCE_DIR="$PHASE3_EVIDENCE_ROOT/$PHASE3_EVIDENCE_RUN_ID"
  mkdir -p "$EVIDENCE_DIR"
  log "writing destroy evidence to $EVIDENCE_DIR"
}

record_destroy_summary() {
  [ -n "$EVIDENCE_DIR" ] || return 0
  status=$1
  account_json=${2:-{}}
  python3 - "$EVIDENCE_DIR/destroy-summary.json" "$status" "$REASON" "${PHASE3_AWS_REGION:-}" "$tfvars_region" "$account_json" <<'PY'
import json
import sys
from datetime import datetime, timezone

path, status, reason, selected_region, tfvars_region, account_json = sys.argv[1:]
try:
    account = json.loads(account_json)
except json.JSONDecodeError:
    account = {}

summary = {
    "kind": "phase3-destroy",
    "status": status,
    "destroyed": status == "destroyed",
    "timestamp_utc": datetime.now(timezone.utc).isoformat(),
    "reason": reason,
    "selected_region": selected_region,
    "terraform_tfvars_region": tfvars_region,
    "aws_account": account.get("Account", ""),
    "aws_arn": account.get("Arn", ""),
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(summary, handle, indent=2, sort_keys=True)
    handle.write("\n")
PY
}

run_logged() {
  log_path=$1
  shift
  if [ -n "$EVIDENCE_DIR" ]; then
    "$@" 2>&1 | tee "$EVIDENCE_DIR/$log_path"
    return "${PIPESTATUS[0]}"
  fi
  "$@"
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

[ "$CONFIRM" = "destroy-cets-phase3" ] || die "set PHASE3_CONFIRM_DESTROY=destroy-cets-phase3"
[ -n "$REASON" ] || die "set PHASE3_DESTROY_REASON with the cleanup reason"
[ -f "$TF_DIR/terraform.tfvars" ] || die "missing $TF_DIR/terraform.tfvars"
[ -f "$SELECTED_REGION_ENV" ] || die "missing $SELECTED_REGION_ENV"
command -v "$TF_BIN" >/dev/null 2>&1 || die "missing Terraform/OpenTofu binary: $TF_BIN"
command -v "$AWS_BIN" >/dev/null 2>&1 || die "missing aws CLI: $AWS_BIN"

# shellcheck disable=SC1090
. "$SELECTED_REGION_ENV"
[ -n "${PHASE3_AWS_REGION:-}" ] || die "$SELECTED_REGION_ENV does not define PHASE3_AWS_REGION"

tfvars_region=$(tfvars_value aws_region)
[ -n "$tfvars_region" ] || die "terraform.tfvars does not define aws_region"
[ "$tfvars_region" = "$PHASE3_AWS_REGION" ] || die "terraform.tfvars aws_region=$tfvars_region does not match cost-gate selected region $PHASE3_AWS_REGION"

configure_destroy_evidence

log "checking least-privilege AWS identity"
run_logged identity-guard.txt "$ROOT_DIR/scripts/aws/phase3-identity-guard.sh" >/dev/null
account_json=$("$AWS_BIN" sts get-caller-identity --output json)
if [ -n "$EVIDENCE_DIR" ]; then
  printf '%s\n' "$account_json" >"$EVIDENCE_DIR/aws-identity.json"
fi
record_destroy_summary "planned" "$account_json"

log "destroy reason: $REASON"
log "planning destroy"
(
  cd "$TF_DIR"
  run_logged terraform-init.txt "$TF_BIN" init -backend=false
  run_logged terraform-destroy-plan.txt "$TF_BIN" plan -destroy -out phase3-destroy.tfplan
)

log "applying destroy plan"
(
  cd "$TF_DIR"
  run_logged terraform-destroy-apply.txt "$TF_BIN" apply phase3-destroy.tfplan
  if [ -n "$EVIDENCE_DIR" ]; then
    "$TF_BIN" state list >"$EVIDENCE_DIR/terraform-state-list-after-destroy.txt"
  fi
)

record_destroy_summary "destroyed" "$account_json"

log "destroy flow complete"
