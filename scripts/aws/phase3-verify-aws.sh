#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TF_DIR="$ROOT_DIR/infra/aws/free-tier-compose"
AWS_BIN=${AWS_BIN:-aws}
PHASE3_EXPECT_UNHEALTHY_TARGET=${PHASE3_EXPECT_UNHEALTHY_TARGET:-}
PHASE3_MIN_HEALTHY_TARGETS=${PHASE3_MIN_HEALTHY_TARGETS:-}
PHASE3_REQUIRE_LGTM_DATASOURCES=${PHASE3_REQUIRE_LGTM_DATASOURCES:-false}
PHASE3_REQUIRE_LGTM_EVIDENCE=${PHASE3_REQUIRE_LGTM_EVIDENCE:-false}

log() {
  printf '[phase3-aws-verify] %s\n' "$*"
}

die() {
  printf '[phase3-aws-verify] error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

have() {
  command -v "$1" >/dev/null 2>&1
}

tf_output_raw() {
  (cd "$TF_DIR" && "$TF_BIN" output -raw "$1")
}

curl_retry() {
  url=$1
  for _ in $(seq 1 18); do
    if curl -fsS "$url" >/dev/null; then
      return 0
    fi
    sleep 5
  done
  die "HTTPS smoke failed after retries: $url"
}

grafana_get() {
  path=$1
  curl -fsS -u "$GRAFANA_ADMIN_USER:$GRAFANA_ADMIN_PASSWORD" "$grafana_url$path"
}

grafana_proxy_get() {
  uid=$1
  path=$2
  shift 2
  curl -fsS -u "$GRAFANA_ADMIN_USER:$GRAFANA_ADMIN_PASSWORD" \
    --get "$@" \
    "$grafana_url/api/datasources/proxy/uid/$uid$path"
}

check_grafana_datasource() {
  uid=$1
  grafana_get "/api/datasources/uid/$uid" |
    grep -Eq "\"uid\"[[:space:]]*:[[:space:]]*\"$uid\"" ||
    die "Grafana datasource $uid is not provisioned"

  health=$(grafana_get "/api/datasources/uid/$uid/health" 2>/dev/null || true)
  printf '%s\n' "$health" | grep -Eiq '"status"[[:space:]]*:[[:space:]]*"(ok|success)"|datasource is working' ||
    die "Grafana datasource $uid health check failed: ${health:-empty response}"
}

check_grafana_datasources() {
  if [ -z "${GRAFANA_ADMIN_USER:-}" ] || [ -z "${GRAFANA_ADMIN_PASSWORD:-}" ]; then
    if [ "$PHASE3_REQUIRE_LGTM_DATASOURCES" = "true" ]; then
      die "GRAFANA_ADMIN_USER and GRAFANA_ADMIN_PASSWORD are required for LGTM datasource health"
    fi
    log "skipping Grafana datasource health; set GRAFANA_ADMIN_USER/GRAFANA_ADMIN_PASSWORD to enable"
    return
  fi

  for uid in Prometheus Loki Tempo Pyroscope; do
    check_grafana_datasource "$uid"
  done
}

require_grafana_credentials() {
  [ -n "${GRAFANA_ADMIN_USER:-}" ] && [ -n "${GRAFANA_ADMIN_PASSWORD:-}" ] ||
    die "GRAFANA_ADMIN_USER and GRAFANA_ADMIN_PASSWORD are required for LGTM evidence checks"
}

wait_grafana_proxy_grep() {
  label=$1
  uid=$2
  path=$3
  pattern=$4
  shift 4
  for _ in $(seq 1 18); do
    response=$(grafana_proxy_get "$uid" "$path" "$@" 2>/dev/null || true)
    if printf '%s\n' "$response" | grep -Eq "$pattern"; then
      printf '%s\n' "$response"
      return
    fi
    curl -fsS "$endpoint/readyz" >/dev/null 2>&1 || true
    sleep 5
  done
  die "$label evidence not found"
}

try_grafana_proxy_grep() {
  uid=$1
  path=$2
  pattern=$3
  shift 3
  response=$(grafana_proxy_get "$uid" "$path" "$@" 2>/dev/null || true)
  printf '%s\n' "$response" | grep -Eq "$pattern"
}

run_app_redaction_canary() {
  instance_id=$1
  canary_script=$(mktemp)
  params_json=$(mktemp)
  cat >"$canary_script" <<'SCRIPT'
docker rm -f cets-phase3-redaction-canary >/dev/null 2>&1 || true
docker run \
  --name cets-phase3-redaction-canary \
  --label com.docker.compose.service=redaction-canary \
  -d public.ecr.aws/docker/library/alpine:3.22 \
  sh -c 'printf '"'"'{"message":"phase3-redaction-canary","signed_token":"phase3-raw-signed-canary","signed_qr_token":"phase3-raw-qr-canary","provider_token":"phase3-raw-provider-canary","email_body":"phase3-raw-email-body-canary","raw_recipient_email":"phase3-raw-recipient-email-canary@example.test","raw_idempotency_key":"phase3-raw-idempotency-canary","pii":"phase3-raw-pii-canary"}\n'"'"'; sleep 120'
SCRIPT
  jq -n --rawfile command "$canary_script" '{commands: [$command]}' >"$params_json"
  command_id=$("$AWS_BIN" ssm send-command \
    --region "$region" \
    --instance-ids "$instance_id" \
    --document-name AWS-RunShellScript \
    --comment "phase3 redaction canary" \
    --parameters "file://$params_json" \
    --query 'Command.CommandId' \
    --output text)

  for _ in $(seq 1 24); do
    status=$("$AWS_BIN" ssm get-command-invocation \
      --region "$region" \
      --command-id "$command_id" \
      --instance-id "$instance_id" \
      --query Status \
      --output text 2>/dev/null || true)
    case "$status" in
      Success)
        rm -f "$canary_script" "$params_json"
        return
        ;;
      Failed | Cancelled | TimedOut | Cancelling)
        "$AWS_BIN" ssm get-command-invocation \
          --region "$region" \
          --command-id "$command_id" \
          --instance-id "$instance_id" \
          --output json >&2 || true
        rm -f "$canary_script" "$params_json"
        die "redaction canary SSM command failed with status $status"
        ;;
    esac
    sleep 5
  done

  rm -f "$canary_script" "$params_json"
  die "redaction canary SSM command did not finish"
}

