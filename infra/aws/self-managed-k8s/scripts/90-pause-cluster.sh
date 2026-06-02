#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd aws
require_aws_identity
validate_region

[ "$PAUSE_DESTROYS_NODE_LOCAL_DATA_ACK" = "true" ] ||
  die "set PAUSE_DESTROYS_NODE_LOCAL_DATA_ACK=true; pause scales ASGs to zero and destroys node-local Kubernetes data"

if ! stack_exists "$CLUSTER_STACK_NAME"; then
  log "cluster stack $CLUSTER_STACK_NAME does not exist; nothing to pause"
  exit 0
fi
assert_stack_in_scope "$CLUSTER_STACK_NAME"

asg_instance_ids() {
  local asg_name=$1
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$asg_name" \
    --query 'AutoScalingGroups[0].Instances[].InstanceId' \
    --output text | tr '\t' '\n' | sed '/^$/d'
}

scale_stack_asg_to_zero() {
  local output=$1
  local asg_name
  asg_name=$(stack_output "$CLUSTER_STACK_NAME" "$output")
  [ -n "$asg_name" ] && [ "$asg_name" != "None" ] ||
    die "cluster stack output $output is missing"
  assert_asg_in_scope "$asg_name"
  log "scaling $asg_name to zero"
  aws_cli autoscaling update-auto-scaling-group \
    --auto-scaling-group-name "$asg_name" \
    --min-size 0 \
    --desired-capacity 0
}

wait_for_asg_empty() {
  local output=$1
  local asg_name deadline ids
  asg_name=$(stack_output "$CLUSTER_STACK_NAME" "$output")
  deadline=$((SECONDS + 1200))
  while [ "$SECONDS" -lt "$deadline" ]; do
    ids=$(asg_instance_ids "$asg_name")
    if [ -z "$ids" ]; then
      log "$asg_name has no remaining ASG instances"
      return 0
    fi
    log "waiting for $asg_name to terminate: $(tr '\n' ' ' <<<"$ids")"
    sleep 15
  done
  die "$asg_name still has ASG instances after pause timeout"
}

scale_stack_asg_to_zero ControlPlaneAutoScalingGroupName
scale_stack_asg_to_zero WorkerAutoScalingGroupName

if [ "$PAUSE_WAIT" = "true" ]; then
  wait_for_asg_empty ControlPlaneAutoScalingGroupName
  wait_for_asg_empty WorkerAutoScalingGroupName
fi

cat >"$GENERATED_DIR/pause-state.env" <<EOF
PAUSED_AT_UTC=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
AWS_REGION=$AWS_REGION
AWS_STACK_PREFIX=$AWS_STACK_PREFIX
NODE_COUNT=$NODE_COUNT
PURCHASE_OPTION=$PURCHASE_OPTION
ACTIVE_RUNTIME_HOURS=$ACTIVE_RUNTIME_HOURS
MAX_RUNTIME_HOURS=$MAX_RUNTIME_HOURS
EOF
rm -f "$GENERATED_DIR/kubeconfig"

log "cluster compute paused; CloudFormation, ECR, NLBs, and guardrails remain until resume or destroy-all"
