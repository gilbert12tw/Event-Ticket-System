#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
OUT_FILE=${OUT_FILE:-$ROOT_DIR/infra/aws/free-tier-compose/.env.local}
AWS_BIN=${AWS_BIN:-aws}
DEPLOYER_USER=${PHASE3_DEPLOYER_USER:-cets-phase3-deployer}
PROFILE_NAME=${PHASE3_DEPLOYER_PROFILE:-cets-phase3-deployer}
POLICY_NAME=${PHASE3_DEPLOYER_POLICY:-cets-phase3-deployer}
APPLY=${APPLY:-false}
ROTATE_ACCESS_KEY=${ROTATE_ACCESS_KEY:-true}

log() {
  printf '[phase3-aws-bootstrap] %s\n' "$*"
}

die() {
  printf '[phase3-aws-bootstrap] error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Bootstrap the least-privilege IAM user/profile used by Phase 3 AWS deploy scripts.

Defaults are dry-run. Set APPLY=true to create/update IAM resources and write an
untracked .env.local containing the generated access key.

Required current AWS identity:
  root, or set PHASE3_ALLOW_ROOT_BOOTSTRAP=true when using scripts that enforce root guards.

Optional env:
  PHASE3_DEPLOYER_USER       default: cets-phase3-deployer
  PHASE3_DEPLOYER_PROFILE    default: cets-phase3-deployer
  PHASE3_DEPLOYER_POLICY     default: cets-phase3-deployer
  OUT_FILE                   default: infra/aws/free-tier-compose/.env.local
  ROTATE_ACCESS_KEY          default: true; set false to update IAM policy only

Example:
  APPLY=true scripts/aws/phase3-bootstrap-iam.sh
  source infra/aws/free-tier-compose/.env.local
  aws configure set aws_access_key_id "$AWS_ACCESS_KEY_ID" --profile "$AWS_PROFILE_NAME"
  aws configure set aws_secret_access_key "$AWS_SECRET_ACCESS_KEY" --profile "$AWS_PROFILE_NAME"
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

command -v "$AWS_BIN" >/dev/null 2>&1 || die "missing aws CLI"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

arn=$("$AWS_BIN" sts get-caller-identity --query Arn --output text)
case "$arn" in
  arn:aws:iam::*:root)
    log "root identity confirmed for IAM bootstrap only"
    ;;
  *)
    log "non-root identity detected: $arn"
    log "continuing because IAM bootstrap can also be run by an existing admin principal"
    ;;
esac

policy_doc=$(mktemp)
cat >"$policy_doc" <<'JSON'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "Phase3Infrastructure",
      "Effect": "Allow",
      "Action": [
        "acm:*",
        "budgets:*",
        "cloudwatch:Describe*",
        "cloudwatch:Get*",
        "ec2:*",
        "elasticloadbalancing:*",
        "iam:AddRoleToInstanceProfile",
        "iam:AttachRolePolicy",
        "iam:CreateAccessKey",
        "iam:CreateInstanceProfile",
        "iam:CreatePolicy",
        "iam:CreatePolicyVersion",
        "iam:CreateRole",
        "iam:CreateUser",
        "iam:DeleteAccessKey",
        "iam:DeleteInstanceProfile",
        "iam:DeletePolicy",
        "iam:DeletePolicyVersion",
        "iam:DeleteRole",
        "iam:DeleteUser",
        "iam:DetachRolePolicy",
        "iam:Get*",
        "iam:List*",
        "iam:PassRole",
        "iam:PutRolePolicy",
        "iam:RemoveRoleFromInstanceProfile",
        "iam:Tag*",
        "iam:Untag*",
        "pricing:GetProducts",
        "rds:*",
        "s3:*",
        "ssm:DescribeInstanceInformation",
        "ssm:GetCommandInvocation",
        "ssm:ListCommandInvocations",
        "ssm:SendCommand",
        "sts:GetCallerIdentity"
      ],
      "Resource": "*"
    }
  ]
}
JSON

