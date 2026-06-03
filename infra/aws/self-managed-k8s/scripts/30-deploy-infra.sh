#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd aws
require_aws_identity
validate_node_count
validate_region
validate_purchase_option
validate_instance_type_overrides
validate_ingress_mode

"$SCRIPT_DIR/10-cost-plan.sh"
# shellcheck disable=SC1091
. "$GENERATED_DIR/cost-plan.env"

if [ "$SELECTED_AWS_REGION" != "$AWS_REGION" ]; then
  die "cost plan selected $SELECTED_AWS_REGION but AWS_REGION is $AWS_REGION; update AWS_REGION or rerun cost plan"
fi

if ! aws_cli cloudformation describe-stacks --stack-name "$GUARDRAILS_STACK_NAME" >/dev/null 2>&1; then
  die "guardrails stack $GUARDRAILS_STACK_NAME is required before cluster deploy; run 20-deploy-guardrails.sh first"
fi
guardrails_expiration=$(stack_output "$GUARDRAILS_STACK_NAME" ExpirationUtc)
[ -n "$guardrails_expiration" ] && [ "$guardrails_expiration" != "None" ] ||
  die "guardrails stack $GUARDRAILS_STACK_NAME does not expose ExpirationUtc; rerun 20-deploy-guardrails.sh"
current_utc=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
[ "$guardrails_expiration" \> "$current_utc" ] ||
  die "guardrails ExpirationUtc $guardrails_expiration has expired; rerun 20-deploy-guardrails.sh"
[ "$guardrails_expiration" \> "$EXPIRATION_UTC" ] &&
  die "guardrails ExpirationUtc $guardrails_expiration extends beyond current EXPIRATION_UTC $EXPIRATION_UTC; rerun 20-deploy-guardrails.sh"
if [ "$guardrails_expiration" != "$EXPIRATION_UTC" ]; then
  log "guardrails expires at $guardrails_expiration; current requested EXPIRATION_UTC is $EXPIRATION_UTC"
fi

log "deploying cluster stack $CLUSTER_STACK_NAME"
aws_cli cloudformation deploy \
  --stack-name "$CLUSTER_STACK_NAME" \
  --template-file "$(template_path cluster)" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides \
    StackPrefix="$AWS_STACK_PREFIX" \
    ClusterName="$AWS_STACK_PREFIX" \
    NodeCount="$NODE_COUNT" \
    PurchaseOption="$PURCHASE_OPTION" \
    PrimaryInstanceType="$CONTROL_PLANE_INSTANCE_TYPE" \
    FallbackInstanceType1="$FALLBACK_INSTANCE_TYPE_1" \
    FallbackInstanceType2="$FALLBACK_INSTANCE_TYPE_2" \
    WorkerMaxCapacity="$WORKER_MAX_CAPACITY" \
    RootVolumeGb="$ROOT_VOLUME_GB" \
    AdminCidr="$ADMIN_CIDR" \
    KeyName="$EC2_KEY_NAME" \
    AppIngressMode="$APP_INGRESS_MODE" \
  --tags \
    Project=cets \
    Track=aws-self-managed-k8s \
    CetsCleanupGroup="$AWS_STACK_PREFIX" \
    ExpiresAt="$EXPIRATION_UTC"

log "cluster stack deployed"
log "control-plane ASG: $(stack_output "$CLUSTER_STACK_NAME" ControlPlaneAutoScalingGroupName)"
log "worker ASG: $(stack_output "$CLUSTER_STACK_NAME" WorkerAutoScalingGroupName)"
log "api endpoint: $(stack_output "$CLUSTER_STACK_NAME" ApiEndpoint)"
log "app endpoint: $(stack_output "$CLUSTER_STACK_NAME" AppEndpoint)"
