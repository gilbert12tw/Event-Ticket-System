#!/usr/bin/env python3
"""Audit whether local Phase 3 AWS evidence is strong enough to close goal.md."""

from __future__ import annotations

import argparse
import json
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
TF_DIR = ROOT / "infra/aws/free-tier-compose"
DEFAULT_EVIDENCE_ROOT = TF_DIR / "cost-reports/evidence"
REQUIRED_BUDGET_THRESHOLDS = {1, 25, 90, 150, 180}
REQUIRED_DATASOURCES = ("Prometheus", "Loki", "Tempo", "Pyroscope")
EXPECTED_COST_WINDOW_START_TAIPEI = "2026-06-01T00:00:00+08:00"
EXPECTED_COST_WINDOW_END_TAIPEI = "2026-06-15T00:00:00+08:00"
EXPECTED_BUDGET_WINDOW_START_UTC = "2026-05-31_16:00"
EXPECTED_BUDGET_WINDOW_END_UTC = "2026-06-14_16:00"


@dataclass
class Check:
    name: str
    passed: bool
    detail: str


def run_text(command: list[str]) -> str:
    result = subprocess.run(
        command,
        cwd=ROOT,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return result.stdout.strip()


def read_json(path: Path) -> Any:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def parse_env_file(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    if not path.exists():
        return values
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip('"')
    return values


def tfvars_value(path: Path, key: str) -> str | None:
    if not path.exists():
        return None
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        found_key, value = line.split("=", 1)
        if found_key.strip() == key:
            return value.strip().strip('"')
    return None


def latest_evidence_dir(root: Path) -> Path | None:
    if not root.exists():
        return None
    candidates = [path for path in root.iterdir() if path.is_dir()]
    return sorted(candidates)[-1] if candidates else None


def evidence_dirs(root: Path) -> list[Path]:
    if not root.exists():
        return []
    return sorted(path for path in root.iterdir() if path.is_dir())


def evidence_mode(path: Path) -> str | None:
    safe_outputs = path / "terraform-safe-outputs.json"
    if not safe_outputs.exists():
        return None
    try:
        return str(read_json(safe_outputs).get("deployment_mode") or "")
    except (OSError, json.JSONDecodeError):
        return None


def latest_evidence_dir_by_mode(root: Path, mode: str) -> Path | None:
    matches = [path for path in evidence_dirs(root) if evidence_mode(path) == mode]
    return matches[-1] if matches else None


def is_destroy_evidence_dir(path: Path) -> bool:
    return (path / "destroy-summary.json").exists()


def latest_destroy_evidence_dir(root: Path) -> Path | None:
    matches = [path for path in evidence_dirs(root) if is_destroy_evidence_dir(path)]
    return matches[-1] if matches else None


def budget_thresholds_from_notifications(path: Path) -> set[int]:
    data = read_json(path)
    thresholds: set[int] = set()
    for item in data.get("Notifications", []):
        if item.get("NotificationType") != "ACTUAL":
            continue
        try:
            thresholds.add(int(float(item["Threshold"])))
        except (KeyError, TypeError, ValueError):
            continue
    return thresholds


def budget_window_present(path: Path) -> bool:
    data = read_json(path)
    budget = data.get("Budget", {})
    limit = budget.get("BudgetLimit", {})
    period = budget.get("TimePeriod", {})
    return (
        str(limit.get("Amount", "")).startswith("180")
        and limit.get("Unit") == "USD"
        and bool(period.get("Start"))
        and bool(period.get("End"))
    )


def normalize_budget_time(value: Any) -> str:
    text = str(value or "").strip()
    if not text:
        return ""
    if len(text) == 16 and text[10] == "_":
        return text
    normalized = text.replace("Z", "+00:00")
    try:
        from datetime import datetime, timezone

        parsed = datetime.fromisoformat(normalized)
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone(timezone.utc).strftime("%Y-%m-%d_%H:%M")
    except ValueError:
        return text[:16].replace("T", "_")


def budget_window_matches_goal(path: Path) -> tuple[bool, str]:
    data = read_json(path)
    period = data.get("Budget", {}).get("TimePeriod", {})
    actual_start = normalize_budget_time(period.get("Start"))
    actual_end = normalize_budget_time(period.get("End"))
    passed = (
        actual_start == EXPECTED_BUDGET_WINDOW_START_UTC
        and actual_end == EXPECTED_BUDGET_WINDOW_END_UTC
    )
    return (
        passed,
        f"actual={actual_start or 'missing'}..{actual_end or 'missing'} "
        f"expected={EXPECTED_BUDGET_WINDOW_START_UTC}..{EXPECTED_BUDGET_WINDOW_END_UTC}",
    )


def target_health_summary(path: Path) -> tuple[int, list[str]]:
    data = read_json(path)
    healthy_ids: list[str] = []
    for item in data.get("TargetHealthDescriptions", []):
        if item.get("TargetHealth", {}).get("State") == "healthy":
            target_id = item.get("Target", {}).get("Id")
            if target_id:
                healthy_ids.append(target_id)
    return len(healthy_ids), healthy_ids


def healthy_az_count(app_instances_path: Path, healthy_ids: list[str]) -> int:
    if not healthy_ids or not app_instances_path.exists():
        return 0
    data = read_json(app_instances_path)
    healthy = set(healthy_ids)
    azs: set[str] = set()
    for instance in data:
        if instance.get("InstanceId") in healthy:
            az = instance.get("AvailabilityZone")
            if az:
                azs.add(az)
    return len(azs)


def json_file_has_text(path: Path, text: str) -> bool:
    if not path.exists():
        return False
    return text in path.read_text(encoding="utf-8", errors="replace")


def add(checks: list[Check], name: str, passed: bool, detail: str) -> None:
    checks.append(Check(name=name, passed=passed, detail=detail))


def audit_evidence_dir(
    checks: list[Check],
    evidence_dir: Path | None,
    label: str,
    *,
    expected_mode: str | None = None,
    require_demo_ha: bool = False,
    require_lgtm: bool = False,
) -> str | None:
    add(
        checks,
        f"{label} evidence directory",
        bool(evidence_dir and evidence_dir.exists()),
        str(evidence_dir) if evidence_dir else "missing",
    )
    if not evidence_dir or not evidence_dir.exists():
        return None

    safe_outputs = evidence_dir / "terraform-safe-outputs.json"
    if safe_outputs.exists():
        outputs = read_json(safe_outputs)
        mode = outputs.get("deployment_mode")
        add(checks, f"{label} terraform safe outputs", True, f"mode={mode}")
        if expected_mode:
            add(
                checks,
                f"{label} deployment mode",
                mode == expected_mode,
                f"actual={mode or 'missing'} expected={expected_mode}",
            )
        budget_outputs_ok = (
            str(outputs.get("budget_limit_usd", "")).startswith("180")
            and outputs.get("budget_time_period_start") == EXPECTED_BUDGET_WINDOW_START_UTC
            and outputs.get("budget_time_period_end") == EXPECTED_BUDGET_WINDOW_END_UTC
            and set(outputs.get("budget_notification_thresholds_usd", [])) == REQUIRED_BUDGET_THRESHOLDS
        )
        add(
            checks,
            f"{label} budget outputs",
            budget_outputs_ok,
            "limit/exact-window/threshold outputs checked",
        )
    else:
        mode = None
        add(checks, f"{label} terraform safe outputs", False, f"missing {safe_outputs.name}")

    budget_path = evidence_dir / "budget.json"
    add(
        checks,
        f"{label} budget api window",
        budget_path.exists() and budget_window_present(budget_path),
        "180 USD budget with start/end" if budget_path.exists() else f"missing {budget_path.name}",
    )
    if budget_path.exists():
        window_ok, window_detail = budget_window_matches_goal(budget_path)
        add(checks, f"{label} budget api exact window", window_ok, window_detail)

    notifications_path = evidence_dir / "budget-notifications.json"
    if notifications_path.exists():
        thresholds = budget_thresholds_from_notifications(notifications_path)
        add(
            checks,
            f"{label} budget notifications",
            REQUIRED_BUDGET_THRESHOLDS.issubset(thresholds),
            f"actual thresholds={sorted(thresholds)}",
        )
    else:
        add(checks, f"{label} budget notifications", False, f"missing {notifications_path.name}")

    for name in ("healthz.json", "readyz.json", "s3-smoke.txt", "phase3-verify-aws.txt"):
        path = evidence_dir / name
        add(checks, f"{label} {name}", path.exists(), "present" if path.exists() else "missing")

    target_path = evidence_dir / "alb-target-health.json"
    app_instances_path = evidence_dir / "app-instances.json"
    if target_path.exists():
        healthy_count, healthy_ids = target_health_summary(target_path)
        minimum = 2 if require_demo_ha else 1
        add(checks, f"{label} alb healthy targets", healthy_count >= minimum, f"healthy={healthy_count} min={minimum}")
        if require_demo_ha:
            az_count = healthy_az_count(app_instances_path, healthy_ids)
            add(checks, f"{label} healthy AZ spread", az_count >= 2, f"healthy_az_count={az_count}")
    else:
        add(checks, f"{label} alb healthy targets", False, f"missing {target_path.name}")

    if require_demo_ha:
        drill_files = ("phase3-failure-drill.txt", "phase3-failure-drill-verify.txt")
        drill_present = any((evidence_dir / name).exists() for name in drill_files)
        add(
            checks,
            f"{label} failure drill evidence",
            drill_present,
            "present" if drill_present else f"missing one of {', '.join(drill_files)}",
        )

    if require_lgtm:
        for datasource in REQUIRED_DATASOURCES:
            path = evidence_dir / f"grafana-datasource-{datasource}.json"
            add(checks, f"{label} grafana datasource {datasource}", path.exists(), "present" if path.exists() else "missing")

        lgtm_files = {
            "prometheus backend": "prometheus-backend-up.json",
            "tempo backend": "tempo-backend-search.json",
            "pyroscope backend": "pyroscope-backend-render.json",
            "loki backend logs": "loki-backend-logs.json",
            "loki redaction canary": "loki-redaction-canary.json",
        }
        for item_label, filename in lgtm_files.items():
            path = evidence_dir / filename
            add(checks, f"{label} {item_label}", path.exists(), "present" if path.exists() else "missing")

        redaction_path = evidence_dir / "loki-redaction-canary.json"
        if redaction_path.exists():
            raw_markers = (
                "phase3-raw-signed-canary",
                "phase3-raw-qr-canary",
                "phase3-raw-provider-canary",
                "phase3-raw-email-body-canary",
                "phase3-raw-recipient-email-canary",
                "phase3-raw-idempotency-canary",
                "phase3-raw-pii-canary",
            )
            leaked = [marker for marker in raw_markers if json_file_has_text(redaction_path, marker)]
            add(
                checks,
                f"{label} redaction canary leakage",
                not leaked and json_file_has_text(redaction_path, "[REDACTED]"),
                "raw markers absent and [REDACTED] present" if not leaked else f"leaked={leaked}",
            )

    return str(mode or "") if mode else None


def audit_destroy_evidence_dir(
    checks: list[Check],
    evidence_dir: Path | None,
    selected_region: str | None,
) -> bool:
    add(
        checks,
        "post-demo destroy evidence directory",
        bool(evidence_dir and evidence_dir.exists()),
        str(evidence_dir) if evidence_dir else "missing",
    )
    if not evidence_dir or not evidence_dir.exists():
        return False

    summary_path = evidence_dir / "destroy-summary.json"
    if not summary_path.exists():
        add(checks, "post-demo destroy summary", False, f"missing {summary_path.name}")
        return False

    summary = read_json(summary_path)
    destroyed = bool(summary.get("destroyed")) and summary.get("status") == "destroyed"
    add(
        checks,
        "post-demo destroy summary",
        destroyed,
        f"status={summary.get('status') or 'missing'} destroyed={summary.get('destroyed')}",
    )
    add(
        checks,
        "post-demo destroy selected region",
        bool(selected_region and summary.get("selected_region") == selected_region),
        f"actual={summary.get('selected_region') or 'missing'} expected={selected_region or 'missing'}",
    )
    add(
        checks,
        "post-demo destroy reason",
        bool(str(summary.get("reason") or "").strip()),
        "present" if summary.get("reason") else "missing",
    )

    for name in (
        "aws-identity.json",
        "identity-guard.txt",
        "terraform-destroy-plan.txt",
        "terraform-destroy-apply.txt",
        "terraform-state-list-after-destroy.txt",
    ):
        path = evidence_dir / name
        add(checks, f"post-demo destroy {name}", path.exists(), "present" if path.exists() else "missing")

    state_list_path = evidence_dir / "terraform-state-list-after-destroy.txt"
    state_empty = state_list_path.exists() and not state_list_path.read_text(encoding="utf-8").strip()
    add(
        checks,
        "post-demo destroy state empty",
        state_empty,
        "terraform state list empty" if state_empty else "terraform state list still has resources or is missing",
    )
    return destroyed and state_empty


def audit(args: argparse.Namespace) -> list[Check]:
    checks: list[Check] = []

    try:
        branch = run_text(["git", "branch", "--show-current"])
        add(
            checks,
            "branch",
            branch == "feature/phase3-compose-ha-lgtm-pr",
            f"current branch: {branch or 'unknown'}",
        )
    except subprocess.CalledProcessError as exc:
        add(checks, "branch", False, exc.stderr.strip() or "git branch failed")

    tfvars = TF_DIR / "terraform.tfvars"
    selected_env = Path(args.selected_region_env)
    selected_values = parse_env_file(selected_env)
    selected_region = selected_values.get("PHASE3_AWS_REGION")
    report_path = selected_values.get("PHASE3_COST_REPORT")

    add(
        checks,
        "terraform tfvars",
        tfvars.exists(),
        "present" if tfvars.exists() else f"missing {tfvars}",
    )
    tfvars_region = tfvars_value(tfvars, "aws_region")
    add(
        checks,
        "selected region alignment",
        bool(tfvars_region and selected_region and tfvars_region == selected_region),
        f"tfvars={tfvars_region or 'missing'} selected={selected_region or 'missing'}",
    )

    report_ok = False
    if report_path and Path(report_path).exists():
        report = read_json(Path(report_path))
        candidate_regions = set(report.get("candidate_regions", []))
        totals = report.get("totals_by_region", {})
        required_items = set(report.get("required_cost_items", []))
        selected_total = float(report.get("selected_region_total_usd", 999999))
        report_ok = (
            report.get("status") == "pass"
            and selected_total < 180
            and candidate_regions == {"us-east-1", "us-east-2", "us-west-2"}
            and set(totals) >= candidate_regions
            and bool(required_items)
            and report.get("cost_window_start_taipei") == EXPECTED_COST_WINDOW_START_TAIPEI
            and report.get("cost_window_end_taipei") == EXPECTED_COST_WINDOW_END_TAIPEI
        )
        detail = (
            f"selected={report.get('selected_region')} "
            f"total={selected_total:.4f} "
            f"window={report.get('cost_window_start_taipei')}..{report.get('cost_window_end_taipei')}"
        )
    else:
        detail = f"missing report from {selected_env}"
    add(checks, "cost gate report", report_ok, detail)

    evidence_root = Path(args.evidence_root)
    if args.evidence_dir:
        audit_evidence_dir(
            checks,
            Path(args.evidence_dir),
            "selected",
            require_demo_ha=False,
            require_lgtm=True,
        )
        return checks

    demo_evidence_dir = (
        Path(args.demo_evidence_dir)
        if args.demo_evidence_dir
        else latest_evidence_dir_by_mode(evidence_root, "demo-ha")
    )
    audit_evidence_dir(
        checks,
        demo_evidence_dir,
        "demo-ha",
        expected_mode="demo-ha",
        require_demo_ha=True,
        require_lgtm=True,
    )

    post_demo_evidence_dir = (
        Path(args.post_demo_evidence_dir)
        if args.post_demo_evidence_dir
        else latest_evidence_dir_by_mode(evidence_root, "dev-single-az")
    )
    destroy_evidence_dir = None if post_demo_evidence_dir else latest_destroy_evidence_dir(evidence_root)
    if post_demo_evidence_dir and is_destroy_evidence_dir(post_demo_evidence_dir):
        destroyed = audit_destroy_evidence_dir(checks, post_demo_evidence_dir, selected_region)
        add(
            checks,
            "post-demo scale-down",
            destroyed,
            "resources destroyed" if destroyed else "destroy evidence incomplete",
        )
    elif post_demo_evidence_dir:
        post_mode = audit_evidence_dir(
            checks,
            post_demo_evidence_dir,
            "post-demo",
            expected_mode="dev-single-az",
            require_demo_ha=False,
            require_lgtm=False,
        )
        add(
            checks,
            "post-demo scale-down",
            post_mode == "dev-single-az",
            f"mode={post_mode or 'missing'}",
        )
    elif destroy_evidence_dir:
        destroyed = audit_destroy_evidence_dir(checks, destroy_evidence_dir, selected_region)
        add(
            checks,
            "post-demo scale-down",
            destroyed,
            "resources destroyed" if destroyed else "destroy evidence incomplete",
        )
    else:
        add(
            checks,
            "post-demo evidence directory",
            bool(args.allow_missing_post_demo),
            "allowed missing interim evidence" if args.allow_missing_post_demo else "missing dev-single-az evidence after demo",
        )
        add(
            checks,
            "post-demo scale-down",
            bool(args.allow_missing_post_demo),
            "allowed missing interim evidence" if args.allow_missing_post_demo else "missing post-demo dev-single-az/destroy evidence",
        )

    return checks


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-root", default=str(DEFAULT_EVIDENCE_ROOT))
    parser.add_argument("--evidence-dir", default="")
    parser.add_argument("--demo-evidence-dir", default="")
    parser.add_argument("--post-demo-evidence-dir", default="")
    parser.add_argument("--selected-region-env", default=str(TF_DIR / "selected-region.env"))
    parser.add_argument(
        "--allow-missing-post-demo",
        action="store_true",
        help="Allow interim audits before the scheduled demos are complete.",
    )
    args = parser.parse_args()

    checks = audit(args)
    failures = [check for check in checks if not check.passed]
    for check in checks:
        status = "PASS" if check.passed else "FAIL"
        print(f"{status}\t{check.name}\t{check.detail}")
    print()
    if failures:
        print(f"phase3 completion audit failed: {len(failures)} missing or weak evidence item(s)")
        return 1
    print("phase3 completion audit passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
