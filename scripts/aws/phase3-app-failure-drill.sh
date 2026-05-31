#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
AWS_BIN=${AWS_BIN:-aws}
TARGET_INDEX=${TARGET_INDEX:-0}
EVIDENCE_DIR=${PHASE3_DRILL_EVIDENCE_DIR:-}
stopped_instance_id=""

log() {
  printf '[phase3-aws-drill] %s\n' "$*"
}

die() {
  printf '[phase3-aws-drill] error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

have() {
  command -v "$1" >/dev/null 2>&1
}

tf_output_json() {
  (cd "$TF_DIR" && "$TF_BIN" output -json "$1")
}

tf_output_raw() {
  (cd "$TF_DIR" && "$TF_BIN" output -raw "$1")
}

restore_stopped_instance() {
  [ -n "$stopped_instance_id" ] || return 0

  state=$("$AWS_BIN" ec2 describe-instances \
    --region "$region" \
    --instance-ids "$stopped_instance_id" \
    --query 'Reservations[0].Instances[0].State.Name' \
    --output text 2>/dev/null || true)
  case "$state" in
    running)
      stopped_instance_id=""
      return 0
      ;;
    pending)
      "$AWS_BIN" ec2 wait instance-running --region "$region" --instance-ids "$stopped_instance_id" 2>/dev/null || true
      stopped_instance_id=""
      return 0
      ;;
    stopping)
      "$AWS_BIN" ec2 wait instance-stopped --region "$region" --instance-ids "$stopped_instance_id" 2>/dev/null || true
      log "restoring app instance $stopped_instance_id after interrupted drill"
      "$AWS_BIN" ec2 start-instances --region "$region" --instance-ids "$stopped_instance_id" >/dev/null || true
      ;;
    stopped)
      log "restoring app instance $stopped_instance_id after interrupted drill"
      "$AWS_BIN" ec2 start-instances --region "$region" --instance-ids "$stopped_instance_id" >/dev/null || true
      ;;
    *)
      log "could not determine restore action for $stopped_instance_id state=$state"
      return 0
      ;;
  esac

  "$AWS_BIN" ec2 wait instance-running --region "$region" --instance-ids "$stopped_instance_id" 2>/dev/null || true
  stopped_instance_id=""
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

if [ -n "$EVIDENCE_DIR" ]; then
  mkdir -p "$EVIDENCE_DIR"
fi

write_json() {
  path=$1
  jq . >"$path"
}

write_target_health_evidence() {
  name=$1
  [ -n "$EVIDENCE_DIR" ] || return 0
  "$AWS_BIN" elbv2 describe-target-health \
    --region "$region" \
    --target-group-arn "$target_group_arn" \
    --output json | write_json "$EVIDENCE_DIR/$name"
}

region=$(tf_output_raw selected_region)
mode=$(tf_output_raw deployment_mode)
[ "$mode" = "demo-ha" ] || die "failure drill requires demo-ha mode; current mode is $mode"
target_group_arn=$(tf_output_raw app_target_group_arn)
trap restore_stopped_instance EXIT

instance_id=$(tf_output_json app_instance_ids | jq -r --arg key "$TARGET_INDEX" '.[$key]')
[ "$instance_id" != "null" ] && [ -n "$instance_id" ] || die "missing app instance for TARGET_INDEX=$TARGET_INDEX"

if [ -n "$EVIDENCE_DIR" ]; then
  cat >"$EVIDENCE_DIR/phase3-failure-drill.env" <<EOF
PHASE3_DRILL_TARGET_INDEX=$TARGET_INDEX
PHASE3_DRILL_INSTANCE_ID=$instance_id
PHASE3_DRILL_REGION=$region
PHASE3_DRILL_MODE=$mode
EOF
  write_target_health_evidence "phase3-failure-drill-before-target-health.json"
fi

log "stopping app instance $instance_id"
"$AWS_BIN" ec2 stop-instances --region "$region" --instance-ids "$instance_id" >/dev/null
stopped_instance_id=$instance_id
"$AWS_BIN" ec2 wait instance-stopped --region "$region" --instance-ids "$instance_id"
write_target_health_evidence "phase3-failure-drill-stopped-target-health.json"

log "verifying ALB routes around stopped target"
if [ -n "$EVIDENCE_DIR" ]; then
  PHASE3_MIN_HEALTHY_TARGETS=1 \
    PHASE3_EXPECT_UNHEALTHY_TARGET="$instance_id" \
    PHASE3_REQUIRE_LGTM_DATASOURCES=false \
    PHASE3_REQUIRE_LGTM_EVIDENCE=false \
    "$ROOT_DIR/scripts/aws/phase3-verify-aws.sh" |
    tee "$EVIDENCE_DIR/phase3-failure-drill-verify.txt"
else
  PHASE3_MIN_HEALTHY_TARGETS=1 \
    PHASE3_EXPECT_UNHEALTHY_TARGET="$instance_id" \
    PHASE3_REQUIRE_LGTM_DATASOURCES=false \
    PHASE3_REQUIRE_LGTM_EVIDENCE=false \
    "$ROOT_DIR/scripts/aws/phase3-verify-aws.sh"
fi

log "starting app instance $instance_id"
"$AWS_BIN" ec2 start-instances --region "$region" --instance-ids "$instance_id" >/dev/null
"$AWS_BIN" ec2 wait instance-running --region "$region" --instance-ids "$instance_id"
stopped_instance_id=""

log "waiting for restored target health"
for _ in $(seq 1 30); do
  if PHASE3_REQUIRE_LGTM_DATASOURCES=false \
    PHASE3_REQUIRE_LGTM_EVIDENCE=false \
    "$ROOT_DIR/scripts/aws/phase3-verify-aws.sh"; then
    write_target_health_evidence "phase3-failure-drill-restored-target-health.json"
    if [ -n "$EVIDENCE_DIR" ]; then
      cat >"$EVIDENCE_DIR/phase3-failure-drill.txt" <<EOF
phase3 failure drill passed
target_index=$TARGET_INDEX
stopped_instance_id=$instance_id
mode=$mode
region=$region
EOF
    fi
    exit 0
  fi
  sleep 10
done

die "app target did not recover after restart"
