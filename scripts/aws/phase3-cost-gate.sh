#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
OUT_DIR=${OUT_DIR:-$ROOT_DIR/infra/aws/free-tier-compose/cost-reports}
OUT_ENV=${OUT_ENV:-$ROOT_DIR/infra/aws/free-tier-compose/selected-region.env}
MAX_FORECAST_USD=${MAX_FORECAST_USD:-180}
RISK_STOP_USD=${RISK_STOP_USD:-150}
ALLOW_EXAMPLE_COSTS=${ALLOW_EXAMPLE_COSTS:-false}
PHASE3_CANDIDATE_REGIONS=${PHASE3_CANDIDATE_REGIONS:-"us-east-1 us-east-2 us-west-2"}
PHASE3_REQUIRED_COST_ITEMS=${PHASE3_REQUIRED_COST_ITEMS:-"ec2-app-baseline ec2-observability ec2-extra-demo-app-nodes rds-postgresql-single-az rds-postgresql-multi-az rds-storage-guardrail alb-hours alb-lcu-minimum ec2-ebs-gp3-app ec2-ebs-gp3-observability s3-export-storage cloudwatch-logs-guardrail data-transfer-guardrail dns-acm-guardrail"}
PHASE3_FORECAST_WINDOW_HOURS=${PHASE3_FORECAST_WINDOW_HOURS:-336}
PHASE3_DEMO_HA_WINDOW_HOURS=${PHASE3_DEMO_HA_WINDOW_HOURS:-96}
PHASE3_COST_WINDOW_START_TAIPEI=${PHASE3_COST_WINDOW_START_TAIPEI:-"2026-06-01T00:00:00+08:00"}
PHASE3_COST_WINDOW_END_TAIPEI=${PHASE3_COST_WINDOW_END_TAIPEI:-"2026-06-15T00:00:00+08:00"}

log() {
  printf '[phase3-aws-cost] %s\n' "$*"
}

die() {
  printf '[phase3-aws-cost] error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  scripts/aws/phase3-cost-gate.sh path/to/cost-input.csv

CSV columns:
  region,mode,line_item,estimated_usd,source

Example:
  us-east-1,dev-single-az,ec2-app-node,2.50,AWS Pricing Calculator 2026-05-31
  us-east-1,demo-ha,alb,4.00,AWS Pricing Calculator 2026-05-31

The script records the lowest region total and fails when the forecast reaches
the 150 USD risk threshold or exceeds the 180 USD hard cap.
On success, it also writes PHASE3_AWS_REGION to an ignored selected-region.env file.

The input must cover all required candidate regions and every required cost item
for each region, so an incomplete CSV cannot accidentally become the "cheapest"
region evidence. Override PHASE3_CANDIDATE_REGIONS or PHASE3_REQUIRED_COST_ITEMS
only for an explicit rehearsal and record the reason in the evidence directory.

The default forecast window is 336 hours, from 2026-06-01 00:00 through
2026-06-15 00:00 Asia/Taipei.
EOF
}

input=${1:-}
if [ -z "$input" ] || [ "$input" = "-h" ] || [ "$input" = "--help" ]; then
  usage
  exit 0
fi

[ -f "$input" ] || die "missing cost input CSV: $input"
command -v python3 >/dev/null 2>&1 || die "missing python3"

mkdir -p "$OUT_DIR"
timestamp=$(date -u +"%Y%m%dT%H%M%SZ")
generated_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
report="$OUT_DIR/phase3-cost-gate-$timestamp.json"

python3 - \
  "$input" \
  "$PHASE3_CANDIDATE_REGIONS" \
  "$PHASE3_REQUIRED_COST_ITEMS" \
  "$MAX_FORECAST_USD" \
  "$RISK_STOP_USD" \
  "$ALLOW_EXAMPLE_COSTS" \
  "$generated_at" \
  "$report" \
  "$OUT_ENV" \
  "$PHASE3_FORECAST_WINDOW_HOURS" \
  "$PHASE3_DEMO_HA_WINDOW_HOURS" \
  "$PHASE3_COST_WINDOW_START_TAIPEI" \
  "$PHASE3_COST_WINDOW_END_TAIPEI" <<'PY'
import csv
import json
import os
import sys
from collections import defaultdict
from pathlib import Path

(
    path,
    candidate_regions_raw,
    required_items_raw,
    max_forecast_raw,
    risk_stop_raw,
    allow_example_raw,
    generated_at,
    report_path,
    out_env_path,
    forecast_window_hours_raw,
    demo_ha_window_hours_raw,
    window_start_taipei,
    window_end_taipei,
) = sys.argv[1:14]
candidate_regions = [region for region in candidate_regions_raw.split() if region]
required_items = [item for item in required_items_raw.split() if item]
max_forecast = float(max_forecast_raw)
risk_stop = float(risk_stop_raw)
allow_example = allow_example_raw == "true"
forecast_window_hours = float(forecast_window_hours_raw)
demo_ha_window_hours = float(demo_ha_window_hours_raw)

