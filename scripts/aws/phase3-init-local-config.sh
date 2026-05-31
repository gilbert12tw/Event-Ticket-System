#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
OUT_FILE=${OUT_FILE:-$TF_DIR/terraform.tfvars}
SELECTED_REGION_ENV=${SELECTED_REGION_ENV:-$TF_DIR/selected-region.env}
EXPECTED_COST_WINDOW_START_TAIPEI=2026-06-01T00:00:00+08:00
EXPECTED_COST_WINDOW_END_TAIPEI=2026-06-15T00:00:00+08:00
EXPECTED_BUDGET_WINDOW_START_UTC=2026-05-31_16:00
EXPECTED_BUDGET_WINDOW_END_UTC=2026-06-14_16:00

log() {
  printf '[phase3-aws-init-config] %s\n' "$*"
}

die() {
  printf '[phase3-aws-init-config] error: %s\n' "$*" >&2
  exit 1
}

need_env() {
  name=$1
  value=${!name:-}
  [ -n "$value" ] || die "missing required env $name"
}

json_value() {
  file=$1
  key=$2
  python3 - "$file" "$key" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    value = json.load(handle).get(sys.argv[2], "")
print(value)
PY
}

secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 48 | tr -d '\n'
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY'
import base64, os
print(base64.b64encode(os.urandom(48)).decode(), end="")
PY
    return
  fi
  die "openssl or python3 is required to generate secrets"
}

usage() {
  cat <<'EOF'
Create untracked infra/aws/free-tier-compose/terraform.tfvars.

Required env:
  AWS_PROFILE_NAME
  PHASE3_AWS_REGION             selected by scripts/aws/phase3-cost-gate.sh
  PHASE3_DOMAIN_NAME
  PHASE3_BUDGET_EMAIL
  PHASE3_ALLOWLIST_CIDRS        comma-separated CIDRs
  PHASE3_REPOSITORY_URL

Optional env:
  PHASE3_PUBLIC_APP_CIDRS       default: 0.0.0.0/0
  PHASE3_ACM_CERTIFICATE_ARN    default: empty, stack creates DNS-validated cert
  PHASE3_BUDGET_START_UTC       default: 2026-05-31_16:00
  PHASE3_BUDGET_END_UTC         default: 2026-06-14_16:00
  OUT_FILE                      default: infra/aws/free-tier-compose/terraform.tfvars
  SELECTED_REGION_ENV           default: infra/aws/free-tier-compose/selected-region.env

Example:
  AWS_PROFILE_NAME=cets-phase3-deployer \
  PHASE3_AWS_REGION=us-east-1 \
  PHASE3_DOMAIN_NAME=tickets.example.com \
  PHASE3_BUDGET_EMAIL=you@example.com \
  PHASE3_ALLOWLIST_CIDRS=203.0.113.10/32 \
  PHASE3_REPOSITORY_URL=https://github.com/OWNER/Event-Ticket-System.git \
  scripts/aws/phase3-init-local-config.sh
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

[ ! -e "$OUT_FILE" ] || die "$OUT_FILE already exists; refusing to overwrite"

need_env AWS_PROFILE_NAME
need_env PHASE3_AWS_REGION
need_env PHASE3_DOMAIN_NAME
need_env PHASE3_BUDGET_EMAIL
need_env PHASE3_ALLOWLIST_CIDRS
need_env PHASE3_REPOSITORY_URL

aws_region=$PHASE3_AWS_REGION
case "$aws_region" in
  us-east-1 | us-east-2 | us-west-2)
    ;;
  *)
    die "PHASE3_AWS_REGION must be one of us-east-1, us-east-2, or us-west-2"
    ;;
esac

