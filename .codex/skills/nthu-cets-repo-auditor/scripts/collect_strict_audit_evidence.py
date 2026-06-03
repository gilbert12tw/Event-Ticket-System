#!/usr/bin/env python3
"""Collect static evidence for the NTHU CETS strict repo audit."""

from __future__ import annotations

import argparse
import json
import re
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Iterable


RUBRIC = [
    ("requirements", "30% 需求轉換與實作"),
    ("architecture", "25% 架構設計與可擴展性"),
    ("testing", "25% 系統測試與驗證"),
    ("quality", "10% 程式碼品質"),
    ("ops", "10% 運維與可靠性"),
]

SOURCE_EXTENSIONS = {".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".css"}
TEXT_EXTENSIONS = SOURCE_EXTENSIONS | {".md", ".yaml", ".yml", ".json", ".rb", ".sh"}
EXCLUDED_DIRS = {
    ".git",
    ".pnpm-store",
    ".turbo",
    "node_modules",
    "dist",
    "build",
    "coverage",
    "playwright-report",
    "playwright-live-report",
    "test-results",
}
GENERATED_OR_STATIC_MARKERS = {
    ".codex/skills/nthu-cets-repo-auditor",
    "services/api/internal/httpapi/static/assets",
    "apps/web/node_modules",
}
EXCLUDED_FILE_NAMES = {
    "go.sum",
    "package-lock.json",
    "pnpm-lock.yaml",
    "skills-lock.json",
    "yarn.lock",
}
SENSITIVE_PATTERNS = [
    ("signed token", re.compile(r"signed[_-]?token|qr[_-]?payload|qr[_-]?token", re.I)),
    ("provider/session token", re.compile(r"provider[_-]?token|session[_-]?secret|set-cookie", re.I)),
    ("secret config", re.compile(r"password|secret|token_signing|object_storage_secret", re.I)),
    ("demo pii", re.compile(r"Ariel Chen|Ben Lin|Carla Wu|[A-Za-z0-9._%+-]+@cets\\.local")),
]


@dataclass
class FileSizeRisk:
    severity: str
    path: str
    lines: int


@dataclass
class GateCheck:
    key: str
    status: str
    evidence: list[str]
    missing: list[str]


@dataclass
class SensitiveCandidate:
    kind: str
    path: str
    line: int
    excerpt: str


def rel(path: Path, repo: Path) -> str:
    return path.relative_to(repo).as_posix()


def is_excluded(path: Path, repo: Path) -> bool:
    relative = rel(path, repo) if path != repo else ""
    parts = set(Path(relative).parts)
    if parts & EXCLUDED_DIRS:
        return True
    if path.name in EXCLUDED_FILE_NAMES:
        return True
    return any(relative.startswith(marker) for marker in GENERATED_OR_STATIC_MARKERS)


def iter_files(repo: Path, extensions: set[str]) -> Iterable[Path]:
    for path in repo.rglob("*"):
        if path.is_file() and path.suffix in extensions and not is_excluded(path, repo):
            yield path


def line_count(path: Path) -> int:
    try:
        with path.open("r", encoding="utf-8", errors="ignore") as handle:
            return sum(1 for _ in handle)
    except OSError:
        return 0


def collect_file_size_risks(repo: Path) -> list[FileSizeRisk]:
    risks: list[FileSizeRisk] = []
    for path in iter_files(repo, SOURCE_EXTENSIONS):
        lines = line_count(path)
        if lines > 500:
            risks.append(FileSizeRisk("fail", rel(path, repo), lines))
        elif lines >= 300:
            risks.append(FileSizeRisk("warn", rel(path, repo), lines))
    return sorted(risks, key=lambda risk: (-risk.lines, risk.path))


def existing(repo: Path, paths: Iterable[str]) -> list[str]:
    found = []
    for item in paths:
        if (repo / item).exists():
            found.append(item)
    return found


def count_matches(repo: Path, glob: str) -> int:
    return sum(1 for path in repo.glob(glob) if path.is_file() and not is_excluded(path, repo))


def gate(key: str, evidence: list[str], required: list[str]) -> GateCheck:
    missing = [item for item in required if item not in evidence]
    status = "pass" if not missing else "warn" if evidence else "fail"
    return GateCheck(key, status, evidence, missing)


def collect_gate_checks(repo: Path) -> list[GateCheck]:
    checks: list[GateCheck] = []
    checks.append(gate("go backend", existing(repo, ["services/api/go.mod", "services/api/.golangci.yml"]), ["services/api/go.mod", "services/api/.golangci.yml"]))
    checks.append(gate("react frontend", existing(repo, ["apps/web/package.json", "apps/web/vite.config.ts", "apps/web/src/App.tsx"]), ["apps/web/package.json", "apps/web/vite.config.ts", "apps/web/src/App.tsx"]))
    checks.append(gate("compose runtime", existing(repo, ["services/api/deploy/compose.yaml", "services/api/deploy/.env.example"]), ["services/api/deploy/compose.yaml", "services/api/deploy/.env.example"]))
    checks.append(gate("openapi contract", existing(repo, ["docs/openapi.yaml", "scripts/check-openapi-contract.rb"]), ["docs/openapi.yaml", "scripts/check-openapi-contract.rb"]))
    checks.append(gate("sonar quality", existing(repo, ["sonar-project.properties", "scripts/coverage/sonar-go-coverage.sh"]), ["sonar-project.properties", "scripts/coverage/sonar-go-coverage.sh"]))
    checks.append(gate("playwright e2e", existing(repo, ["apps/web/playwright.config.ts", "apps/web/playwright.live.config.ts", "apps/web/e2e-live/phase1-live-flow.spec.ts"]), ["apps/web/playwright.config.ts", "apps/web/e2e-live/phase1-live-flow.spec.ts"]))
    checks.append(gate("k6 performance", existing(repo, ["k6/phase1-production-gate.js", "k6/phase1-release-gate.js"]), ["k6/phase1-production-gate.js", "k6/phase1-release-gate.js"]))
    checks.append(gate("observability", existing(repo, ["services/api/internal/observability/metrics.go", "services/api/deploy/observability/prometheus.yml", "services/api/deploy/observability/alertmanager.yml"]), ["services/api/internal/observability/metrics.go", "services/api/deploy/observability/prometheus.yml"]))
    checks.append(gate("github actions", existing(repo, [".github/workflows/ci.yml", ".actrc"]), [".github/workflows/ci.yml"]))
    go_tests = count_matches(repo, "services/api/**/*_test.go")
    web_tests = count_matches(repo, "apps/web/src/**/*.test.ts") + count_matches(repo, "apps/web/src/**/*.test.tsx")
    missing = [] if go_tests and web_tests else ["go and web tests must both exist"]
    checks.append(GateCheck("test inventory", "pass" if not missing else "warn", [f"go_tests={go_tests}", f"web_tests={web_tests}"], missing))
    return checks


def collect_sensitive_candidates(repo: Path, limit: int) -> list[SensitiveCandidate]:
    candidates: list[SensitiveCandidate] = []
    for path in iter_files(repo, TEXT_EXTENSIONS):
        try:
            lines = path.read_text(encoding="utf-8", errors="ignore").splitlines()
        except OSError:
            continue
        for number, line in enumerate(lines, start=1):
            for kind, pattern in SENSITIVE_PATTERNS:
                if pattern.search(line):
                    excerpt = line.strip()
                    if len(excerpt) > 160:
                        excerpt = excerpt[:157] + "..."
                    candidates.append(SensitiveCandidate(kind, rel(path, repo), number, excerpt))
                    break
            if len(candidates) >= limit:
                return candidates
    return candidates


def rubric_status(checks: list[GateCheck], size_risks: list[FileSizeRisk]) -> dict[str, str]:
    passed = {check.key for check in checks if check.status == "pass"}
    return {
        "requirements": "partial",
        "architecture": "strong" if {"openapi contract", "compose runtime"} <= passed else "partial",
        "testing": "strong" if {"playwright e2e", "k6 performance", "test inventory"} <= passed else "partial",
        "quality": "partial" if any(risk.severity == "fail" for risk in size_risks) else "strong",
        "ops": "strong" if {"compose runtime", "observability", "github actions"} <= passed else "partial",
    }


def collect(repo: Path, candidate_limit: int) -> dict:
    size_risks = collect_file_size_risks(repo)
    checks = collect_gate_checks(repo)
    candidates = collect_sensitive_candidates(repo, candidate_limit)
    status = rubric_status(checks, size_risks)
    return {
        "repo": str(repo),
        "rubric": [{"key": key, "criterion": label, "status": status[key]} for key, label in RUBRIC],
        "gate_checks": [asdict(check) for check in checks],
        "file_size_risks": [asdict(risk) for risk in size_risks],
        "sensitive_candidates": [asdict(candidate) for candidate in candidates],
        "recommended_commands": [
            "git diff --check",
            "pnpm check",
            "pnpm test:coverage",
            "pnpm sonar:scan",
            "cd services/api && go test ./... -count=1",
            "docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config",
        ],
    }


def render_markdown(data: dict) -> str:
    lines = ["# Strict Audit Evidence Snapshot", ""]
    lines.append("## Rubric Coverage")
    for item in data["rubric"]:
        lines.append(f"- {item['criterion']}: {item['status']}")
    lines.append("")
    lines.append("## Gate Checks")
    for check in data["gate_checks"]:
        evidence = ", ".join(check["evidence"]) if check["evidence"] else "none"
        lines.append(f"- {check['key']}: {check['status']} ({evidence})")
        if check["missing"]:
            lines.append(f"  Missing: {', '.join(check['missing'])}")
    lines.append("")
    lines.append("## File Size Risks")
    if data["file_size_risks"]:
        for risk in data["file_size_risks"][:40]:
            lines.append(f"- {risk['severity']}: {risk['path']} ({risk['lines']} lines)")
    else:
        lines.append("- none")
    lines.append("")
    lines.append("## Sensitive Data Candidates")
    if data["sensitive_candidates"]:
        for candidate in data["sensitive_candidates"][:40]:
            lines.append(f"- {candidate['kind']}: {candidate['path']}:{candidate['line']} - {candidate['excerpt']}")
    else:
        lines.append("- none")
    lines.append("")
    lines.append("## Recommended Verification Commands")
    for command in data["recommended_commands"]:
        lines.append(f"- `{command}`")
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=".", help="Repository root to inspect.")
    parser.add_argument("--format", choices=["markdown", "json"], default="markdown")
    parser.add_argument("--candidate-limit", type=int, default=80)
    args = parser.parse_args()

    repo = Path(args.repo).resolve()
    data = collect(repo, args.candidate_limit)
    if args.format == "json":
        print(json.dumps(data, ensure_ascii=False, indent=2))
    else:
        print(render_markdown(data), end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
