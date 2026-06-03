#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)
AWS_K8S_DIR="$ROOT_DIR/infra/aws/self-managed-k8s"
LOCAL_ENV="$AWS_K8S_DIR/.env.aws.local"
GENERATED_DIR="$AWS_K8S_DIR/generated"

log() {
  printf '[aws-k8s] %s\n' "$*" >&2
}

die() {
  printf '[aws-k8s] error: %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

require_env() {
  local name=$1
  [ -n "${!name:-}" ] || die "$name is required; set it in $LOCAL_ENV"
}

require_apply() {
  [ "${APPLY:-false}" = "true" ] || die "set APPLY=true to perform this change"
}

load_env() {
  if [ -f "$LOCAL_ENV" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$LOCAL_ENV"
    set +a
  fi

  AWS_REGION=${AWS_REGION:-us-west-2}
  AWS_ALLOWED_REGIONS=${AWS_ALLOWED_REGIONS:-us-west-2,us-east-2,us-east-1}
  AWS_STACK_PREFIX=${AWS_STACK_PREFIX:-cets-aws-k8s}
  AWS_ACCOUNT_ID=${AWS_ACCOUNT_ID:-}
  NODE_COUNT=${NODE_COUNT:-3}
  CONTROL_PLANE_INSTANCE_TYPE=${CONTROL_PLANE_INSTANCE_TYPE:-r6a.large}
  FALLBACK_INSTANCE_TYPE_1=${FALLBACK_INSTANCE_TYPE_1:-r5a.large}
  FALLBACK_INSTANCE_TYPE_2=${FALLBACK_INSTANCE_TYPE_2:-r6i.large}
  PURCHASE_OPTION=${PURCHASE_OPTION:-OnDemand}
  ROOT_VOLUME_GB=${ROOT_VOLUME_GB:-120}
  WORKER_MAX_CAPACITY=${WORKER_MAX_CAPACITY:-0}
  BUDGET_LIMIT_USD=${BUDGET_LIMIT_USD:-190}
  COST_RESERVE_USD=${COST_RESERVE_USD:-15}
  MAX_RUNTIME_HOURS=${MAX_RUNTIME_HOURS:-336}
  ACTIVE_RUNTIME_HOURS=${ACTIVE_RUNTIME_HOURS:-$MAX_RUNTIME_HOURS}
  EXPIRATION_UTC=${EXPIRATION_UTC:-$(default_expiration_utc)}
  COST_ALERT_EMAIL=${COST_ALERT_EMAIL:-}
  DEDICATED_AWS_ACCOUNT_ACK=${DEDICATED_AWS_ACCOUNT_ACK:-false}
  ADMIN_CIDR=${ADMIN_CIDR:-0.0.0.0/32}
  EC2_KEY_NAME=${EC2_KEY_NAME:-}
  AWS_USE_SSM=${AWS_USE_SSM:-true}
  AWS_SSH_USER=${AWS_SSH_USER:-ubuntu}
  AWS_SSH_KEY_PATH=${AWS_SSH_KEY_PATH:-}
  APP_INGRESS_MODE=${APP_INGRESS_MODE:-cloudflare}
  CETS_NAMESPACE=${CETS_NAMESPACE:-cets}
  CETS_PUBLIC_HOSTNAME=${CETS_PUBLIC_HOSTNAME:-tickets.example.com}
  CETS_APP_ENV=${CETS_APP_ENV:-aws-self-managed-k8s}
  ALLOW_DEMO_APP_ENV=${ALLOW_DEMO_APP_ENV:-false}
  K8S_VERSION=${K8S_VERSION:-1.35}
  K8S_POD_CIDR=${K8S_POD_CIDR:-192.168.0.0/16}
  K8S_SERVICE_CIDR=${K8S_SERVICE_CIDR:-10.96.0.0/12}
  K8S_CONTAINER_LOG_MAX_SIZE=${K8S_CONTAINER_LOG_MAX_SIZE:-50Mi}
  K8S_CONTAINER_LOG_MAX_FILES=${K8S_CONTAINER_LOG_MAX_FILES:-10}
  POSTGRES_DB=${POSTGRES_DB:-cets}
  POSTGRES_USER=${POSTGRES_USER:-cets}
  OBJECT_STORAGE_BUCKET=${OBJECT_STORAGE_BUCKET:-cets-dev}
  OBJECT_STORAGE_ACCESS_KEY=${OBJECT_STORAGE_ACCESS_KEY:-minioadmin}
  MAILER_FROM=${MAILER_FROM:-no-reply@cets.local}
  MAILER_REDIRECT_TO=${MAILER_REDIRECT_TO:-}
  BUILD_AND_PUSH_IMAGES=${BUILD_AND_PUSH_IMAGES:-false}
  FORCE_PRIVATE_IMAGE_ROLLOUT=${FORCE_PRIVATE_IMAGE_ROLLOUT:-false}
  CETS_IMAGE_PLATFORM=${CETS_IMAGE_PLATFORM:-linux/amd64}
  BACKEND_MAX_REPLICAS=${BACKEND_MAX_REPLICAS:-6}
  FRONTEND_MAX_REPLICAS=${FRONTEND_MAX_REPLICAS:-6}
  PAUSE_WAIT=${PAUSE_WAIT:-true}
  PAUSE_DESTROYS_NODE_LOCAL_DATA_ACK=${PAUSE_DESTROYS_NODE_LOCAL_DATA_ACK:-false}
  RESUME_DEPLOY_APP=${RESUME_DEPLOY_APP:-false}
  RESUME_VERIFY=${RESUME_VERIFY:-true}
  RELEASE_BRANCH=${RELEASE_BRANCH:-release/aws-self-managed-k8s}
  CETS_RELEASE_VERSION=${CETS_RELEASE_VERSION:-}
  ARGOCD_APP_NAME=${ARGOCD_APP_NAME:-cets-aws-self-managed}
  ARGOCD_NAMESPACE=${ARGOCD_NAMESPACE:-argocd}
  ARGOCD_PROJECT=${ARGOCD_PROJECT:-default}
  ARGOCD_REPO_URL=${ARGOCD_REPO_URL:-https://github.com/gilbert12tw/Event-Ticket-System.git}
  ARGOCD_TARGET_REVISION=${ARGOCD_TARGET_REVISION:-$RELEASE_BRANCH}

  GUARDRAILS_STACK_NAME="${AWS_STACK_PREFIX}-guardrails"
  CLUSTER_STACK_NAME="${AWS_STACK_PREFIX}-cluster"
  GITOPS_APP_DIR="$AWS_K8S_DIR/gitops/app"
  GITOPS_ARGOCD_APP="$AWS_K8S_DIR/gitops/argocd/application.yaml"
  mkdir -p "$GENERATED_DIR"
}

default_expiration_utc() {
  local epoch
  epoch=$(($(date +%s) + ${MAX_RUNTIME_HOURS:-336} * 3600))
  if date -u -r "$epoch" '+%Y-%m-%dT%H:%M:%SZ' >/dev/null 2>&1; then
    date -u -r "$epoch" '+%Y-%m-%dT%H:%M:%SZ'
  else
    date -u -d "@$epoch" '+%Y-%m-%dT%H:%M:%SZ'
  fi
}

validate_node_count() {
  case "$NODE_COUNT" in
    1|3) ;;
    2) die "NODE_COUNT=2 is rejected because stacked etcd needs quorum; use 1 or 3" ;;
    *) die "NODE_COUNT must be 1 or 3, got $NODE_COUNT" ;;
  esac
}

