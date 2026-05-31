# Phase 1 UI/UX Flow Completion

## Goal

Complete the Phase 1 production UI journeys for local/demo static builds while preserving the modular monolith architecture and existing Docker Compose topology.

## Acceptance Criteria

- `/api/v1/auth/bootstrap` returns `debug_chrome_enabled`, enabled only for local, demo, and test environments and disabled in production.
- The web app uses the bootstrap flag to decide whether Debug chrome is available. `?debug=1` only enables the chrome when the server allows it.
- Debug controls remain discoverable on login, auth-required, desktop header, desktop sidebar context, and mobile utility surfaces.
- Check-in results present holder name, department, city, and family count before ticket identifiers. Duplicate scans show first scan time and device. Rejected scans show reason code and recovery copy without relying on color.
- Frontend API contracts match current backend fields for check-in, offline check-in packages, reports, lottery runs, and related UI models.
- API observer and offline package UI never display raw ticket signatures, QR payloads, package signatures, token hashes, or provider tokens. Short fingerprints are acceptable.
- Admin eligibility governance previews server impact, shows zero-match warnings, requires explicit zero-match confirmation, and saves rule versions through backend APIs.
- Registration governance includes allocation controls. Lottery events require a seed and explicit deterministic-run confirmation before calling the lottery endpoint; FCFS events state that allocation is not applicable.
- HR reports show capacity type, nullable capacity and remaining capacity, employee count, family count, attendee count, city distribution, and export lifecycle state.
- Audit filters are URL-backed, selected rows are deep-linkable, and cursor pagination uses a generated cursor from the last visible row.

## Non-Goals

- No new service, queue, database, deployment topology, or frontend dependency.
- No Phase 1 microservice, Kafka, Kubernetes, service mesh, or cross-region HA claims.
- No direct HR report download endpoint; export remains status and object-key based.

## Test Strategy

- Backend unit tests cover auth bootstrap debug availability across test/local/demo and production.
- Frontend unit tests cover debug gating, redaction fingerprints, check-in result rendering, and API paths.
- Existing route Playwright coverage is extended through mocks where the new fields are required.
- Verification includes web type-check, Vitest, core role routes, targeted Go auth tests, and `git diff --check`.