check_lgtm_evidence() {
  if [ "$PHASE3_REQUIRE_LGTM_EVIDENCE" != "true" ]; then
    log "skipping deep LGTM evidence; set PHASE3_REQUIRE_LGTM_EVIDENCE=true to enable"
    return
  fi
  require_grafana_credentials

  log "checking Prometheus backend scrape evidence"
  wait_grafana_proxy_grep \
    "Prometheus cets-backend up metric" \
    Prometheus \
    /api/v1/query \
    '"value":\[[^]]+,"[0-9.]*[1-9][0-9.]*"\]' \
    --data-urlencode 'query=sum(up{job="cets-backend"})' >/dev/null

  log "checking Tempo backend trace evidence"
  trace_found=false
  for _ in $(seq 1 18); do
    for service_name in cets-backend cets-backend-0 cets-backend-1 cets-backend-2; do
      if try_grafana_proxy_grep \
        Tempo \
        /api/search \
        '"traceID"' \
        --data-urlencode "tags=service.name=$service_name" \
        --data-urlencode 'limit=1'; then
        trace_found=true
        break 2
      fi
    done
    curl -fsS "$endpoint/readyz" >/dev/null 2>&1 || true
    sleep 5
  done
  [ "$trace_found" = "true" ] || die "Tempo did not return backend traces"

  log "checking Pyroscope backend profile evidence"
  profile_found=false
  for _ in $(seq 1 18); do
    for service_name in cets-backend cets-backend-0 cets-backend-1 cets-backend-2; do
      if try_grafana_proxy_grep \
        Pyroscope \
        /pyroscope/render \
        '"numTicks":[0-9]*[1-9][0-9]*' \
        --data-urlencode "query=process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name=\"$service_name\"}" \
        --data-urlencode 'from=now-1h' \
        --data-urlencode 'until=now' \
        --data-urlencode 'maxNodes=64'; then
        profile_found=true
        break 2
      fi
    done
    sleep 5
  done
  [ "$profile_found" = "true" ] || die "Pyroscope did not return backend profile samples"

  log "checking Loki backend log evidence"
  wait_grafana_proxy_grep \
    "Loki backend trace-correlated logs" \
    Loki \
    /loki/api/v1/query_range \
    'otel_trace_id' \
    --data-urlencode 'query={service_name=~"app|backend-.*"} |= "otel_trace_id"' \
    --data-urlencode 'limit=5' >/dev/null

  log "checking app-node telemetry redaction canary"
  canary_instance_id=$(printf '%s\n' "$healthy_target_ids" | awk 'NF { print; exit }')
  [ -n "$canary_instance_id" ] || die "no healthy app target available for redaction canary"
  run_app_redaction_canary "$canary_instance_id"
  redacted_logs=$(wait_grafana_proxy_grep \
    "Loki redaction canary logs" \
    Loki \
    /loki/api/v1/query_range \
    'phase3-redaction-canary' \
    --data-urlencode 'query={service_name="redaction-canary"} |= "phase3-redaction-canary"' \
    --data-urlencode 'limit=5')
  printf '%s\n' "$redacted_logs" | grep -Eq 'phase3-raw-(signed|qr|provider|email-body|recipient-email|idempotency|pii)-canary' &&
    die "Loki contains a raw redaction canary secret"
  printf '%s\n' "$redacted_logs" | grep -q '\[REDACTED\]' ||
    die "Loki canary log was found but sensitive fields were not redacted"
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

region=$(tf_output_raw selected_region)
mode=$(tf_output_raw deployment_mode)
endpoint=$(tf_output_raw https_endpoint)
budget_name=$(tf_output_raw budget_name)
target_group_arn=$(tf_output_raw app_target_group_arn)

log "checking AWS identity and budget $budget_name"
"$ROOT_DIR/scripts/aws/phase3-identity-guard.sh" >/dev/null
account_id=$("$AWS_BIN" sts get-caller-identity --query Account --output text)
"$AWS_BIN" budgets describe-budget \
  --account-id "$account_id" \
  --budget-name "$budget_name" >/dev/null

budget_thresholds=$("$AWS_BIN" budgets describe-notifications-for-budget \
  --account-id "$account_id" \
  --budget-name "$budget_name" \
  --query 'Notifications[?NotificationType==`ACTUAL`].Threshold' \
  --output text)
for required_threshold in 1 25 90 150 180; do
  printf '%s\n' "$budget_thresholds" | tr '\t' '\n' |
    awk -v want="$required_threshold" '($1 + 0) == (want + 0) { found = 1 } END { exit found ? 0 : 1 }' ||
    die "budget $budget_name missing ACTUAL notification threshold $required_threshold"
done

log "checking ALB target health"
target_health_json=$("$AWS_BIN" elbv2 describe-target-health \
  --region "$region" \
  --target-group-arn "$target_group_arn" \
  --output json)

healthy_count=$(printf '%s\n' "$target_health_json" |
  jq '[.TargetHealthDescriptions[].TargetHealth.State | select(. == "healthy")] | length')
healthy_target_ids=$(printf '%s\n' "$target_health_json" |
  jq -r '.TargetHealthDescriptions[] | select(.TargetHealth.State == "healthy") | .Target.Id')
if [ -z "$PHASE3_MIN_HEALTHY_TARGETS" ]; then
  if [ "$mode" = "demo-ha" ]; then
    PHASE3_MIN_HEALTHY_TARGETS=2
  else
    PHASE3_MIN_HEALTHY_TARGETS=1
  fi
fi
case "$PHASE3_MIN_HEALTHY_TARGETS" in
  '' | *[!0-9]*) die "PHASE3_MIN_HEALTHY_TARGETS must be a non-negative integer" ;;
