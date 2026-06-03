# Repo Review Playbook

Use this playbook when producing the strict audit report.

## Severity

- `Critical`: likely major grading loss or correctness break in core ticketing, such as oversell, invalid booking, duplicate check-in, missing role protection, or fabricated evidence.
- `High`: important rubric gap with direct score impact, such as missing integration/load/E2E evidence, weak architecture proof, or absent reliability story.
- `Medium`: maintainability, presentation evidence, or partial-coverage issue that can still cost points.
- `Low`: polish, traceability, or reporting clarity issue.

## Finding Format

Use this shape for every actionable finding:

```text
[Severity] Short title
Rubric impact: 25% 系統測試與驗證
Evidence: /absolute/path/file.ext:123
Problem: observed fact and why it weakens the final-project score.
Required proof: command, test, diagram, screenshot, or code evidence needed to close it.
```

If there is no exact line number, cite the closest file or command output. Do not invent line references.

## Review Procedure

1. Run or inspect the evidence collector output.
2. Map repo evidence to the five rubric categories.
3. Cross-check each core business promise:
   - Activity creation and publishing.
   - Eligibility and final booking recheck.
   - Limited/unlimited capacity behavior.
   - Idempotent booking and cancellation.
   - Waitlist and allocation behavior.
   - Signed tickets and non-transferability.
   - One-time online and offline check-in.
   - Notification retry and outbox behavior.
   - Aggregate reporting and privacy redaction.
   - Audit filtering and immutable sensitive-action records.
   - RBAC for employee, admin, check-in staff, and HR.
4. Reject weak evidence patterns:
   - Docs-only completion claims.
   - Route exposure without service/integration behavior.
   - Fake services as the only production proof.
   - Mocked UI tests as the only end-to-end proof.
   - Load scripts without thresholds or execution evidence.
   - Monitoring assets without metrics tied to failure modes.
5. State what was not verified.

## Commands To Consider

Use commands only when appropriate for the user's request and environment:

```bash
python3 .codex/skills/nthu-cets-repo-auditor/scripts/collect_strict_audit_evidence.py --repo . --format markdown
git diff --check
pnpm check
pnpm test:coverage
pnpm sonar:scan
cd services/api && go test ./... -count=1
docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config
```

For full release-style evidence, also consider live Playwright and k6 gates if local services are available.

## Category Status Rules

- `Strong`: implementation, negative/failure tests, user-facing or integration proof, and operational evidence are all present.
- `Partial`: implementation exists but one or more proof layers are missing.
- `Weak`: mostly docs, mock-only checks, or isolated helpers.
- `Missing`: no meaningful repo evidence found.

## Final Summary

End with a concise readiness judgment:

- `Ready for strict grading`: only minor evidence gaps remain.
- `Presentation-ready but risky`: demo may work, but strict reviewers can find gaps.
- `Not ready`: critical correctness, testing, or reliability proof is missing.
