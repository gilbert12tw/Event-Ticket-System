#!/usr/bin/env python3
"""Generate Phase 3 AWS cost-gate CSV from the AWS Pricing API."""

from __future__ import annotations

import argparse
import csv
import json
import os
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable


REGION_LOCATIONS = {
    "us-east-1": "US East (N. Virginia)",
    "us-east-2": "US East (Ohio)",
    "us-west-2": "US West (Oregon)",
}


@dataclass(frozen=True)
class Price:
    usd: float
    unit: str
    description: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate a live AWS Pricing API CSV for phase3-cost-gate.sh."
    )
    parser.add_argument(
        "--out",
        default="infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv",
        help="CSV output path.",
    )
    parser.add_argument(
        "--aws-profile",
        default=os.environ.get("AWS_PROFILE", ""),
        help="AWS CLI profile for pricing reads. Defaults to AWS_PROFILE.",
    )
    parser.add_argument("--pricing-region", default="us-east-1")
    parser.add_argument("--total-hours", type=float, default=336)
    parser.add_argument("--demo-hours", type=float, default=96)
    parser.add_argument("--app-instance-type", default="t4g.micro")
    parser.add_argument("--observability-instance-type", default="t4g.small")
    parser.add_argument("--rds-instance-class", default="db.t4g.micro")
    parser.add_argument("--demo-app-node-count", type=int, default=2)
    parser.add_argument("--app-ebs-gb", type=float, default=20)
    parser.add_argument("--observability-ebs-gb", type=float, default=30)
    parser.add_argument("--s3-export-gb", type=float, default=1)
    parser.add_argument("--assumed-alb-lcus", type=float, default=1)
    parser.add_argument("--rds-storage-guardrail-usd", type=float, default=5)
    parser.add_argument("--cloudwatch-guardrail-usd", type=float, default=5)
    parser.add_argument("--data-transfer-guardrail-usd", type=float, default=5)
    parser.add_argument("--dns-acm-guardrail-usd", type=float, default=1)
    return parser.parse_args()