[ -f "$SELECTED_REGION_ENV" ] || die "missing $SELECTED_REGION_ENV; run the live pricing cost gate before generating terraform.tfvars"
# shellcheck disable=SC1090
. "$SELECTED_REGION_ENV"
[ -n "${PHASE3_AWS_REGION:-}" ] || die "$SELECTED_REGION_ENV does not define PHASE3_AWS_REGION"
[ "$aws_region" = "$PHASE3_AWS_REGION" ] || die "PHASE3_AWS_REGION=$aws_region does not match cost-gate selected region $PHASE3_AWS_REGION"
[ -n "${PHASE3_COST_REPORT:-}" ] || die "$SELECTED_REGION_ENV does not define PHASE3_COST_REPORT"
[ -f "$PHASE3_COST_REPORT" ] || die "missing cost-gate report from $SELECTED_REGION_ENV: $PHASE3_COST_REPORT"
command -v python3 >/dev/null 2>&1 || die "python3 is required to validate cost and CIDR inputs"

cost_status=$(json_value "$PHASE3_COST_REPORT" status)
[ "$cost_status" = "pass" ] || die "cost-gate report status is not pass: ${cost_status:-missing}"
cost_window_start=$(json_value "$PHASE3_COST_REPORT" cost_window_start_taipei)
cost_window_end=$(json_value "$PHASE3_COST_REPORT" cost_window_end_taipei)
[ "$cost_window_start" = "$EXPECTED_COST_WINDOW_START_TAIPEI" ] ||
  die "cost gate start window must be $EXPECTED_COST_WINDOW_START_TAIPEI; got ${cost_window_start:-missing}"
[ "$cost_window_end" = "$EXPECTED_COST_WINDOW_END_TAIPEI" ] ||
  die "cost gate end window must be $EXPECTED_COST_WINDOW_END_TAIPEI; got ${cost_window_end:-missing}"

public_app_cidrs=${PHASE3_PUBLIC_APP_CIDRS:-0.0.0.0/0}
acm_certificate_arn=${PHASE3_ACM_CERTIFICATE_ARN:-}
budget_start_utc=${PHASE3_BUDGET_START_UTC:-$EXPECTED_BUDGET_WINDOW_START_UTC}
budget_end_utc=${PHASE3_BUDGET_END_UTC:-$EXPECTED_BUDGET_WINDOW_END_UTC}

case "$budget_start_utc" in
  20[0-9][0-9]-[0-1][0-9]-[0-3][0-9]_[0-2][0-9]:[0-5][0-9])
    ;;
  *)
    die "PHASE3_BUDGET_START_UTC must use AWS Budgets YYYY-MM-DD_HH:MM UTC format"
    ;;
esac
case "$budget_end_utc" in
  20[0-9][0-9]-[0-1][0-9]-[0-3][0-9]_[0-2][0-9]:[0-5][0-9])
    ;;
  *)
    die "PHASE3_BUDGET_END_UTC must use AWS Budgets YYYY-MM-DD_HH:MM UTC format"
    ;;
esac
[ "$budget_start_utc" = "$EXPECTED_BUDGET_WINDOW_START_UTC" ] ||
  die "PHASE3_BUDGET_START_UTC must be $EXPECTED_BUDGET_WINDOW_START_UTC for this two-week demo guardrail"
[ "$budget_end_utc" = "$EXPECTED_BUDGET_WINDOW_END_UTC" ] ||
  die "PHASE3_BUDGET_END_UTC must be $EXPECTED_BUDGET_WINDOW_END_UTC for this two-week demo guardrail"

validate_domain() {
  value=$1
  case "$value" in
    http://* | https://* | */* | *:* | *' '* | .* | *.)
      die "PHASE3_DOMAIN_NAME must be a hostname only, without scheme, path, port, spaces, or leading/trailing dot"
      ;;
  esac
  python3 - "$value" <<'PY' || exit 1
import re
import sys

domain = sys.argv[1]
labels = domain.split(".")
if len(labels) < 2:
    print("PHASE3_DOMAIN_NAME must include at least one dot", file=sys.stderr)
    sys.exit(1)
label_pattern = re.compile(r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$")
if any(not label_pattern.fullmatch(label) for label in labels):
    print("PHASE3_DOMAIN_NAME contains an invalid DNS label", file=sys.stderr)
    sys.exit(1)
PY
}

validate_email() {
  value=$1
  case "$value" in
    *@*.*)
      ;;
    *)
      die "PHASE3_BUDGET_EMAIL must look like an email address"
      ;;
  esac
}

