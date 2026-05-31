#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
MODE=${1:-}
ALLOW_DEMO_HA_OUTSIDE_WINDOW=${PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW:-false}
NOW_TAIPEI=${PHASE3_NOW_TAIPEI:-}

log() {
  printf '[phase3-aws-mode] %s\n' "$*"
}

die() {
  printf '[phase3-aws-mode] error: %s\n' "$*" >&2
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
  scripts/aws/phase3-mode.sh dev-single-az
  scripts/aws/phase3-mode.sh demo-ha
  scripts/aws/phase3-mode.sh post-demo

The script plans the requested deployment mode using untracked terraform.tfvars.
It does not apply automatically. Review the plan and cost gate before running apply.

demo-ha is refused outside the scheduled Asia/Taipei deploy/test and demo windows unless
PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW=true is set for an explicit rehearsal.
EOF
}

guard_demo_ha_window() {
  [ "$deployment_mode" = "demo-ha" ] || return 0

  args=("--mode" "$deployment_mode")
  if [ -n "$NOW_TAIPEI" ]; then
    args+=("--now-taipei" "$NOW_TAIPEI")
  fi
  if [ "$ALLOW_DEMO_HA_OUTSIDE_WINDOW" = "true" ]; then
    args+=("--allow-outside-window")
  fi
  if ! status=$("$ROOT_DIR/scripts/aws/phase3-demo-window-guard.py" "${args[@]}" 2>&1); then
    now_value=$(printf '%s' "$status" | cut -d'|' -f3)
    windows_value=$(printf '%s' "$status" | cut -d'|' -f4-)
    die "demo-ha is outside scheduled Asia/Taipei windows at $now_value; allowed windows: $windows_value; set PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW=true only for an explicit rehearsal"
  fi
  case "$status" in
    inside\|*)
      window_name=$(printf '%s' "$status" | cut -d'|' -f2)
      now_value=$(printf '%s' "$status" | cut -d'|' -f3)
      log "demo-ha schedule guard passed: $window_name at $now_value"
      ;;
    override\|*)
      now_value=$(printf '%s' "$status" | cut -d'|' -f3)
      windows_value=$(printf '%s' "$status" | cut -d'|' -f4-)
      log "demo-ha schedule guard override enabled at $now_value"
      log "allowed windows: $windows_value"
      ;;
    *)
      die "could not evaluate demo-ha schedule guard"
      ;;
  esac
}

case "$MODE" in
  dev-single-az | demo-ha)
    deployment_mode=$MODE
    ;;
  post-demo)
    deployment_mode=dev-single-az
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

[ -f "$TF_DIR/terraform.tfvars" ] || die "missing $TF_DIR/terraform.tfvars"
command -v "$TF_BIN" >/dev/null 2>&1 || die "missing Terraform/OpenTofu binary: $TF_BIN"
guard_demo_ha_window

plan_file="$TF_DIR/phase3-${MODE}.tfplan"
log "planning mode $deployment_mode"
(
  cd "$TF_DIR"
  "$TF_BIN" init -backend=false
  "$TF_BIN" plan \
    -var "deployment_mode=$deployment_mode" \
    -out "$plan_file"
)

cat <<EOF
[phase3-aws-mode] Plan written to $plan_file
[phase3-aws-mode] Required next checks before apply:
- Confirm latest cost estimate is below 180 USD and risk threshold is below 150 USD.
- Confirm AWS Budgets are planned or already present for 1, 25, 90, 150, and 180 USD.
- For demo-ha, confirm this is a deploy/test or demo window from goal.md.
- For post-demo, apply this plan after the demo to return to dev-single-az.
EOF
