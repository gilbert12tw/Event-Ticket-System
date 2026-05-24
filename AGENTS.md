# Corporate Event Ticketing System Agent Guide

This file is the concise entry point for coding agents and developers working in this repository. Keep it under 100 lines. Detailed rules live in `docs/agent-rules/`.

## Source of Truth

- Architecture decisions: follow `docs/ARCHITECTURE.md`.
- Implementation discipline: follow this file and `docs/agent-rules/*.md`.
- Product and design context: see `docs/PRODUCT.md` and `docs/DESIGN.md`.
- Specs: use `docs/specs/` for task-specific acceptance criteria.
- Backend directory refactors: start from `docs/specs/backend-directory-architecture.md`.
- If architecture or implementation strategy changes, update the relevant docs before changing code.

## Required Rule Reading

Before changing code, read the rule files relevant to the task:

- `docs/agent-rules/architecture.md` for Phase 1 scope, modules, and evolution limits.
- `docs/agent-rules/clean-code.md` for Google style, file size, cohesion, and coupling rules.
- `docs/agent-rules/correctness.md` for ticketing correctness, idempotency, and audit rules.
- `docs/agent-rules/development-workflow.md` for small tasks, tests, and commit rules.
- `docs/agent-rules/local-environment.md` for Docker Compose, backing services, and verification.

## Project Context

This project implements a TDD-first corporate event ticketing system. The current codebase is a Go
modular monolith with a React SPA and focuses on:

- event publishing, eligibility, booking, waitlist, and allocation correctness;
- signed tickets, one-time QR redemption, and offline check-in conflict boundaries;
- notification, reporting, audit, privacy, and role-based operational workflows;
- PostgreSQL source-of-truth transactions plus Redis, MinIO, Mailhog, HR, and SSO adapters.

Treat the local docs below as source material. Retired research artifacts have been consolidated
into these checked-in docs and are not required for agent work:

- [docs/principles/solid.md](docs/principles/solid.md)
- [docs/principles/twelve-factor-rule.md](docs/principles/twelve-factor-rule.md)

## Non-Negotiable Rules

- Keep hand-written source files small: target 200-300 lines, never exceed 500 lines.
- Use Google style guidance plus project formatter and linter settings.
- Keep module boundaries explicit; avoid cyclic dependencies, hidden global state, and broad shared utility dumping grounds.
- Prevent oversell with PostgreSQL transactions, row locks or unique constraints; Redis is never the final transaction truth.
- Use idempotency keys or equivalent deduplication for booking, cancellation, ticket generation, notification, and check-in sync.
- Recheck eligibility and event state during final booking, not only in cached event lists.
- One ticket may be redeemed successfully only once; database constraints are the final guarantee.
- Record audit logs for sensitive actions and conflict outcomes.

## Workflow

Do not implement a large task in one pass. Split work into small tasks by use case, module, risk, or independently verifiable behavior.

For large coordinated tasks, delegate narrow workstreams to specialist subagents with explicit file ownership. Prefer fast coding agents such as `gpt-5.3-codex-spark` for bounded implementation or review slices when available.

Before each small task, define the goal, acceptance criteria, impact scope, test strategy, and non-goals. Keep diffs focused. Do not mix broad formatting, dependency upgrades, unrelated refactors, docs rewrites, and feature work in one task.

After each small task, add or update matching tests, inspect `git status` and `git diff --stat`, then create a local commit unless the user explicitly says not to commit.

Before pushing committed work, run `act push` once to verify the GitHub Actions push workflow locally. Do not push if `act push` fails; either fix the workflow/code issue or document the blocker explicitly with the failed job and log excerpt.

Reviewer subagents must check correctness, tests, 12-Factor compliance, clean-code limits, and unrelated diff churn before a task is accepted.

## Verification

Use task-relevant checks. At minimum, documentation-only changes need `git diff --check`. Runtime or Docker-related changes should verify:

- `docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config`
- Relevant Go, frontend, integration, and failure tests
- Logs do not contain full PII or secrets
- Phase 1 docs do not claim microservices, Kafka, Kubernetes, or cross-region HA are complete

## Codex Rules Note

OpenAI Codex `.codex/rules/*.rules` files are for sandbox escalation and command prefix policy. Do not place architecture, clean-code, workflow, or product guidance there.