if not candidate_regions:
    raise SystemExit("PHASE3_CANDIDATE_REGIONS is empty")
if not required_items:
    raise SystemExit("PHASE3_REQUIRED_COST_ITEMS is empty")

seen: dict[str, set[str]] = defaultdict(set)
rows: list[dict[str, object]] = []
totals: dict[str, float] = defaultdict(float)
contains_example_prices = False
with open(path, newline="", encoding="utf-8") as handle:
    reader = csv.DictReader(handle)
    expected_columns = ["region", "mode", "line_item", "estimated_usd", "source"]
    if reader.fieldnames != expected_columns:
        raise SystemExit(
            f"cost input header must be {expected_columns}, got {reader.fieldnames}"
        )
    row_count = 0
    for row_number, row in enumerate(reader, start=2):
        row_count += 1
        region = (row["region"] or "").strip()
        item = (row["line_item"] or "").strip()
        amount_raw = (row["estimated_usd"] or "").strip()
        if not region or not item:
            raise SystemExit(f"cost input row {row_number} has empty region or line_item")
        try:
            amount = float(amount_raw)
        except ValueError as exc:
            raise SystemExit(
                f"cost input row {row_number} has invalid estimated_usd={amount_raw!r}"
            ) from exc
        if amount < 0:
            raise SystemExit(f"cost input row {row_number} has negative estimated_usd")
        if region in candidate_regions:
            seen[region].add(item)
        source = (row["source"] or "").strip()
        if "example replace-before-use" in source:
            contains_example_prices = True
        totals[region] += amount
        rows.append(
            {
                "region": region,
                "mode": (row["mode"] or "").strip(),
                "line_item": item,
                "estimated_usd": round(amount, 4),
                "source": source,
            }
        )

if row_count == 0:
    raise SystemExit("cost input has no rows")

missing_regions = [region for region in candidate_regions if region not in seen]
if missing_regions:
    raise SystemExit(
        "cost input is missing candidate region coverage: " + ", ".join(missing_regions)
    )

missing_items = {
    region: [item for item in required_items if item not in seen[region]]
    for region in candidate_regions
}
missing_items = {region: items for region, items in missing_items.items() if items}
if missing_items:
    details = "; ".join(
        f"{region}: {', '.join(items)}" for region, items in missing_items.items()
    )
    raise SystemExit("cost input is missing required line items: " + details)

selected_region = min(candidate_regions, key=lambda region: (totals[region], region))
selected_total = totals[selected_region]
status = "stop" if (contains_example_prices and not allow_example) or selected_total >= risk_stop else "pass"
report = {
    "generated_at_utc": generated_at,
    "forecast_window_hours": forecast_window_hours,
    "demo_ha_window_hours": demo_ha_window_hours,
    "cost_window_start_taipei": window_start_taipei,
    "cost_window_end_taipei": window_end_taipei,
    "max_forecast_usd": max_forecast,
    "risk_stop_usd": risk_stop,
    "candidate_regions": candidate_regions,
    "required_cost_items": required_items,
    "regions": rows,
    "totals_by_region": {region: round(total, 4) for region, total in sorted(totals.items())},
    "selected_region": selected_region,
    "selected_region_total_usd": round(selected_total, 4),
    "contains_example_prices": contains_example_prices,
    "status": status,
}
Path(report_path).parent.mkdir(parents=True, exist_ok=True)
with open(report_path, "w", encoding="utf-8") as handle:
    json.dump(report, handle, indent=2, sort_keys=False)
    handle.write("\n")

print(f"selected_region={selected_region}")
print(f"selected_region_total_usd={selected_total:.4f}")
print(f"report={report_path}")

if contains_example_prices and not allow_example:
    print("cost input still contains example replace-before-use rows", file=sys.stderr)
    raise SystemExit(5)
if selected_total > max_forecast:
    print(
        f"forecast {selected_total:.4f} exceeds hard cap {max_forecast:.4f}",
        file=sys.stderr,
    )
    raise SystemExit(3)
if selected_total >= risk_stop:
    print(
        f"forecast {selected_total:.4f} reaches risk threshold {risk_stop:.4f}",
        file=sys.stderr,
    )
    raise SystemExit(4)

out_env = f"PHASE3_AWS_REGION={selected_region}\nPHASE3_COST_REPORT={report_path}\n"
Path(out_env_path).parent.mkdir(parents=True, exist_ok=True)
fd = os.open(out_env_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
with os.fdopen(fd, "w", encoding="utf-8") as handle:
    handle.write(out_env)
PY

log "cost gate passed; wrote $OUT_ENV"