esac
if [ "$healthy_count" -lt "$PHASE3_MIN_HEALTHY_TARGETS" ]; then
  die "expected at least $PHASE3_MIN_HEALTHY_TARGETS healthy app targets; got $healthy_count"
fi

healthy_target_azs="not_checked"
if [ "$mode" = "demo-ha" ] && [ "$PHASE3_MIN_HEALTHY_TARGETS" -ge 2 ]; then
  [ -n "$healthy_target_ids" ] || die "demo-ha has no healthy app target IDs"
  # shellcheck disable=SC2086
  healthy_target_azs=$("$AWS_BIN" ec2 describe-instances \
    --region "$region" \
    --instance-ids $healthy_target_ids \
    --query 'Reservations[].Instances[].Placement.AvailabilityZone' \
    --output text)
  unique_healthy_az_count=$(printf '%s\n' "$healthy_target_azs" |
    tr '\t ' '\n' |
    awk 'NF && !seen[$0]++ { count++ } END { print count + 0 }')
  if [ "$unique_healthy_az_count" -lt 2 ]; then
    die "demo-ha requires healthy app targets across at least two AZs; got $unique_healthy_az_count ($healthy_target_azs)"
  fi
fi

if [ -n "$PHASE3_EXPECT_UNHEALTHY_TARGET" ]; then
  expected_state=$(printf '%s\n' "$target_health_json" |
    jq -r --arg id "$PHASE3_EXPECT_UNHEALTHY_TARGET" '
      .TargetHealthDescriptions[]
      | select(.Target.Id == $id)
      | .TargetHealth.State
    ')
  [ -n "$expected_state" ] || die "target $PHASE3_EXPECT_UNHEALTHY_TARGET is not registered in target group"
  if [ "$expected_state" = "healthy" ]; then
    die "target $PHASE3_EXPECT_UNHEALTHY_TARGET is still healthy during failure drill"
  fi
fi

log "checking HTTPS smoke endpoints"
curl_retry "$endpoint/healthz"
curl_retry "$endpoint/readyz"

log "checking S3 report export bucket path"
"$ROOT_DIR/scripts/aws/phase3-s3-smoke.sh" >/dev/null

log "checking Grafana endpoint from allowlisted network"
grafana_url=$(tf_output_raw grafana_url)
curl -fsS "$grafana_url/api/health" | grep -Eq '"database"[[:space:]]*:[[:space:]]*"ok"'
check_grafana_datasources
check_lgtm_evidence

cat <<EOF
[phase3-aws-verify] ok
mode=$mode
endpoint=$endpoint
healthy_targets=$healthy_count
min_healthy_targets=$PHASE3_MIN_HEALTHY_TARGETS
healthy_target_azs=$healthy_target_azs
s3_export_path=checked
grafana_url=$grafana_url
grafana_datasource_health=$([ -n "${GRAFANA_ADMIN_USER:-}" ] && [ -n "${GRAFANA_ADMIN_PASSWORD:-}" ] && printf 'checked' || printf 'skipped')
lgtm_evidence=$([ "$PHASE3_REQUIRE_LGTM_EVIDENCE" = "true" ] && printf 'checked' || printf 'skipped')
EOF
