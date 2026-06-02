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

empty_matching_ecr() {
  local repos
  repos=$(aws_cli ecr describe-repositories \
    --query "repositories[?starts_with(repositoryName, '$AWS_STACK_PREFIX/')].[repositoryName,repositoryArn]" \
    --output text 2>/dev/null || true)
  [ -n "$repos" ] || return 0
  local repo arn group track image_ids
  while IFS=$'\t' read -r repo arn; do
    [ -n "$repo" ] || continue
    group=$(aws_cli ecr list-tags-for-resource \
      --resource-arn "$arn" \
      --query "tags[?Key=='CetsCleanupGroup'].Value | [0]" \
      --output text)
    track=$(aws_cli ecr list-tags-for-resource \
      --resource-arn "$arn" \
      --query "tags[?Key=='Track'].Value | [0]" \
      --output text)
    if [ "$group" != "$AWS_STACK_PREFIX" ] || [ "$track" != "aws-self-managed-k8s" ]; then
      log "skipping ECR repository $repo without matching cleanup tags"
      continue
    fi
    log "emptying ECR repository $repo"
    image_ids=$(aws_cli ecr list-images --repository-name "$repo" --query 'imageIds' --output json)
    if [ "$image_ids" != "[]" ]; then
      aws_cli ecr batch-delete-image --repository-name "$repo" --image-ids "$image_ids" >/dev/null || true
    fi
  done <<<"$repos"
}

asg_tag_value() {
  local asg_name=$1
  local key=$2
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$asg_name" \
    --query "AutoScalingGroups[0].Tags[?Key=='$key'].Value | [0]" \
    --output text
}

stack_tag_value() {
  local stack=$1
  local key=$2
  aws_cli cloudformation describe-stacks \
    --stack-name "$stack" \
    --query "Stacks[0].Tags[?Key=='$key'].Value | [0]" \
    --output text
}

scale_asg_to_zero() {
  local stack=$1
  local asg_name group track output
  for output in ControlPlaneAutoScalingGroupName WorkerAutoScalingGroupName; do
    asg_name=$(stack_output "$stack" "$output" 2>/dev/null || true)
    if [ -n "${asg_name:-}" ] && [ "$asg_name" != "None" ]; then
      if [[ "$asg_name" != "$AWS_STACK_PREFIX-"* ]]; then
        log "skipping ASG $asg_name because it does not match AWS_STACK_PREFIX=$AWS_STACK_PREFIX"
        continue
      fi
      group=$(asg_tag_value "$asg_name" CetsCleanupGroup)
      track=$(asg_tag_value "$asg_name" Track)
      if [ "$group" != "$AWS_STACK_PREFIX" ] || [ "$track" != "aws-self-managed-k8s" ]; then
        log "skipping ASG $asg_name without matching cleanup tags"
        continue
      fi
      log "scaling $asg_name to zero"
      aws_cli autoscaling update-auto-scaling-group \
        --auto-scaling-group-name "$asg_name" \
        --min-size 0 --desired-capacity 0 || true
    fi
  done
}

delete_stack_if_exists() {
  local stack=$1
  local group track
  if aws_cli cloudformation describe-stacks --stack-name "$stack" >/dev/null 2>&1; then
    if [[ "$stack" != "$AWS_STACK_PREFIX"* ]]; then
      log "skipping stack $stack because it does not match AWS_STACK_PREFIX=$AWS_STACK_PREFIX"
      return 0
    fi
    group=$(stack_tag_value "$stack" CetsCleanupGroup)
    track=$(stack_tag_value "$stack" Track)
    if [ "$group" != "$AWS_STACK_PREFIX" ] || [ "$track" != "aws-self-managed-k8s" ]; then
      log "skipping stack $stack without matching cleanup tags"
      return 0
    fi
    log "deleting stack $stack"
    aws_cli cloudformation delete-stack --stack-name "$stack"
    wait_for_stack delete "$stack"
  fi
}

if aws_cli cloudformation describe-stacks --stack-name "$CLUSTER_STACK_NAME" >/dev/null 2>&1; then
  scale_asg_to_zero "$CLUSTER_STACK_NAME"
fi
empty_matching_ecr
delete_stack_if_exists "$CLUSTER_STACK_NAME"
delete_stack_if_exists "$GUARDRAILS_STACK_NAME"
log "destroy-all completed"