validate_region() {
  case ",$AWS_ALLOWED_REGIONS," in
    *",$AWS_REGION,"*) ;;
    *) die "AWS_REGION=$AWS_REGION is not in AWS_ALLOWED_REGIONS=$AWS_ALLOWED_REGIONS" ;;
  esac
}

validate_purchase_option() {
  case "$PURCHASE_OPTION" in
    Spot|OnDemand) ;;
    *) die "PURCHASE_OPTION must be Spot or OnDemand, got $PURCHASE_OPTION" ;;
  esac
}

validate_instance_type_overrides() {
  [ "$CONTROL_PLANE_INSTANCE_TYPE" != "$FALLBACK_INSTANCE_TYPE_1" ] ||
    die "CONTROL_PLANE_INSTANCE_TYPE and FALLBACK_INSTANCE_TYPE_1 must be distinct for ASG MixedInstancesPolicy"
  [ "$CONTROL_PLANE_INSTANCE_TYPE" != "$FALLBACK_INSTANCE_TYPE_2" ] ||
    die "CONTROL_PLANE_INSTANCE_TYPE and FALLBACK_INSTANCE_TYPE_2 must be distinct for ASG MixedInstancesPolicy"
  [ "$FALLBACK_INSTANCE_TYPE_1" != "$FALLBACK_INSTANCE_TYPE_2" ] ||
    die "FALLBACK_INSTANCE_TYPE_1 and FALLBACK_INSTANCE_TYPE_2 must be distinct for ASG MixedInstancesPolicy"
}

