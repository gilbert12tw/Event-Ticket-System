#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
MAX_FORECAST_USD=${MAX_FORECAST_USD:-180}
RISK_STOP_USD=${RISK_STOP_USD:-150}
SELECTED_REGION_ENV=${SELECTED_REGION_ENV:-$TF_DIR/selected-region.env}
EXPECTED_COST_WINDOW_START_TAIPEI=2026-06-01T00:00:00+08:00
EXPECTED_COST_WINDOW_END_TAIPEI=2026-06-15T00:00:00+08:00
EXPECTED_BUDGET_WINDOW_START_UTC=2026-05-31_16:00
EXPECTED_BUDGET_WINDOW_END_UTC=2026-06-14_16:00

log() {
  printf '[phase3-aws-preflight] %s\n' "$*"
}

die() {
  printf '[phase3-aws-preflight] error: %s\n' "$*" >&2
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

[ -f "$TF_DIR/terraform.tfvars" ] || die "missing $TF_DIR/terraform.tfvars; copy terraform.tfvars.example and fill untracked values"
[ -f "$SELECTED_REGION_ENV" ] || die "missing $SELECTED_REGION_ENV; run scripts/aws/phase3-live-price-csv.py and scripts/aws/phase3-cost-gate.sh before preflight"
have "$TF_BIN" || die "missing Terraform/OpenTofu binary: $TF_BIN"
have aws || die "missing aws CLI"
have python3 || die "missing python3"

# shellcheck disable=SC1090
. "$SELECTED_REGION_ENV"
[ -n "${PHASE3_AWS_REGION:-}" ] || die "$SELECTED_REGION_ENV does not define PHASE3_AWS_REGION"
[ -n "${PHASE3_COST_REPORT:-}" ] || die "$SELECTED_REGION_ENV does not define PHASE3_COST_REPORT"
[ -f "$PHASE3_COST_REPORT" ] || die "missing cost-gate report from $SELECTED_REGION_ENV: $PHASE3_COST_REPORT"

tfvars_region=$(tfvars_value aws_region)
[ -n "$tfvars_region" ] || die "terraform.tfvars does not define aws_region"
[ "$tfvars_region" = "$PHASE3_AWS_REGION" ] || die "terraform.tfvars aws_region=$tfvars_region does not match cost-gate selected region $PHASE3_AWS_REGION"

tfvars_budget_start=$(tfvars_value budget_time_period_start)
tfvars_budget_end=$(tfvars_value budget_time_period_end)
[ "${tfvars_budget_start:-$EXPECTED_BUDGET_WINDOW_START_UTC}" = "$EXPECTED_BUDGET_WINDOW_START_UTC" ] ||
  die "terraform.tfvars budget_time_period_start must be $EXPECTED_BUDGET_WINDOW_START_UTC for the two-week guardrail"
[ "${tfvars_budget_end:-$EXPECTED_BUDGET_WINDOW_END_UTC}" = "$EXPECTED_BUDGET_WINDOW_END_UTC" ] ||
  die "terraform.tfvars budget_time_period_end must be $EXPECTED_BUDGET_WINDOW_END_UTC for the two-week guardrail"

cost_status=$(awk -F\" '/"status"/ { print $4; exit }' "$PHASE3_COST_REPORT")
[ "$cost_status" = "pass" ] || die "cost-gate report status is not pass: ${cost_status:-missing}"
cost_window_start=$(python3 - "$PHASE3_COST_REPORT" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    print(json.load(handle).get("cost_window_start_taipei", "missing"))
PY
)
cost_window_end=$(python3 - "$PHASE3_COST_REPORT" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    print(json.load(handle).get("cost_window_end_taipei", "missing"))
PY
)
[ "$cost_window_start" = "$EXPECTED_COST_WINDOW_START_TAIPEI" ] ||
  die "cost gate start window must be $EXPECTED_COST_WINDOW_START_TAIPEI; got $cost_window_start"
[ "$cost_window_end" = "$EXPECTED_COST_WINDOW_END_TAIPEI" ] ||
  die "cost gate end window must be $EXPECTED_COST_WINDOW_END_TAIPEI; got $cost_window_end"

log "checking AWS identity"
"$ROOT_DIR/scripts/aws/phase3-identity-guard.sh" >/dev/null

log "format checking Terraform"
(cd "$TF_DIR" && "$TF_BIN" fmt -check)

log "initializing Terraform without backend"
(cd "$TF_DIR" && "$TF_BIN" init -backend=false)

log "validating Terraform"
(cd "$TF_DIR" && "$TF_BIN" validate)

log "planning Terraform"
(cd "$TF_DIR" && "$TF_BIN" plan -out phase3.tfplan)

cat <<EOF
[phase3-aws-preflight] cost gate verified:
- Selected region: $PHASE3_AWS_REGION
- Cost report: $PHASE3_COST_REPORT
- Forecast window: $cost_window_start to $cost_window_end
- Stop if forecast > ${MAX_FORECAST_USD} USD.
- Stop or scale down if risk threshold >= ${RISK_STOP_USD} USD.
- Confirm AWS Budgets notifications are in the plan for 1, 25, 90, 150, and 180 USD before apply.
EOF