if [ "$APPLY" != "true" ]; then
  cat <<EOF
[phase3-aws-bootstrap] dry-run only
Would create/update:
- IAM user: $DEPLOYER_USER
- IAM policy: $POLICY_NAME
- Access key written to: $OUT_FILE
Run with APPLY=true to execute.
EOF
  rm -f "$policy_doc"
  exit 0
fi

log "creating or reusing IAM user $DEPLOYER_USER"
if ! "$AWS_BIN" iam get-user --user-name "$DEPLOYER_USER" >/dev/null 2>&1; then
  "$AWS_BIN" iam create-user --user-name "$DEPLOYER_USER" >/dev/null
fi

account_id=$("$AWS_BIN" sts get-caller-identity --query Account --output text)
policy_arn="arn:aws:iam::$account_id:policy/$POLICY_NAME"

log "creating or updating IAM policy $POLICY_NAME"
if "$AWS_BIN" iam get-policy --policy-arn "$policy_arn" >/dev/null 2>&1; then
  default_version=$("$AWS_BIN" iam get-policy --policy-arn "$policy_arn" --query 'Policy.DefaultVersionId' --output text)
  versions=$("$AWS_BIN" iam list-policy-versions --policy-arn "$policy_arn" --query 'Versions[?IsDefaultVersion==`false`].VersionId' --output text)
  for version in $versions; do
    "$AWS_BIN" iam delete-policy-version --policy-arn "$policy_arn" --version-id "$version" >/dev/null || true
  done
  "$AWS_BIN" iam create-policy-version --policy-arn "$policy_arn" --policy-document "file://$policy_doc" --set-as-default >/dev/null
  "$AWS_BIN" iam delete-policy-version --policy-arn "$policy_arn" --version-id "$default_version" >/dev/null || true
else
  policy_arn=$("$AWS_BIN" iam create-policy \
    --policy-name "$POLICY_NAME" \
    --policy-document "file://$policy_doc" \
    --query 'Policy.Arn' \
    --output text)
fi

log "attaching policy to $DEPLOYER_USER"
"$AWS_BIN" iam attach-user-policy --user-name "$DEPLOYER_USER" --policy-arn "$policy_arn"

if [ "$ROTATE_ACCESS_KEY" != "true" ]; then
  rm -f "$policy_doc"
  log "skipped access-key rotation because ROTATE_ACCESS_KEY=$ROTATE_ACCESS_KEY"
  log "updated IAM policy only"
  exit 0
fi

existing_keys=$("$AWS_BIN" iam list-access-keys --user-name "$DEPLOYER_USER" --query 'AccessKeyMetadata[].AccessKeyId' --output text)
for key_id in $existing_keys; do
  log "deleting previous access key $key_id"
  "$AWS_BIN" iam delete-access-key --user-name "$DEPLOYER_USER" --access-key-id "$key_id"
done

log "creating new access key"
access_json=$("$AWS_BIN" iam create-access-key --user-name "$DEPLOYER_USER")
access_key_id=$(printf '%s' "$access_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["AccessKey"]["AccessKeyId"])')
secret_access_key=$(printf '%s' "$access_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["AccessKey"]["SecretAccessKey"])')

install -d "$(dirname "$OUT_FILE")"
umask 077
cat >"$OUT_FILE" <<EOF
AWS_PROFILE_NAME=$PROFILE_NAME
AWS_ACCESS_KEY_ID=$access_key_id
AWS_SECRET_ACCESS_KEY=$secret_access_key
AWS_ACCOUNT_ID=$account_id
PHASE3_DEPLOYER_USER=$DEPLOYER_USER
PHASE3_DEPLOYER_POLICY_ARN=$policy_arn
EOF

rm -f "$policy_doc"
log "wrote untracked credentials helper to $OUT_FILE"
log "configure your AWS CLI profile with the values in $OUT_FILE, then run phase3-init-local-config.sh"