validate_ingress_mode() {
  case "$APP_INGRESS_MODE" in
    cloudflare|nlb) ;;
    *) die "APP_INGRESS_MODE must be cloudflare or nlb, got $APP_INGRESS_MODE" ;;
  esac
}

validate_cets_app_env() {
  case "$CETS_APP_ENV" in
    aws-self-managed-k8s) ;;
    demo|local|test)
      [ "$ALLOW_DEMO_APP_ENV" = "true" ] ||
        die "CETS_APP_ENV=$CETS_APP_ENV enables mock profile login; set ALLOW_DEMO_APP_ENV=true only for temporary demos"
      ;;
    *) die "CETS_APP_ENV must be aws-self-managed-k8s, or demo/local/test with ALLOW_DEMO_APP_ENV=true, got $CETS_APP_ENV" ;;
  esac
}

validate_runtime_hours() {
  case "$MAX_RUNTIME_HOURS" in
    ''|*[!0-9]*) die "MAX_RUNTIME_HOURS must be a non-negative integer, got $MAX_RUNTIME_HOURS" ;;
  esac
  case "$ACTIVE_RUNTIME_HOURS" in
    ''|*[!0-9]*) die "ACTIVE_RUNTIME_HOURS must be a non-negative integer, got $ACTIVE_RUNTIME_HOURS" ;;
  esac
  [ "$ACTIVE_RUNTIME_HOURS" -le "$MAX_RUNTIME_HOURS" ] ||
    die "ACTIVE_RUNTIME_HOURS=$ACTIVE_RUNTIME_HOURS must be <= MAX_RUNTIME_HOURS=$MAX_RUNTIME_HOURS"
}

validate_k8s_log_rotation() {
  local size_number size_suffix
  [ "${#K8S_CONTAINER_LOG_MAX_SIZE}" -gt 2 ] ||
    die "K8S_CONTAINER_LOG_MAX_SIZE must be a Kubernetes quantity like 50Mi, got $K8S_CONTAINER_LOG_MAX_SIZE"
  size_number=${K8S_CONTAINER_LOG_MAX_SIZE%??}
  size_suffix=${K8S_CONTAINER_LOG_MAX_SIZE#"${size_number}"}
  case "$size_number" in
    ''|*[!0-9]*) die "K8S_CONTAINER_LOG_MAX_SIZE must start with a positive integer, got $K8S_CONTAINER_LOG_MAX_SIZE" ;;
  esac
  [ "$size_number" -ge 1 ] ||
    die "K8S_CONTAINER_LOG_MAX_SIZE must be >= 1, got $K8S_CONTAINER_LOG_MAX_SIZE"
  case "$size_suffix" in
    Ki|Mi|Gi|Ti) ;;
    *)
      die "K8S_CONTAINER_LOG_MAX_SIZE must use a binary size suffix Ki, Mi, Gi, or Ti; got $K8S_CONTAINER_LOG_MAX_SIZE"
      ;;
  esac

  case "$K8S_CONTAINER_LOG_MAX_FILES" in
    ''|*[!0-9]*) die "K8S_CONTAINER_LOG_MAX_FILES must be a positive integer, got $K8S_CONTAINER_LOG_MAX_FILES" ;;
  esac
  [ "$K8S_CONTAINER_LOG_MAX_FILES" -ge 1 ] ||
    die "K8S_CONTAINER_LOG_MAX_FILES must be >= 1, got $K8S_CONTAINER_LOG_MAX_FILES"
  [ "$K8S_CONTAINER_LOG_MAX_FILES" -le 100 ] ||
    die "K8S_CONTAINER_LOG_MAX_FILES must be <= 100, got $K8S_CONTAINER_LOG_MAX_FILES"
}

