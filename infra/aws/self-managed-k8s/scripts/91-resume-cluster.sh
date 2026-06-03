#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd aws
require_cmd jq
require_cmd kubectl
require_cmd helm
require_aws_identity
validate_node_count
validate_region
validate_purchase_option
validate_instance_type_overrides
validate_ingress_mode
validate_cets_app_env
validate_runtime_hours
validate_k8s_log_rotation

require_fresh_guardrails() {
  local guardrails_expiration current_utc
  if ! stack_exists "$GUARDRAILS_STACK_NAME"; then
    die "guardrails stack $GUARDRAILS_STACK_NAME is required before resume"
  fi
  assert_stack_in_scope "$GUARDRAILS_STACK_NAME"
  guardrails_expiration=$(stack_output "$GUARDRAILS_STACK_NAME" ExpirationUtc)
  [ -n "$guardrails_expiration" ] && [ "$guardrails_expiration" != "None" ] ||
    die "guardrails stack $GUARDRAILS_STACK_NAME does not expose ExpirationUtc"
  current_utc=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  [ "$guardrails_expiration" \> "$current_utc" ] ||
    die "guardrails ExpirationUtc $guardrails_expiration has expired; rerun 20-deploy-guardrails.sh"
  if [ "$guardrails_expiration" \> "$EXPIRATION_UTC" ]; then
    die "guardrails ExpirationUtc $guardrails_expiration extends beyond current EXPIRATION_UTC $EXPIRATION_UTC; rerun 20-deploy-guardrails.sh"
  fi
}

cluster_stack_needs_reconcile() {
  local needs_reconcile=false
  local actual expected key
  while IFS='=' read -r key expected; do
    actual=$(stack_parameter_value "$CLUSTER_STACK_NAME" "$key")
    if [ "$actual" != "$expected" ]; then
      log "cluster parameter $key is $actual; requested $expected"
      needs_reconcile=true
    fi
  done <<EOF
NodeCount=$NODE_COUNT
PurchaseOption=$PURCHASE_OPTION
PrimaryInstanceType=$CONTROL_PLANE_INSTANCE_TYPE
FallbackInstanceType1=$FALLBACK_INSTANCE_TYPE_1
FallbackInstanceType2=$FALLBACK_INSTANCE_TYPE_2
WorkerMaxCapacity=$WORKER_MAX_CAPACITY
RootVolumeGb=$ROOT_VOLUME_GB
AppIngressMode=$APP_INGRESS_MODE
EOF
  [ "$needs_reconcile" = "true" ]
}

log "checking cost plan before resume"
"$SCRIPT_DIR/10-cost-plan.sh"
# shellcheck disable=SC1091
. "$GENERATED_DIR/cost-plan.env"
[ "$SELECTED_AWS_REGION" = "$AWS_REGION" ] ||
  die "cost plan selected $SELECTED_AWS_REGION but AWS_REGION is $AWS_REGION; update AWS_REGION or rerun cost plan"

require_fresh_guardrails

if ! stack_exists "$CLUSTER_STACK_NAME"; then
  log "cluster stack $CLUSTER_STACK_NAME does not exist; deploying infra before resume"
  "$SCRIPT_DIR/30-deploy-infra.sh"
else
  assert_stack_in_scope "$CLUSTER_STACK_NAME"
  if cluster_stack_needs_reconcile; then
    log "reconciling cluster stack parameters before resume"
    "$SCRIPT_DIR/30-deploy-infra.sh"
  fi
fi
assert_stack_in_scope "$CLUSTER_STACK_NAME"

CONTROL_PLANE_ASG=$(stack_output "$CLUSTER_STACK_NAME" ControlPlaneAutoScalingGroupName)
WORKER_ASG=$(stack_output "$CLUSTER_STACK_NAME" WorkerAutoScalingGroupName)
assert_asg_in_scope "$CONTROL_PLANE_ASG"
assert_asg_in_scope "$WORKER_ASG"

log "restoring control-plane ASG $CONTROL_PLANE_ASG to $NODE_COUNT nodes"
aws_cli autoscaling update-auto-scaling-group \
  --auto-scaling-group-name "$CONTROL_PLANE_ASG" \
  --min-size "$NODE_COUNT" \
  --max-size "$NODE_COUNT" \
  --desired-capacity "$NODE_COUNT"

log "keeping worker ASG $WORKER_ASG at desired zero with max $WORKER_MAX_CAPACITY"
aws_cli autoscaling update-auto-scaling-group \
  --auto-scaling-group-name "$WORKER_ASG" \
  --min-size 0 \
  --max-size "$WORKER_MAX_CAPACITY" \
  --desired-capacity 0

log "bootstrapping kubeadm cluster after resume"
"$SCRIPT_DIR/40-bootstrap-k8s.sh"

if [ "$RESUME_DEPLOY_APP" = "true" ]; then
  log "deploying CETS app after resume"
  "$SCRIPT_DIR/50-build-and-deploy-cets.sh"
  if [ "$RESUME_VERIFY" = "true" ]; then
    "$SCRIPT_DIR/60-verify.sh"
  fi
else
  KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"
  export KUBECONFIG="$KUBECONFIG_AWS"
  kubectl get nodes -o wide
  log "RESUME_DEPLOY_APP=false; skipped app deploy and app verification"
fi

log "resume completed"
