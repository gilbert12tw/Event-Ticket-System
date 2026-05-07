# Team Collaboration Rules

Collaboration rules live in Git and are reviewed like code. Do not rely on chat, memory, or local-only notes for API shape, module ownership, acceptance criteria, or test expectations.

## API Contracts

- `docs/openapi.yaml` is the canonical API contract anchor for new or changed HTTP APIs.
- Add or update the OpenAPI contract before implementing backend handlers, frontend API calls, or shared types.
- Each endpoint contract must define method, path, request body or query params, response body, status codes, error envelope, and auth requirement.
- Existing endpoints do not require a full OpenAPI backfill in one pass, but any endpoint touched by a feature must be documented before merge.
- API responses must use the project envelope:
  - Success: `{"success": true, "data": <payload>, "error": null}`
  - Error: `{"success": false, "data": null, "error": "<message>"}`
- Backward compatibility is required after frontend usage exists: adding optional fields is allowed, but deleting fields, renaming fields, or changing field types requires a versioned contract or an approved migration plan.
- Frontend work may proceed before backend implementation by using mock data, generated types, or hand-written types derived from the OpenAPI contract. The backend must later satisfy the same contract.

## Linear and PR Flow

- Every feature or bugfix starts from a Linear issue unless the change is a tiny repo-maintenance fix.
- Linear issues must state the owner, module or feature area, acceptance criteria, non-goals, and validation plan before implementation begins.
- PRs must link the Linear issue and summarize contract changes, module ownership, migrations, tests, and any follow-up work.
- API contract PRs are reviewed by at least one frontend and one backend reviewer before implementation depends on them.
- PRs stay small and map to one use case, module, risk, or independently verifiable behavior.
- CI must pass before merge. If CI fails, fix the failure or explicitly mark the PR blocked.

## Ownership and Boundaries

- Backend work is split by domain or feature ownership, not by horizontal layers such as controller-only, service-only, or repository-only ownership.
- Module owners may change code inside their owned module, including handlers, application service behavior, repositories, models, and tests.
- Cross-module changes must tag the affected module owner for review.
- Shared code, API contracts, config, route registration, generated types, and database migrations require stricter review because they commonly create merge conflicts.
- Keep route registration thin; place route and handler behavior in module-owned files under `services/api/internal/httpapi` and business behavior under the owning domain code in `services/api/internal/ticketing`.
- Frontend API calls and shared TypeScript contracts belong under `apps/web/src/lib/api`; feature UI code should consume those contracts rather than inventing local response shapes.

## Migration and Conflict Control

- Sync with `main` daily while actively working. The team should use one agreed strategy for branch updates, preferably rebase unless the team decides otherwise.
- Do not let feature branches accumulate unrelated work for several days before review.
- Name migrations clearly by timestamp and intent, and rerun migrations before merge when schema order changed.
- Do not make concurrent uncoordinated changes to the same table, enum, shared DTO, route registry, config file, or OpenAPI section.
- Schema changes must document nullable/default/index decisions and remain backward compatible unless the Linear issue and PR call out an approved breaking change.

## Test Convention

- Unit tests cover single functions, domain rules, and service behavior without external process dependencies.
- Integration tests cover API, PostgreSQL state, migrations, idempotency, audit rows, and important failure paths.
- Contract tests verify implemented HTTP responses match `docs/openapi.yaml` for endpoints touched by the feature.
- E2E tests cover a small number of critical frontend-to-backend workflows; do not replace lower-level tests with E2E tests.
- Load or k6 checks are required for production-gate flows called out by specs or architecture docs.
- Test names should describe behavior with `should ...` or equivalent Given / When / Then wording, and tests should follow Arrange / Act / Assert structure when practical.
- PRs without relevant tests need an explicit reviewer-approved reason.
