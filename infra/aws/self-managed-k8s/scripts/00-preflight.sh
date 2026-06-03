#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd aws
require_cmd jq
require_cmd curl
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

if [ "$AWS_USE_SSM" != "true" ]; then
  require_cmd ssh
  require_cmd scp
  [ -n "$EC2_KEY_NAME" ] || die "EC2_KEY_NAME is required when AWS_USE_SSM=false"
  [ -n "$AWS_SSH_KEY_PATH" ] || die "AWS_SSH_KEY_PATH is required when AWS_USE_SSM=false"
fi

if [ "$APP_INGRESS_MODE" = "cloudflare" ]; then
  log "Cloudflare Tunnel mode selected; CLOUDFLARE_TUNNEL_TOKEN is required before app deploy"
fi

if command -v cfn-lint >/dev/null 2>&1; then
  cfn-lint "$AWS_K8S_DIR"/templates/*.yaml
else
  log "cfn-lint not installed; skipping local template lint"
fi

aws_cli cloudformation validate-template --template-body "file://$(template_path guardrails)" >/dev/null
aws_cli cloudformation validate-template --template-body "file://$(template_path cluster)" >/dev/null

log "preflight ok for account $AWS_ACCOUNT_ID in $AWS_REGION"