def run_aws(args: argparse.Namespace, command: list[str]) -> dict:
    full_command = ["aws"]
    if args.aws_profile:
        full_command.extend(["--profile", args.aws_profile])
    full_command.extend(command)
    result = subprocess.run(
        full_command,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return json.loads(result.stdout)


def filters(values: dict[str, str]) -> list[str]:
    rendered = []
    for field, value in values.items():
        rendered.append(f"Type=TERM_MATCH,Field={field},Value={value}")
    return rendered


def ondemand_price(
    args: argparse.Namespace,
    service_code: str,
    filter_values: dict[str, str],
    units: Iterable[str],
    description_contains: str | None = None,
) -> Price:
    response = run_aws(
        args,
        [
            "pricing",
            "get-products",
            "--region",
            args.pricing_region,
            "--service-code",
            service_code,
            "--filters",
            *filters(filter_values),
            "--max-results",
            "100",
            "--output",
            "json",
        ],
    )
    accepted_units = set(units)
    candidates: list[Price] = []
    for encoded in response.get("PriceList", []):
        product = json.loads(encoded)
        for term in product.get("terms", {}).get("OnDemand", {}).values():
            for dimension in term.get("priceDimensions", {}).values():
                unit = dimension.get("unit", "")
                description = dimension.get("description", "")
                if unit not in accepted_units:
                    continue
                if description_contains and description_contains not in description:
                    continue
                if dimension.get("beginRange", "0") != "0":
                    continue
                usd = float(dimension["pricePerUnit"]["USD"])
                candidates.append(Price(usd=usd, unit=unit, description=description))
    if not candidates:
        raise RuntimeError(
            f"no pricing result for {service_code} filters={filter_values} units={accepted_units}"
        )
    return sorted(candidates, key=lambda price: price.usd)[0]


def source(price: Price, generated_at: str) -> str:
    cleaned = " ".join(price.description.replace(",", ";").split())
    return f"AWS Pricing API {generated_at} {price.unit} {cleaned}"


def add_row(
    rows: list[list[str]],
    region: str,
    mode: str,
    item: str,
    amount: float,
    src: str,
) -> None:
    rows.append([region, mode, item, f"{amount:.4f}", src.replace(",", ";")])


def rows_for_region(args: argparse.Namespace, region: str, generated_at: str) -> list[list[str]]:
    location = REGION_LOCATIONS[region]
    total_hours = args.total_hours
    demo_hours = args.demo_hours
    dev_hours = max(total_hours - demo_hours, 0)
    extra_demo_app_nodes = max(args.demo_app_node_count - 1, 0)

    app_ec2 = ondemand_price(
        args,
        "AmazonEC2",
        {
            "instanceType": args.app_instance_type,
            "location": location,
            "operatingSystem": "Linux",
            "tenancy": "Shared",
            "preInstalledSw": "NA",
            "capacitystatus": "Used",
        },
        ["Hrs"],
    )
    obs_ec2 = ondemand_price(
        args,
        "AmazonEC2",
        {
            "instanceType": args.observability_instance_type,
            "location": location,
            "operatingSystem": "Linux",
            "tenancy": "Shared",
            "preInstalledSw": "NA",
            "capacitystatus": "Used",
        },
        ["Hrs"],
    )
    rds_single = ondemand_price(
        args,
        "AmazonRDS",
        {
            "instanceType": args.rds_instance_class,
            "location": location,
            "databaseEngine": "PostgreSQL",
            "deploymentOption": "Single-AZ",
        },
        ["Hrs"],
    )
    rds_multi = ondemand_price(
        args,
        "AmazonRDS",
        {
            "instanceType": args.rds_instance_class,
            "location": location,
            "databaseEngine": "PostgreSQL",
            "deploymentOption": "Multi-AZ",
        },
        ["Hrs"],
    )
    alb_hour = ondemand_price(
        args,
        "AWSELB",
        {
            "location": location,
            "locationType": "AWS Region",
            "operation": "LoadBalancing:Application",
        },
        ["Hrs"],
        "Application LoadBalancer-hour",
    )
    alb_lcu = ondemand_price(
        args,
        "AWSELB",
        {
            "location": location,
            "locationType": "AWS Region",
            "operation": "LoadBalancing:Application",
        },
        ["LCU-Hrs"],
        "used Application load balancer capacity",
    )
    ebs_gp3 = ondemand_price(
        args,
        "AmazonEC2",
        {
            "location": location,
            "locationType": "AWS Region",
            "volumeApiName": "gp3",
        },
        ["GB-Mo"],
        "General Purpose (gp3) provisioned storage",
    )
    s3_standard = ondemand_price(
        args,
        "AmazonS3",
        {
            "location": location,
            "storageClass": "General Purpose",
            "volumeType": "Standard",
        },
        ["GB-Mo"],
    )

    month_fraction = total_hours / 730
    demo_month_fraction = demo_hours / 730
    rows: list[list[str]] = []
    add_row(
        rows,
        region,
        "dev-single-az",
        "ec2-app-baseline",
        app_ec2.usd * total_hours,
        source(app_ec2, generated_at),
    )
    add_row(
        rows,
        region,
        "dev-single-az",
        "ec2-observability",
        obs_ec2.usd * total_hours,
        source(obs_ec2, generated_at),
    )
    add_row(
        rows,
        region,
        "demo-ha",
        "ec2-extra-demo-app-nodes",
        app_ec2.usd * extra_demo_app_nodes * demo_hours,
        source(app_ec2, generated_at),
    )
    add_row(
        rows,
        region,
        "dev-single-az",
        "rds-postgresql-single-az",
        rds_single.usd * dev_hours,
        source(rds_single, generated_at),
    )
    add_row(
        rows,
        region,
        "demo-ha",
        "rds-postgresql-multi-az",
        rds_multi.usd * demo_hours,
        source(rds_multi, generated_at),
    )
    add_row(
        rows,
        region,
        "shared",
        "rds-storage-guardrail",
        args.rds_storage_guardrail_usd,
        f"guardrail estimate {generated_at} RDS storage backup snapshots",
    )
    add_row(
        rows,
        region,
        "shared",
        "alb-hours",
        alb_hour.usd * total_hours,
        source(alb_hour, generated_at),
    )
    add_row(
        rows,
        region,
        "shared",
        "alb-lcu-minimum",
        alb_lcu.usd * args.assumed_alb_lcus * total_hours,
        source(alb_lcu, generated_at),
    )
    app_ebs_gb_months = args.app_ebs_gb * month_fraction
    app_ebs_gb_months += args.app_ebs_gb * extra_demo_app_nodes * demo_month_fraction
    add_row(
        rows,
        region,
        "shared",
        "ec2-ebs-gp3-app",
        ebs_gp3.usd * app_ebs_gb_months,
        source(ebs_gp3, generated_at),
    )
    add_row(
        rows,
        region,
        "shared",
        "ec2-ebs-gp3-observability",
        ebs_gp3.usd * args.observability_ebs_gb * month_fraction,
        source(ebs_gp3, generated_at),
    )
    add_row(
        rows,
        region,
        "shared",
        "s3-export-storage",
        s3_standard.usd * args.s3_export_gb * month_fraction,
        source(s3_standard, generated_at),
    )
    add_row(
        rows,
        region,
        "shared",
        "cloudwatch-logs-guardrail",
        args.cloudwatch_guardrail_usd,
        f"guardrail estimate {generated_at} logs metrics alarms",
    )
    add_row(
        rows,
        region,
        "shared",
        "data-transfer-guardrail",
        args.data_transfer_guardrail_usd,
        f"guardrail estimate {generated_at} low demo traffic",
    )
    add_row(
        rows,
        region,
        "shared",
        "dns-acm-guardrail",
        args.dns_acm_guardrail_usd,
        f"guardrail estimate {generated_at} Route53 DNS ACM validation assumptions",
    )
    return rows


def main() -> int:
    args = parse_args()
    generated_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    output = Path(args.out)
    output.parent.mkdir(parents=True, exist_ok=True)

    rows: list[list[str]] = []
    for region in REGION_LOCATIONS:
        rows.extend(rows_for_region(args, region, generated_at))

    with output.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(["region", "mode", "line_item", "estimated_usd", "source"])
        writer.writerows(rows)
    print(output)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as exc:
        print(exc.stderr, file=sys.stderr, end="")
        raise
