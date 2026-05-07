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
- `docs/agent-rules/team-collaboration.md` for API contracts, Linear workflow, ownership, PR review, migration coordination, and test conventions.

## Phase 1 Focus

Phase 1 uses Docker Compose backing services and a modular monolith. Do not implement Phase 1 as microservices. Do not treat Kafka, Kubernetes, cross-region HA, or a service mesh as required Phase 1 deliverables.

Phase 1 must cover:

- Employees browse events, check eligibility, and book or join a waitlist.
- Welfare committee or activity admins create events, configure capacity, define eligibility rules, and inspect booking status.
- The system issues electronic tickets and QR codes.
- Check-in staff redeem tickets online while preserving a future offline check-in boundary.
- HR or system admins inspect participation reports and audit logs.

## Core Boundaries

Controllers handle input/output and authorization only. Application services coordinate use cases. Domain code owns business rules. Repositories and adapters wrap PostgreSQL, Redis, object storage, queue, mail, HR, and SSO dependencies.

Primary modules:

- Auth & RBAC
- Event
- Eligibility
- Registration
- Ticket
- Check-in
- Notification
- Reporting
- Admin / Audit

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

Reviewer subagents must check correctness, tests, 12-Factor compliance, clean-code limits, and unrelated diff churn before a task is accepted.

## Verification

Use task-relevant checks. At minimum, documentation-only changes need `git diff --check`. Runtime or Docker-related changes should verify:

- `docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config`
- Relevant Go, frontend, integration, and failure tests
- Logs do not contain full PII or secrets
- Phase 1 docs do not claim microservices, Kafka, Kubernetes, or cross-region HA are complete

## Codex Rules Note

OpenAI Codex `.codex/rules/*.rules` files are for sandbox escalation and command prefix policy. Do not place architecture, clean-code, workflow, or product guidance there.
