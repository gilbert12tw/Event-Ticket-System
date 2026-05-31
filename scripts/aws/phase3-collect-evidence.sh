#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
AWS_BIN=${AWS_BIN:-aws}
EVIDENCE_ROOT=${PHASE3_EVIDENCE_ROOT:-$TF_DIR/cost-reports/evidence}
RUN_ID=${PHASE3_EVIDENCE_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}
EVIDENCE_DIR="$EVIDENCE_ROOT/$RUN_ID"

log() {
  printf '[phase3-aws-evidence] %s\n' "$*"
}

die() {
  printf '[phase3-aws-evidence] error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

have() {
  command -v "$1" >/dev/null 2>&1
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
need curl
need jq

tf_output_raw() {
  (cd "$TF_DIR" && "$TF_BIN" output -raw "$1")
}

tf_output_json() {
  (cd "$TF_DIR" && "$TF_BIN" output -json "$1")
}

write_json() {
  path=$1
  jq . >"$path"
}

copy_if_present() {
  source=$1
  target=$2
  if [ -f "$source" ]; then
    cp "$source" "$target"
  fi
}

grafana_proxy_get() {
  uid=$1
  path=$2
  output=$3
  shift 3
  curl -fsS -u "$GRAFANA_ADMIN_USER:$GRAFANA_ADMIN_PASSWORD" \
    --get "$@" \
    "$grafana_url/api/datasources/proxy/uid/$uid$path" |
    write_json "$output"
}

mkdir -p "$EVIDENCE_DIR"
log "writing evidence to $EVIDENCE_DIR"

"$ROOT_DIR/scripts/aws/phase3-identity-guard.sh" >"$EVIDENCE_DIR/identity-guard.txt"
"$AWS_BIN" sts get-caller-identity --output json | write_json "$EVIDENCE_DIR/aws-identity.json"

region=$(tf_output_raw selected_region)
mode=$(tf_output_raw deployment_mode)
endpoint=$(tf_output_raw https_endpoint)
budget_name=$(tf_output_raw budget_name)
budget_limit_usd=$(tf_output_raw budget_limit_usd)
budget_time_period_start=$(tf_output_raw budget_time_period_start)
budget_time_period_end=$(tf_output_raw budget_time_period_end)
target_group_arn=$(tf_output_raw app_target_group_arn)
grafana_url=$(tf_output_raw grafana_url)
account_id=$(jq -r '.Account' "$EVIDENCE_DIR/aws-identity.json")

cat >"$EVIDENCE_DIR/summary.env" <<EOF
PHASE3_EVIDENCE_RUN_ID=$RUN_ID
PHASE3_AWS_REGION=$region
PHASE3_DEPLOYMENT_MODE=$mode
PHASE3_HTTPS_ENDPOINT=$endpoint
PHASE3_BUDGET_NAME=$budget_name
PHASE3_BUDGET_LIMIT_USD=$budget_limit_usd
PHASE3_BUDGET_TIME_PERIOD_START=$budget_time_period_start
PHASE3_BUDGET_TIME_PERIOD_END=$budget_time_period_end
PHASE3_GRAFANA_URL=$grafana_url
EOF

{
  printf '{\n'
  printf '  "selected_region": %s,\n' "$(jq -Rn --arg value "$region" '$value')"
  printf '  "deployment_mode": %s,\n' "$(jq -Rn --arg value "$mode" '$value')"
  printf '  "https_endpoint": %s,\n' "$(jq -Rn --arg value "$endpoint" '$value')"
  printf '  "budget_name": %s,\n' "$(jq -Rn --arg value "$budget_name" '$value')"
  printf '  "budget_limit_usd": %s,\n' "$(jq -Rn --arg value "$budget_limit_usd" '$value')"
  printf '  "budget_time_period_start": %s,\n' "$(jq -Rn --arg value "$budget_time_period_start" '$value')"
  printf '  "budget_time_period_end": %s,\n' "$(jq -Rn --arg value "$budget_time_period_end" '$value')"
  printf '  "budget_notification_thresholds_usd": %s,\n' "$(tf_output_json budget_notification_thresholds_usd)"
  printf '  "grafana_url": %s,\n' "$(jq -Rn --arg value "$grafana_url" '$value')"
  printf '  "app_instance_ids": %s\n' "$(tf_output_json app_instance_ids)"
  printf '}\n'
} | write_json "$EVIDENCE_DIR/terraform-safe-outputs.json"

copy_if_present "$TF_DIR/selected-region.env" "$EVIDENCE_DIR/selected-region.env"
if [ -f "$TF_DIR/selected-region.env" ]; then
  # shellcheck disable=SC1091
  . "$TF_DIR/selected-region.env"
  copy_if_present "${PHASE3_COST_REPORT:-}" "$EVIDENCE_DIR/cost-gate-report.json"
fi

"$AWS_BIN" budgets describe-budget \
  --account-id "$account_id" \
  --budget-name "$budget_name" \
  --output json | write_json "$EVIDENCE_DIR/budget.json"

"$AWS_BIN" budgets describe-notifications-for-budget \
  --account-id "$account_id" \
  --budget-name "$budget_name" \
  --output json | write_json "$EVIDENCE_DIR/budget-notifications.json"

"$AWS_BIN" elbv2 describe-target-health \
  --region "$region" \
  --target-group-arn "$target_group_arn" \
  --output json | write_json "$EVIDENCE_DIR/alb-target-health.json"

app_instance_ids=$(tf_output_json app_instance_ids | jq -r '.[]')
if [ -n "$app_instance_ids" ]; then
  # shellcheck disable=SC2086
  "$AWS_BIN" ec2 describe-instances \
    --region "$region" \
    --instance-ids $app_instance_ids \
    --query 'Reservations[].Instances[].{InstanceId:InstanceId,State:State.Name,AvailabilityZone:Placement.AvailabilityZone,PrivateIpAddress:PrivateIpAddress,PublicIpAddress:PublicIpAddress,Tags:Tags}' \
    --output json | write_json "$EVIDENCE_DIR/app-instances.json"
fi

curl -fsS "$endpoint/healthz" | write_json "$EVIDENCE_DIR/healthz.json"
curl -fsS "$endpoint/readyz" | write_json "$EVIDENCE_DIR/readyz.json"
curl -fsS "$grafana_url/api/health" | write_json "$EVIDENCE_DIR/grafana-health.json"
PHASE3_S3_SMOKE_EVIDENCE_DIR="$EVIDENCE_DIR" "$ROOT_DIR/scripts/aws/phase3-s3-smoke.sh" \
  | tee "$EVIDENCE_DIR/s3-smoke.txt"

if [ -n "${GRAFANA_ADMIN_USER:-}" ] && [ -n "${GRAFANA_ADMIN_PASSWORD:-}" ]; then
  for uid in Prometheus Loki Tempo Pyroscope; do
    curl -fsS -u "$GRAFANA_ADMIN_USER:$GRAFANA_ADMIN_PASSWORD" \
      "$grafana_url/api/datasources/uid/$uid/health" |
      write_json "$EVIDENCE_DIR/grafana-datasource-${uid}.json"
  done

  grafana_proxy_get Prometheus /api/v1/query "$EVIDENCE_DIR/prometheus-backend-up.json" \
    --data-urlencode 'query=sum(up{job="cets-backend"})'
  grafana_proxy_get Tempo /api/search "$EVIDENCE_DIR/tempo-backend-search.json" \
    --data-urlencode 'tags=service.name=cets-backend-0' \
    --data-urlencode 'limit=5'
  grafana_proxy_get Pyroscope /pyroscope/render "$EVIDENCE_DIR/pyroscope-backend-render.json" \
    --data-urlencode 'query=process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="cets-backend-0"}' \
    --data-urlencode 'from=now-1h' \
    --data-urlencode 'until=now' \
    --data-urlencode 'maxNodes=64'
  grafana_proxy_get Loki /loki/api/v1/query_range "$EVIDENCE_DIR/loki-backend-logs.json" \
    --data-urlencode 'query={service_name=~"app|backend-.*"} |= "otel_trace_id"' \
    --data-urlencode 'limit=5'
  grafana_proxy_get Loki /loki/api/v1/query_range "$EVIDENCE_DIR/loki-redaction-canary.json" \
    --data-urlencode 'query={service_name="redaction-canary"} |= "phase3-redaction-canary"' \
    --data-urlencode 'limit=5'
else
  log "skipping Grafana datasource evidence; set GRAFANA_ADMIN_USER/GRAFANA_ADMIN_PASSWORD to collect it"
fi

"$ROOT_DIR/scripts/aws/phase3-verify-aws.sh" | tee "$EVIDENCE_DIR/phase3-verify-aws.txt"

log "evidence complete: $EVIDENCE_DIR"
