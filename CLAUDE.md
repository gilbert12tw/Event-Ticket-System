# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Source of Truth Docs

Read before changing code:

- `AGENTS.md` — entry point for coding agents; concise rules summary.
- `docs/ARCHITECTURE.md` — Phase 1 architecture, capacity, NFRs, 12-Factor mapping.
- `docs/agent-rules/{architecture,clean-code,correctness,development-workflow,local-environment}.md` — non-negotiable rule files.
- `docs/PRODUCT.md`, `docs/DESIGN.md`, `docs/specs/` — product, UX, and task-specific acceptance criteria.
- `docs/openapi.yaml` + `docs/openapi/` — API contract (gated by `scripts/check-openapi-contract.rb`).

If architecture or strategy changes, update the relevant doc BEFORE changing code.

## Common Commands

Backend (Go modular monolith under `services/api`):

```bash
# Full backend tests
cd services/api && go test ./... -count=1

# Single package / single test
go test ./internal/ticketing -run TestRegistrationOversell -count=1

# Run app or worker locally against Compose DB
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets go run ./services/api/cmd/cets {serve|worker|migrate|seed|hr-sync|ready}
```

Frontend (`apps/web`, React 19 + Vite + TS + Tailwind 4):

```bash
pnpm install                                # at repo root, uses pnpm 10.33.3
pnpm --filter cets-web lint                 # ESLint, --max-warnings=0
pnpm --filter cets-web test                 # vitest run
pnpm --filter cets-web test -- src/foo.test.ts  # single test file
pnpm --filter cets-web build                # tsc -b && vite build → embedded into Go static
pnpm --filter cets-web test:e2e             # Playwright mocked viewport gate
pnpm --filter cets-web test:e2e:live        # Playwright against running Compose app
```

Turbo orchestrates workspace tasks: `pnpm dev|build|test|lint` at root.

OpenAPI contract check: `ruby scripts/check-openapi-contract.rb`.

k6 production gate (requires running stack on `BASE_URL`): see `k6/phase1-production-gate.js`, run via `grafana/k6:1.7.1-with-browser`.

## Docker Compose Workflow

Compose file is the single local entry point. Always pass the env-file and compose file explicitly:

```bash
dc()    { docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml "$@"; }
dcdev() { docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml -f services/api/deploy/compose.dev.yaml "$@"; }

dc build app
dc up -d postgres redis minio mailhog minio-init
dc run --rm migrate                # one-off admin process
dc up -d app worker
curl -fsS http://localhost:8080/readyz
```

Rebuild `app` only when Go code, migrations, the embedded production frontend bundle (`services/api/internal/httpapi/static`), the Dockerfile, or dependency manifests change. Pure frontend hot-reload uses `dcdev ... up -d web-dev` and Vite at `:5173`.

Local default ports (overrides via env): app `8080`, postgres `15432`, redis `16379`, minio `19000/19001`, mailhog `11025/18025`, vite `5173`.

Host-CLI DB URL: `postgresql://cets:cets_dev_password@localhost:15432/cets`. Inside Compose the hostname is `postgres`.

`compose_test.go` validates the compose file via `docker compose ... config`; do not break it.

## Architecture

Phase 1 is a Go modular monolith (`services/api`) + same-binary `worker` process consuming a PostgreSQL `outbox_events` table. Do NOT introduce microservices, Kafka, Kubernetes, or cross-region HA as Phase 1 deliverables.

Layering (strict — enforced by `internal/architecture/architecture_test.go`):

- `cmd/cets` — process entrypoint; dispatches `serve|worker|migrate|seed|hr-sync|ready` (see `cmd/cets/main.go`).
- `internal/httpapi` — controllers, router, auth, request/response shapes. Input/output and authorization only.
- `internal/ticketing` — application services + domain rules for all modules (Auth/RBAC, Event, Eligibility, Registration, Ticket, Check-in, Notification, Reporting, Audit). `internal/ticketing` MUST NOT import `internal/httpapi`.
- `internal/postgres` — migrations.
- `internal/objectstore`, `internal/traceid`, `internal/config` — adapters and infra.

Frontend mirrors backend modules under `apps/web/src/features/{auth,events,registrations,tickets,checkin,reporting,audit,notifications,hr-settings,demo-runbook}`. Routes live in `src/app/routes.ts` with boundary tests in `src/app/architecture-boundaries.test.ts`.

### Non-negotiable correctness rules

- PostgreSQL is the only source of truth for committed booking, ticket, check-in, and audit state. Redis is an optional reservation/idempotency/cache layer; never the final transaction truth.
- Final booking must recheck eligibility, event state, capacity, booking window, and allocation policy inside the DB transaction. Cached event/eligibility summaries cannot authorize a booking.
- Booking, cancellation, ticket generation, notification, and check-in sync require `idempotency_key` (or equivalent) with unique constraints.
- One ticket may be redeemed exactly once. `checkin_record.ticket_id` unique constraint is the final guarantee; first-commit-wins for offline sync, preserve conflict rows with `device_id`/`scanned_at`/`staff_id`.
- Business data and async side effects (notifications, report exports) commit together via the PostgreSQL outbox; the worker must be idempotent and safe to retry.
- Sensitive actions write immutable audit log entries. Logs must never contain full PII or secrets — CI scans `app`/`worker`/`mailhog` logs for known leakage patterns (see `ci.yml` "Scan live gate logs").

### Auth model

Protected APIs and `/api/v1/auth/me` require `Authorization: Bearer <provider-token>` signed with `PROVIDER_TOKEN_SECRET`. The API maps required employee claims to exactly one application role. Local/demo/test profiles ask the backend to mint provider-format bearer tokens; legacy role headers and `/auth/login` session cookies are demo-only and not real auth paths. Production requires non-demo `TOKEN_SIGNING_SECRET` and `PROVIDER_TOKEN_SECRET` values.

### File size enforcement

`internal/architecture/architecture_test.go` fails CI if any hand-written `.go` under `services/api` or any web source under `apps/web/src` exceeds 500 lines. Target 200–300; split before adding behavior past ~400. Generated files, migrations, lockfiles, fixtures are exempt.

## Workflow Expectations

- Split large changes into small tasks. Each task = one use case / module / risk with explicit goal, acceptance criteria, impact scope, test strategy, non-goals.
- Add or update tests with the change. Cover oversell, duplicate booking, ineligible booking, duplicate check-in, notification retry, queue retry where relevant.
- Keep diffs focused: do NOT mix broad formatting, dependency upgrades, unrelated refactors, doc rewrites, and feature work.
- Commit message scope style: `feat(registration): ...`, `fix(checkin): ...`, etc. (see `git log`).
- Do not commit `.env`, real secrets, or unrelated files. Inspect `git status` and `git diff --stat` before committing.
- Phase 1 docs must not claim microservices, Kafka, Kubernetes, or cross-region HA are complete.

## CI Gates (`.github/workflows/ci.yml`)

Path-filtered jobs:

- `backend` — `go test ./... -count=1` against Postgres 16 service.
- `frontend` — pnpm lint, vitest, build, mocked Playwright.
- `openapi` — `ruby scripts/check-openapi-contract.rb`.
- `live-gates` — full Compose stack + live Playwright + k6 smoke (release gate is workflow_dispatch). Also greps logs for PII/secret leakage.

Run the matching local check before pushing.
