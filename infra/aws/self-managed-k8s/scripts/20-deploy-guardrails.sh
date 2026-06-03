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

[ "$DEDICATED_AWS_ACCOUNT_ACK" = "true" ] ||
  die "AWS Budgets are account-wide here; set DEDICATED_AWS_ACCOUNT_ACK=true only for a dedicated/isolated AWS account"

log "deploying guardrails stack $GUARDRAILS_STACK_NAME"
aws_cli cloudformation deploy \
  --stack-name "$GUARDRAILS_STACK_NAME" \
  --template-file "$(template_path guardrails)" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides \
    StackPrefix="$AWS_STACK_PREFIX" \
    BudgetLimitUsd="$BUDGET_LIMIT_USD" \
    ExpirationUtc="$EXPIRATION_UTC" \
    CostAlertEmail="$COST_ALERT_EMAIL" \
  --tags \
    Project=cets \
    Track=aws-self-managed-k8s \
    CetsCleanupGroup="$AWS_STACK_PREFIX"

log "guardrails deployed; cleanup expires at $EXPIRATION_UTC"