validate_repository_url() {
  value=$1
  case "$value" in
    https://* | git@*:*)
      ;;
    *)
      die "PHASE3_REPOSITORY_URL must be an https:// or git@ SSH URL"
      ;;
  esac
}

validate_cidr_csv() {
  name=$1
  value=$2
  python3 - "$name" "$value" <<'PY' || exit 1
import ipaddress
import sys

name, csv = sys.argv[1], sys.argv[2]
items = [item.strip() for item in csv.split(",") if item.strip()]
if not items:
    print(f"{name} must contain at least one CIDR", file=sys.stderr)
    sys.exit(1)
for item in items:
    try:
        ipaddress.ip_network(item, strict=False)
    except ValueError as exc:
        print(f"{name} contains invalid CIDR {item}: {exc}", file=sys.stderr)
        sys.exit(1)
PY
}

validate_domain "$PHASE3_DOMAIN_NAME"
validate_email "$PHASE3_BUDGET_EMAIL"
validate_repository_url "$PHASE3_REPOSITORY_URL"
validate_cidr_csv PHASE3_ALLOWLIST_CIDRS "$PHASE3_ALLOWLIST_CIDRS"
validate_cidr_csv PHASE3_PUBLIC_APP_CIDRS "$public_app_cidrs"

format_list() {
  value=$1
  printf '['
  first=1
  old_ifs=$IFS
  IFS=,
  for item in $value; do
    trimmed=$(printf '%s' "$item" | xargs)
    [ -n "$trimmed" ] || continue
    if [ "$first" -eq 0 ]; then
      printf ', '
    fi
    printf '"%s"' "$trimmed"
    first=0
  done
  IFS=$old_ifs
  printf ']'
}

allowlist=$(format_list "$PHASE3_ALLOWLIST_CIDRS")
public_cidrs=$(format_list "$public_app_cidrs")
db_password=$(secret)
token_secret=$(secret)
provider_secret=$(secret)
grafana_password=$(secret)

mkdir -p "$(dirname "$OUT_FILE")"
umask 077
tmp_file=$(mktemp "${OUT_FILE}.tmp.XXXXXX")
cleanup_tmp() {
  if [ -n "${tmp_file:-}" ] && [ -f "$tmp_file" ]; then
    rm -f "$tmp_file"
  fi
}
trap cleanup_tmp EXIT
cat >"$tmp_file" <<EOF
aws_profile = "$AWS_PROFILE_NAME"
aws_region  = "$aws_region"

deployment_mode     = "dev-single-az"
project_name        = "cets-phase3"
domain_name         = "$PHASE3_DOMAIN_NAME"
acm_certificate_arn = "$acm_certificate_arn"
budget_email        = "$PHASE3_BUDGET_EMAIL"
budget_time_period_start = "$budget_start_utc"
budget_time_period_end   = "$budget_end_utc"

allowlist_cidrs = $allowlist
public_app_cidrs = $public_cidrs

repository_url    = "$PHASE3_REPOSITORY_URL"
repository_branch = "feature/phase3-compose-ha-lgtm-pr"

db_password            = "$db_password"
token_signing_secret   = "$token_secret"
provider_token_secret  = "$provider_secret"
grafana_admin_password = "$grafana_password"

demo_app_node_count         = 2
enable_rds_multi_az_in_demo = true
ttl_expires_on              = "2026-06-12"
EOF
mv "$tmp_file" "$OUT_FILE"
trap - EXIT

log "created $OUT_FILE"
log "review values, then run scripts/aws/phase3-preflight.sh"
