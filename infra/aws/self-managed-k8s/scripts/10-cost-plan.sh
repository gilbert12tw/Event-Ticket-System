#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd aws
require_cmd jq
require_aws_identity
validate_node_count
validate_region
validate_purchase_option
validate_instance_type_overrides
validate_runtime_hours

location_for_region() {
  case "$1" in
    us-east-1) printf 'US East (N. Virginia)' ;;
    us-east-2) printf 'US East (Ohio)' ;;
    us-west-2) printf 'US West (Oregon)' ;;
    *) die "no pricing location mapping for region $1" ;;
  esac
}

fallback_price() {
  case "$1" in
    m7a.4xlarge) printf '0.92736' ;;
    m6a.4xlarge) printf '0.6912' ;;
    m7i.4xlarge) printf '0.8064' ;;
    m7a.2xlarge) printf '0.46368' ;;
    m6a.2xlarge) printf '0.3456' ;;
    m7i.2xlarge) printf '0.4032' ;;
    m7a.xlarge) printf '0.23184' ;;
    m6a.xlarge) printf '0.1728' ;;
    m7i.xlarge) printf '0.2016' ;;
    r6a.xlarge) printf '0.2448' ;;
    r7a.xlarge) printf '0.3223' ;;
    r7i.xlarge) printf '0.2832' ;;
    r5a.large) printf '0.1130' ;;
    r6a.large) printf '0.1130' ;;
    r6i.large) printf '0.1260' ;;
    *) printf '1.00' ;;
  esac
}

on_demand_price() {
  local region=$1
  local instance_type=$2
  local location
  location=$(location_for_region "$region")
  local price_json price
  if ! price_json=$(aws pricing get-products \
    --region us-east-1 \
    --service-code AmazonEC2 \
    --filters \
      "Type=TERM_MATCH,Field=location,Value=$location" \
      "Type=TERM_MATCH,Field=instanceType,Value=$instance_type" \
      "Type=TERM_MATCH,Field=operatingSystem,Value=Linux" \
      "Type=TERM_MATCH,Field=tenancy,Value=Shared" \
      "Type=TERM_MATCH,Field=preInstalledSw,Value=NA" \
      "Type=TERM_MATCH,Field=capacitystatus,Value=Used" \
    --query 'PriceList[0]' \
    --output text 2>/dev/null); then
    fallback_price "$instance_type"
    return
  fi
  price=$(printf '%s\n' "$price_json" |
    jq -r '.. | objects | select(has("pricePerUnit")) | .pricePerUnit.USD? // empty' 2>/dev/null |
    head -1 || true)
  if ! awk -v price="$price" \
    'BEGIN { exit !(price ~ /^[0-9]+([.][0-9]+)?$/ && price + 0 > 0) }'; then
    fallback_price "$instance_type"
    return
  fi
  printf '%s\n' "$price"
}

spot_price() {
  local region=$1
  local instance_type=$2
  local price
  price=$(AWS_REGION="$region" aws ec2 describe-spot-price-history \
    --instance-types "$instance_type" \
    --product-descriptions "Linux/UNIX" \
    --max-results 1 \
    --query 'SpotPriceHistory[0].SpotPrice' \
    --output text 2>/dev/null || true)
  if [ -z "$price" ] || [ "$price" = "None" ]; then
    price=$(on_demand_price "$region" "$instance_type")
  fi
  printf '%s\n' "$price"
}

instance_price() {
  local region=$1
  local instance_type=$2
  if [ "$PURCHASE_OPTION" = "Spot" ]; then
    spot_price "$region" "$instance_type"
  else
    on_demand_price "$region" "$instance_type"
  fi
}

max_instance_price() {
  local region=$1
  local max_price=0
  local instance_type price
  for instance_type in \
    "$CONTROL_PLANE_INSTANCE_TYPE" \
    "$FALLBACK_INSTANCE_TYPE_1" \
    "$FALLBACK_INSTANCE_TYPE_2"; do
    price=$(instance_price "$region" "$instance_type")
    if awk -v price="$price" -v max="$max_price" 'BEGIN { exit !(price > max) }'; then
      max_price=$price
    fi
  done
  printf '%s\n' "$max_price"
}

estimate_region() {
  local region=$1
  local compute_price
  compute_price=$(max_instance_price "$region")

  local hours=$MAX_RUNTIME_HOURS
  local active_hours=$ACTIVE_RUNTIME_HOURS
  local instance_count=$NODE_COUNT
  local ebs_hourly
  local public_ipv4_hourly
  local api_nlb_hourly
  local app_nlb_hourly
  ebs_hourly=$(awk -v gb="$ROOT_VOLUME_GB" 'BEGIN { printf "%.6f", gb * 0.10 / 730 }')
  public_ipv4_hourly=$(awk -v n="$instance_count" 'BEGIN { printf "%.6f", n * 0.005 }')
  if [ "$NODE_COUNT" = "3" ]; then
    api_nlb_hourly=0.030
  else
    api_nlb_hourly=0
  fi
  if [ "$APP_INGRESS_MODE" = "nlb" ]; then
    app_nlb_hourly=0.036
  else
    app_nlb_hourly=0
  fi

  awk -v region="$region" \
    -v compute="$compute_price" \
    -v count="$instance_count" \
    -v hours="$hours" \
    -v active_hours="$active_hours" \
    -v ebs="$ebs_hourly" \
    -v ipv4="$public_ipv4_hourly" \
    -v nlb="$api_nlb_hourly" \
    -v app_nlb="$app_nlb_hourly" \
    'BEGIN {
      active_hourly = compute * count + ebs * count + ipv4
      fixed_hourly = nlb + app_nlb
      hourly = active_hourly + fixed_hourly
      total = active_hourly * active_hours + fixed_hourly * hours
      printf "%s %.6f %.2f\n", region, hourly, total
    }'
}

best_region=""
best_total=""
best_hourly=""
while IFS= read -r region; do
  [ -n "$region" ] || continue
  read -r candidate_region hourly total < <(estimate_region "$region")
  printf '%s hourly=%s estimated_total=%s active_hours=%s stack_hours=%s\n' \
    "$candidate_region" "$hourly" "$total" "$ACTIVE_RUNTIME_HOURS" "$MAX_RUNTIME_HOURS"
  if [ -z "$best_total" ] || awk -v a="$total" -v b="$best_total" 'BEGIN { exit !(a < b) }'; then
    best_region=$candidate_region
    best_hourly=$hourly
    best_total=$total
  fi
done < <(printf '%s\n' "$AWS_ALLOWED_REGIONS" | tr ',' '\n')

limit=$(awk -v budget="$BUDGET_LIMIT_USD" -v reserve="$COST_RESERVE_USD" 'BEGIN { printf "%.2f", budget - reserve }')
if awk -v total="$best_total" -v limit="$limit" 'BEGIN { exit !(total > limit) }'; then
  die "estimated cost $best_total USD exceeds deploy limit $limit USD; reduce NODE_COUNT/runtime or use Spot"
fi

cat >"$GENERATED_DIR/cost-plan.env" <<EOF
SELECTED_AWS_REGION=$best_region
ESTIMATED_HOURLY_USD=$best_hourly
ESTIMATED_TOTAL_USD=$best_total
DEPLOY_LIMIT_USD=$limit
ESTIMATED_ACTIVE_HOURS=$ACTIVE_RUNTIME_HOURS
ESTIMATED_STACK_HOURS=$MAX_RUNTIME_HOURS
EOF

log "selected $best_region at estimated total $best_total USD under deploy limit $limit USD"
