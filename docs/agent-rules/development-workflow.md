# Development Workflow Rules

Before changing code, confirm requirements, acceptance criteria, failure scenarios, and test strategy. If architecture or implementation strategy changes, update `docs/ARCHITECTURE.md`, `AGENTS.md`, or the relevant rule file before changing code.

Do not cite temporary drafts or soon-to-be-removed notes as sources of truth.

## Small Tasks

Do not implement a large task in one pass. Split it into small tasks first. Each small task must map to one use case, module, risk, or independently verifiable behavior.

Before starting each small task, define:

- Goal: the concrete behavior this task delivers.
- Acceptance criteria: testable Given / When / Then conditions or equivalent checks.
- Impact scope: modules, APIs, schemas, config, docs, or tests touched by the task.
- Test strategy: unit, integration, E2E, failure, or load tests to add or update.
- Non-goals: refactors, follow-up features, or future evolution intentionally left out.

Keep diffs focused. Do not mix whole-project formatting, dependency upgrades, broad refactors, documentation rewrites, and feature work in the same small task.

## Completion Standard

After completing a small task, add or update matching tests before deciding whether it satisfies the acceptance criteria.

Minimum completion standard:

- Happy path tests pass.
- Major error scenarios have tests or explicit verification notes.
- Ticketing correctness edges are covered when relevant, including oversell, duplicate booking, ineligible booking, duplicate check-in, notification retry, or queue retry.
- 12-Factor checks pass: env-based config, logs to stdout, state in backing services, and stateless processes.
- `git diff` contains only code, tests, and docs required by the small task.

Do not commit if tests fail, acceptance criteria are not met, or unrelated changes are mixed into the diff.

## Local Commits

After a small task is tested and behaves as expected, create a local commit unless the user explicitly says not to commit. A local commit does not mean push or pull request creation.

Commit rules:

- One commit maps to one small task or one independently verifiable behavior.
- Include code, tests, and necessary documentation for that task.
- Do not mix broad formatting, refactoring, dependency upgrades, and feature work in one commit.
- Do not commit `.env`, real secrets, local temporary files, or unrelated files.
- Before committing, inspect `git status`, `git diff --stat`, and relevant test results.
- Use explicit commit message scopes, for example `feat(registration): add booking idempotency`.
