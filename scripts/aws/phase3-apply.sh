#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
MODE=${1:-}
AWS_BIN=${AWS_BIN:-aws}
APPLY=${APPLY:-false}
SELECTED_REGION_ENV=${SELECTED_REGION_ENV:-$TF_DIR/selected-region.env}
PHASE3_SKIP_VERIFY=${PHASE3_SKIP_VERIFY:-false}
PHASE3_COLLECT_EVIDENCE=${PHASE3_COLLECT_EVIDENCE:-true}
PHASE3_DEMO_HA_LIGHT_VERIFY=${PHASE3_DEMO_HA_LIGHT_VERIFY:-false}
PHASE3_RUN_FAILURE_DRILL=${PHASE3_RUN_FAILURE_DRILL:-true}
PHASE3_EVIDENCE_ROOT=${PHASE3_EVIDENCE_ROOT:-$TF_DIR/cost-reports/evidence}
ALLOW_DEMO_HA_OUTSIDE_WINDOW=${PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW:-false}
NOW_TAIPEI=${PHASE3_NOW_TAIPEI:-}
EXPECTED_BUDGET_WINDOW_START_UTC=2026-05-31_16:00
EXPECTED_BUDGET_WINDOW_END_UTC=2026-06-14_16:00

log() {
  printf '[phase3-aws-apply] %s\n' "$*"
}

die() {
  printf '[phase3-aws-apply] error: %s\n' "$*" >&2
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
  APPLY=true scripts/aws/phase3-apply.sh dev-single-az
  APPLY=true scripts/aws/phase3-apply.sh demo-ha
  APPLY=true scripts/aws/phase3-apply.sh post-demo

This wrapper refuses implicit AWS changes. It verifies the least-privilege AWS identity, the live
cost-gate selected region, the cost report status, and the scheduled demo-ha window guard through
phase3-mode.sh before applying the generated plan.

Optional env:
  PHASE3_SKIP_VERIFY=true          skip phase3-verify-aws.sh after apply
  PHASE3_COLLECT_EVIDENCE=false    skip phase3-collect-evidence.sh after apply
  PHASE3_DEMO_HA_LIGHT_VERIFY=true allow demo-ha verification without deep LGTM evidence
  PHASE3_RUN_FAILURE_DRILL=false   skip automatic demo-ha app-node failure drill
  PHASE3_EVIDENCE_RUN_ID=...       force the evidence directory name used by collect/drill
  TF_BIN=tofu                      use OpenTofu instead of Terraform
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

tfvars_value_or_default() {
  key=$1
  default_value=$2
  value=$(tfvars_value "$key")
  if [ -n "$value" ]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "$default_value"
  fi
}

cost_report_value() {
  key=$1
  awk -F\" -v key="$key" '
    $2 == key {
      if ($0 ~ /"/) {
        print $4
      }
      exit
    }
  ' "$PHASE3_COST_REPORT"
}

verify_budget_notifications() {
  budget_name=$1
  expected_start=$(tfvars_value_or_default budget_time_period_start "$EXPECTED_BUDGET_WINDOW_START_UTC")
  expected_end=$(tfvars_value_or_default budget_time_period_end "$EXPECTED_BUDGET_WINDOW_END_UTC")
  account_id=$("$AWS_BIN" sts get-caller-identity --query Account --output text)
  budget_summary=$("$AWS_BIN" budgets describe-budget \
    --account-id "$account_id" \
    --budget-name "$budget_name" \
    --query 'Budget.{Amount:BudgetLimit.Amount,Unit:BudgetLimit.Unit,TimeUnit:TimeUnit,Start:TimePeriod.Start,End:TimePeriod.End}' \
    --output json)
  printf '%s\n' "$budget_summary" | grep -Eq '"Amount"[[:space:]]*:[[:space:]]*"180(\.0*)?"' ||
    die "budget $budget_name limit amount is not 180 USD"
  printf '%s\n' "$budget_summary" | grep -Eq '"Unit"[[:space:]]*:[[:space:]]*"USD"' ||
    die "budget $budget_name limit unit is not USD"
  printf '%s\n' "$budget_summary" | grep -Eq '"TimeUnit"[[:space:]]*:[[:space:]]*"MONTHLY"' ||
    die "budget $budget_name time unit is not MONTHLY"
  python3 - "$expected_start" "$expected_end" "$budget_summary" <<'PY' ||
import json
import re
import sys
from datetime import datetime, timezone


def normalize_budget_time(value: object) -> str:
    text = str(value or "").strip()
    if not text:
        return ""
    if re.fullmatch(r"\d{4}-\d{2}-\d{2}_\d{2}:\d{2}", text):
        return text
    normalized = text.replace("Z", "+00:00")
    try:
        parsed = datetime.fromisoformat(normalized)
    except ValueError:
        return text[:16].replace("T", "_")
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc).strftime("%Y-%m-%d_%H:%M")


expected_start = sys.argv[1]
expected_end = sys.argv[2]
summary = json.loads(sys.argv[3])
actual_start = normalize_budget_time(summary.get("Start"))
actual_end = normalize_budget_time(summary.get("End"))
if actual_start != expected_start or actual_end != expected_end:
    print(
        "budget window mismatch: "
        f"actual={actual_start}..{actual_end} "
        f"expected={expected_start}..{expected_end}",
        file=sys.stderr,
    )
    sys.exit(1)
PY
    die "budget $budget_name time period does not match the two-week guardrail window"

  budget_thresholds=$("$AWS_BIN" budgets describe-notifications-for-budget \
    --account-id "$account_id" \
    --budget-name "$budget_name" \
    --query 'Notifications[?NotificationType==`ACTUAL`].Threshold' \
    --output text)
  for required_threshold in 1 25 90 150 180; do
    printf '%s\n' "$budget_thresholds" | tr '\t' '\n' |
      awk -v want="$required_threshold" '($1 + 0) == (want + 0) { found = 1 } END { exit found ? 0 : 1 }' ||
      die "budget $budget_name missing ACTUAL notification threshold $required_threshold"
  done
}