aws_cli() {
  AWS_REGION="$AWS_REGION" aws "$@"
}

require_aws_identity() {
  require_cmd aws
  local account
  account=$(aws_cli sts get-caller-identity --query Account --output text)
  if [ -n "$AWS_ACCOUNT_ID" ] && [ "$account" != "$AWS_ACCOUNT_ID" ]; then
    die "AWS credentials point to account $account, expected $AWS_ACCOUNT_ID"
  fi
  AWS_ACCOUNT_ID=$account
}

stack_output() {
  local stack=$1
  local key=$2
  aws_cli cloudformation describe-stacks \
    --stack-name "$stack" \
    --query "Stacks[0].Outputs[?OutputKey=='$key'].OutputValue | [0]" \
    --output text
}

stack_exists() {
  local stack=$1
  aws_cli cloudformation describe-stacks --stack-name "$stack" >/dev/null 2>&1
}

stack_tag_value() {
  local stack=$1
  local key=$2
  aws_cli cloudformation describe-stacks \
    --stack-name "$stack" \
    --query "Stacks[0].Tags[?Key=='$key'].Value | [0]" \
    --output text
}

stack_parameter_value() {
  local stack=$1
  local key=$2
  aws_cli cloudformation describe-stacks \
    --stack-name "$stack" \
    --query "Stacks[0].Parameters[?ParameterKey=='$key'].ParameterValue | [0]" \
    --output text
}

asg_tag_value() {
  local asg_name=$1
  local key=$2
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$asg_name" \
    --query "AutoScalingGroups[0].Tags[?Key=='$key'].Value | [0]" \
    --output text
}

assert_stack_in_scope() {
  local stack=$1
  local group track
  [[ "$stack" == "$AWS_STACK_PREFIX"* ]] ||
    die "stack $stack does not match AWS_STACK_PREFIX=$AWS_STACK_PREFIX"
  group=$(stack_tag_value "$stack" CetsCleanupGroup)
  track=$(stack_tag_value "$stack" Track)
  [ "$group" = "$AWS_STACK_PREFIX" ] && [ "$track" = "aws-self-managed-k8s" ] ||
    die "stack $stack is not tagged for CetsCleanupGroup=$AWS_STACK_PREFIX and Track=aws-self-managed-k8s"
}

assert_asg_in_scope() {
  local asg_name=$1
  local group track
  [[ "$asg_name" == "$AWS_STACK_PREFIX-"* ]] ||
    die "ASG $asg_name does not match AWS_STACK_PREFIX=$AWS_STACK_PREFIX"
  group=$(asg_tag_value "$asg_name" CetsCleanupGroup)
  track=$(asg_tag_value "$asg_name" Track)
  [ "$group" = "$AWS_STACK_PREFIX" ] && [ "$track" = "aws-self-managed-k8s" ] ||
    die "ASG $asg_name is not tagged for CetsCleanupGroup=$AWS_STACK_PREFIX and Track=aws-self-managed-k8s"
}

wait_for_stack() {
  local action=$1
  local stack=$2
  aws_cli cloudformation wait "stack-${action}-complete" --stack-name "$stack"
}

template_path() {
  printf '%s/templates/%s.yaml\n' "$AWS_K8S_DIR" "$1"
}

ssh_prefix() {
  [ -n "$AWS_SSH_KEY_PATH" ] || die "AWS_SSH_KEY_PATH is required when AWS_USE_SSM=false"
  printf 'ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i %q %q@' "$AWS_SSH_KEY_PATH" "$AWS_SSH_USER"
}

remote_private_ips() {
  local asg=$1
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$asg" \
    --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`].InstanceId' \
    --output text |
    tr '\t' '\n' |
    xargs aws_cli ec2 describe-instances --instance-ids \
      --query 'Reservations[].Instances[].PrivateIpAddress' --output text |
    tr '\t' '\n' | sort
}
