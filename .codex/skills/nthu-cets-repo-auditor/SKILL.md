---
name: nthu-cets-repo-auditor
description: Strict review workflow for the NTHU Corporate Event Ticketing System final project repo. Use when Codex must audit this repo for grading risk, evidence gaps, implementation completeness, architecture scalability, tests, code quality, Sonar readiness, performance, operations, reliability, security, privacy, or presentation-supporting proof against the final report rubric.
---

# NTHU CETS Repo Auditor

## Overview

Use this skill to review the Corporate Event Ticketing System repo as a strict final-project judge. The output should be a risk-first audit report, not a fix plan unless the user asks for remediation.

## Required Context

Before judging the repo, read only the context needed for the review:

- `AGENTS.md`
- `docs/INDEX.md`
- `docs/PRODUCT.md`
- `docs/ARCHITECTURE.md`
- `docs/specs/phase1-product-requirements.md`
- `docs/specs/phase1-production-upper-bound.md`
- `docs/specs/phase1-e2e-test-paths.md`
- `docs/specs/phase1-nfr-and-capacity.md`

Read `references/evaluation-rubric.md` for the preserved NTHU grading basis and `references/repo-review-playbook.md` for the report format and severity rules.

## Workflow

1. Ground the review in repo truth.
   - Inspect current docs, API contracts, app/backend source, tests, k6, CI, Compose, Sonar config, and observability assets.
   - Do not rely on the original PDF file; it is intentionally not part of this skill.

2. Run the evidence collector when filesystem access is available:

   ```bash
   python3 .codex/skills/nthu-cets-repo-auditor/scripts/collect_strict_audit_evidence.py --repo . --format markdown
   ```

   Use `--format json` when another tool or report generator needs structured output.

3. Review by grading weight.
   - 30% requirements conversion and implementation.
   - 25% architecture design and scalability.
   - 25% system testing and validation.
   - 10% code quality.
   - 10% operations and reliability.

4. Lead with findings.
   - Findings must cite files and line numbers when possible.
   - Order by severity and grading impact.
   - Prefer evidence gaps and behavioral risks over general advice.
   - Explicitly separate observed repo facts from inferred risk.

5. Verify claims.
   - If commands were run, report pass/fail and important excerpts.
   - If commands were not run, say exactly what remains unverified.
   - Treat route-only, fake-service-only, docs-only, or happy-path-only evidence as insufficient for production completion.

## Scoring Bias

Be stricter than a normal code review. A high score requires demonstrable repo evidence for correctness, tests, performance, reliability, and presentation-ready proof. Penalize:

- Missing negative or failure tests for oversell, duplicate booking, ineligible booking, duplicate check-in, retry, outage, RBAC, or PII redaction.
- Claims of scalability without diagrams, capacity targets, k6 or load results, monitoring metrics, or bottleneck reasoning.
- UI or API flows that only work through mocks, manual DB edits, local demo shortcuts, or undocumented assumptions.
- Large hand-written files over the repo limit, broad utility dumping grounds, cyclic boundaries, or hidden state.
- Reliability claims without Compose, health/readiness, outbox/worker retry, metrics, logs, and degraded-service behavior.

## Output Shape

Use Traditional Chinese by default unless the user asks otherwise. Follow this compact structure:

1. `Findings`: severity, rubric impact, file/line evidence, and why it risks grading loss.
2. `Evidence Coverage`: five grading categories with status and missing proof.
3. `Required Verification`: exact commands to run or rerun.
4. `Presentation Evidence`: concise list of proof objects worth showing in the final report or demo.
5. `Residual Risk`: what cannot be judged from the current repo or command results.