apply_budget_guardrail() {
  project_name=$(tfvars_value_or_default project_name cets-phase3)
  budget_name="${project_name}-two-week-guardrail"
  budget_plan_file="$TF_DIR/phase3-budget-guardrail.tfplan"

  log "applying AWS Budgets guardrail before infrastructure"
  (
    cd "$TF_DIR"
    "$TF_BIN" plan \
      -target=aws_budgets_budget.phase3 \
      -out "$budget_plan_file"
    "$TF_BIN" apply "$budget_plan_file"
  )

  log "verifying AWS Budgets notifications before infrastructure"
  verify_budget_notifications "$budget_name"
}

configure_demo_ha_verifier() {
  [ "$MODE" = "demo-ha" ] || return 0
  [ "$PHASE3_SKIP_VERIFY" != "true" ] || return 0
  if [ "$PHASE3_DEMO_HA_LIGHT_VERIFY" = "true" ]; then
    log "demo-ha light verifier override enabled; deep LGTM evidence is not forced"
    return 0
  fi

  export PHASE3_REQUIRE_LGTM_DATASOURCES=true
  export PHASE3_REQUIRE_LGTM_EVIDENCE=true
  [ -n "${GRAFANA_ADMIN_USER:-}" ] && [ -n "${GRAFANA_ADMIN_PASSWORD:-}" ] ||
    die "demo-ha apply requires GRAFANA_ADMIN_USER and GRAFANA_ADMIN_PASSWORD for deep LGTM evidence; set PHASE3_DEMO_HA_LIGHT_VERIFY=true only for an explicit rehearsal"
}

guard_demo_ha_window_before_write() {
  [ "$MODE" = "demo-ha" ] || return 0

  args=("--mode" "$MODE")
  if [ -n "$NOW_TAIPEI" ]; then
    args+=("--now-taipei" "$NOW_TAIPEI")
  fi
  if [ "$ALLOW_DEMO_HA_OUTSIDE_WINDOW" = "true" ]; then
    args+=("--allow-outside-window")
  fi

  if ! status=$("$ROOT_DIR/scripts/aws/phase3-demo-window-guard.py" "${args[@]}" 2>&1); then
    now_value=$(printf '%s' "$status" | cut -d'|' -f3)
    windows_value=$(printf '%s' "$status" | cut -d'|' -f4-)
    die "demo-ha is outside scheduled Asia/Taipei windows at $now_value; no AWS writes were attempted; allowed windows: $windows_value; set PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW=true only for an explicit rehearsal"
  fi

  case "$status" in
    inside\|*)
      window_name=$(printf '%s' "$status" | cut -d'|' -f2)
      now_value=$(printf '%s' "$status" | cut -d'|' -f3)
      log "demo-ha schedule guard passed before AWS writes: $window_name at $now_value"
      ;;
    override\|*)
      now_value=$(printf '%s' "$status" | cut -d'|' -f3)
      windows_value=$(printf '%s' "$status" | cut -d'|' -f4-)
      log "demo-ha schedule guard override enabled before AWS writes at $now_value"
      log "allowed windows: $windows_value"
      ;;
    *)
      die "could not evaluate demo-ha schedule guard before AWS writes"
      ;;
  esac
}

configure_evidence_run() {
  if [ "$PHASE3_COLLECT_EVIDENCE" = "true" ] || {
    [ "$MODE" = "demo-ha" ] && [ "$PHASE3_RUN_FAILURE_DRILL" = "true" ];
  }; then
    if [ -z "${PHASE3_EVIDENCE_RUN_ID:-}" ]; then
      PHASE3_EVIDENCE_RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
      export PHASE3_EVIDENCE_RUN_ID
    fi
    export PHASE3_EVIDENCE_ROOT
  fi
}

run_demo_ha_failure_drill() {
  [ "$MODE" = "demo-ha" ] || return 0
  [ "$PHASE3_RUN_FAILURE_DRILL" = "true" ] || {
    log "skipping demo-ha failure drill because PHASE3_RUN_FAILURE_DRILL=false"
    return 0
  }

  log "running demo-ha app-node failure drill"
  if [ "$PHASE3_COLLECT_EVIDENCE" = "true" ]; then
    PHASE3_DRILL_EVIDENCE_DIR="$PHASE3_EVIDENCE_ROOT/$PHASE3_EVIDENCE_RUN_ID" \
      "$ROOT_DIR/scripts/aws/phase3-app-failure-drill.sh"
  else
    "$ROOT_DIR/scripts/aws/phase3-app-failure-drill.sh"
  fi
}

case "$MODE" in
  dev-single-az | demo-ha | post-demo)
    ;;
  "" | -h | --help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    die "unsupported mode: $MODE"
    ;;
esac

[ "$APPLY" = "true" ] || die "set APPLY=true to apply AWS changes"
[ -f "$TF_DIR/terraform.tfvars" ] || die "missing $TF_DIR/terraform.tfvars"
[ -f "$SELECTED_REGION_ENV" ] || die "missing $SELECTED_REGION_ENV; run live pricing and cost gate first"
command -v "$TF_BIN" >/dev/null 2>&1 || die "missing Terraform/OpenTofu binary: $TF_BIN"
command -v "$AWS_BIN" >/dev/null 2>&1 || die "missing aws CLI: $AWS_BIN"

# shellcheck disable=SC1090
. "$SELECTED_REGION_ENV"
[ -n "${PHASE3_AWS_REGION:-}" ] || die "$SELECTED_REGION_ENV does not define PHASE3_AWS_REGION"
[ -n "${PHASE3_COST_REPORT:-}" ] || die "$SELECTED_REGION_ENV does not define PHASE3_COST_REPORT"
[ -f "$PHASE3_COST_REPORT" ] || die "missing cost-gate report: $PHASE3_COST_REPORT"

tfvars_region=$(tfvars_value aws_region)
[ -n "$tfvars_region" ] || die "terraform.tfvars does not define aws_region"
[ "$tfvars_region" = "$PHASE3_AWS_REGION" ] || die "terraform.tfvars aws_region=$tfvars_region does not match cost-gate selected region $PHASE3_AWS_REGION"
tfvars_budget_start=$(tfvars_value_or_default budget_time_period_start "$EXPECTED_BUDGET_WINDOW_START_UTC")
tfvars_budget_end=$(tfvars_value_or_default budget_time_period_end "$EXPECTED_BUDGET_WINDOW_END_UTC")
[ "$tfvars_budget_start" = "$EXPECTED_BUDGET_WINDOW_START_UTC" ] ||
  die "terraform.tfvars budget_time_period_start must be $EXPECTED_BUDGET_WINDOW_START_UTC for the two-week guardrail"
[ "$tfvars_budget_end" = "$EXPECTED_BUDGET_WINDOW_END_UTC" ] ||
  die "terraform.tfvars budget_time_period_end must be $EXPECTED_BUDGET_WINDOW_END_UTC for the two-week guardrail"

cost_status=$(cost_report_value status)
[ "$cost_status" = "pass" ] || die "cost-gate report status is not pass: ${cost_status:-missing}"

log "checking least-privilege AWS identity"
"$ROOT_DIR/scripts/aws/phase3-identity-guard.sh" >/dev/null

log "format checking Terraform"
(cd "$TF_DIR" && "$TF_BIN" fmt -check)

log "validating Terraform"
(cd "$TF_DIR" && "$TF_BIN" init -backend=false && "$TF_BIN" validate)

guard_demo_ha_window_before_write
apply_budget_guardrail
configure_demo_ha_verifier
configure_evidence_run

log "planning $MODE through guarded mode helper"
"$ROOT_DIR/scripts/aws/phase3-mode.sh" "$MODE"
plan_file="$TF_DIR/phase3-${MODE}.tfplan"
[ -f "$plan_file" ] || die "expected plan file was not written: $plan_file"

log "applying $plan_file"
(cd "$TF_DIR" && "$TF_BIN" apply "$plan_file")

if [ "$PHASE3_SKIP_VERIFY" != "true" ]; then
  log "running post-apply AWS verifier"
  "$ROOT_DIR/scripts/aws/phase3-verify-aws.sh"
else
  log "skipping post-apply verifier because PHASE3_SKIP_VERIFY=true"
fi

run_demo_ha_failure_drill

if [ "$PHASE3_COLLECT_EVIDENCE" = "true" ]; then
  log "collecting post-apply evidence"
  "$ROOT_DIR/scripts/aws/phase3-collect-evidence.sh"
else
  log "skipping evidence collection because PHASE3_COLLECT_EVIDENCE=false"
fi

log "apply flow complete for $MODE"
