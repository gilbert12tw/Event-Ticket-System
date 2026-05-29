# Phase 2 WS1 — Contracts and Release Engineering

> Workstream-level spec. Parent: `COR-39` / `[PH2-WS1]`. Owner: Person A.
> Anchors: `docs/specs/phase2-scale-hardening.md` (§3.1, §3, §4), `docs/ARCHITECTURE.md` §3, Phase 1 baseline `docs/specs/phase1-production-upper-bound.md`.

## 1. Summary

WS1 owns Phase 2 shared rules: acceptance matrix, per-workstream specs, OpenAPI delta for ops + report freshness, domain event contract v2, process-first architecture docs update, contract/docs guard tests, release checklist, reviewer checklist. WS1 is gating, not runtime — its outputs unblock WS2–WS5 implementation. Once Wave-1 contracts ship, Person A pivots to reviewing WS3/WS4/WS5 PRs instead of authoring more planning docs.

WS1 does not own runtime code, schema migrations, worker code, frontend, or k6 thresholds. Those are consumed contracts for other streams.

## 2. Scope

In scope:

- `PH2-00` Phase 2 acceptance matrix (shipped, `docs/specs/phase2-scale-hardening.md`).
- `PH2-01` this workstream spec set (`docs/specs/phase2-ws{1..5}-*.md`).
- `PH2-02` OpenAPI delta covering capacity pressure admin API, queue/worker admin endpoints, ops UI feed endpoints, report freshness envelope.
- `PH2-03` Domain event contract v2 — outbox envelope shape, event type registry, version field, idempotency key, partition key.
- `PH2-04` `docs/ARCHITECTURE.md` §3 update so Phase 2 reads as process-first (same binary, worker kind split, projection worker) instead of a microservice rewrite.
- `PH2-05` Contract drift + docs guard tests in `services/api/internal/architecture/` and `scripts/check-openapi-contract.rb` so forbidden Phase 2 language and undocumented event types fail CI.
- `PH2-06` Phase 2 release checklist (gate ordering, evidence required per AC, rollback per stream).
- `PH2-07` Linear reviewer checklist template referenced by every PH2-xx PR.

Out of scope (handled elsewhere):

- Runtime behavior of any endpoint listed in the OpenAPI delta — implemented by the consuming WS (WS3 capacity pressure, WS4 queue admin, WS5 ops feed + freshness).
- Event payload semantics (which WS emits which event, retry, dead-letter) — WS4 owns delivery; the WS that emits owns payload meaning.
- k6 thresholds / baseline numbers — WS2.

## 3. Acceptance Criteria

| AC | Given | When | Then |
| --- | --- | --- | --- |
| WS1-AC-1 | The five workstream specs are reviewed | Reviewer reads each file | Each contains Summary, Acceptance Criteria, Edge Cases, NFRs, Minimal API/Data Contract (or explicit N/A with reason), 12-Factor notes, Tests/Verification, Non-goals, and rollback/disable for any runtime change it proposes. |
| WS1-AC-2 | Any child issue `PH2-02`..`PH2-47` is opened | Its scope is compared to §3 of `phase2-scale-hardening.md` and WS specs | It maps to exactly one primary workstream; cross-stream review does not change ownership. |
| WS1-AC-3 | `PH2-02` OpenAPI delta is merged | `ruby scripts/check-openapi-contract.rb` runs | Contract test passes, and every new path/operation has a consuming WS spec referencing it. |
| WS1-AC-4 | `PH2-03` event contract v2 is merged | A consumer adds a new event type | The event type is registered in the contract doc and an architecture test rejects unknown event types in `outbox_events`. |
| WS1-AC-5 | `PH2-05` guard tests are merged | A Phase 2 doc adds forbidden language (Kafka / Kubernetes / service mesh / cross-region HA / "Phase 2 microservices") | `TestPhase2DocsDoNotClaimDeferredInfraIsRequired` fails CI before merge. |
| WS1-AC-6 | Release runs Phase 2 gate | Each WS produces evidence per `PH2-06` checklist | Reviewer sign-off references concrete test/log/baseline links, not docs-only claims. |
| WS1-AC-7 | A PR is opened for any PH2 issue | Reviewer applies `PH2-07` template | Checklist includes ownership, AC mapping, rollback/disable path, evidence link, 12-Factor impact. |

## 4. Edge Cases

| # | Scenario | Expected behavior |
| --- | --- | --- |
| WS1-E-1 | A child issue overlaps two workstreams (e.g. ops UI needs capacity API + queue feed) | Primary owner is the surface owner (UI = WS5); contract dependencies are listed under "Depends on" but ownership does not split. |
| WS1-E-2 | A WS proposes a runtime change without rollback/disable note | Reviewer rejects PR per `PH2-07`; spec must specify env flag or revert path before merge. |
| WS1-E-3 | OpenAPI delta adds an endpoint with no consuming WS spec | `ruby scripts/check-openapi-contract.rb` and reviewer checklist both fail; either the WS spec is updated or the endpoint is removed. |
| WS1-E-4 | Event contract v2 needs a breaking change after `PH2-03` merges | New event type is added (versioned), consumers migrate; the old type stays valid until projection rebuild evidence shows zero in-flight rows. |
| WS1-E-5 | Docs guard test produces false positive on legitimate "Phase 3 may use Kafka" wording | Guard test is scoped to forbid `phase 2 requires/uses` only, not future-tense Phase 3 prose; fix the test, not the doc. |
| WS1-E-6 | A reviewer marks AC complete from docs alone | PH2-06 release checklist rejects docs-only evidence; PR is reopened until matching test/log/baseline is produced. |

## 5. Non-Functional Requirements

| Category | Requirement | Metric |
| --- | --- | --- |
| Scope discipline | Phase 2 docs stay process-first; deferred infra (Kafka / Kubernetes / service mesh / cross-region HA) appears only as decision-gate language. | `TestPhase2DocsDoNotClaimDeferredInfraIsRequired` green. |
| Contract stability | OpenAPI delta breaking changes require version bump or additive path. | `scripts/check-openapi-contract.rb` exit 0. |
| Reviewability | Every WS spec ≤ 500 lines (file-size guard exempts docs but keep parity). | Manual review; aim ≤ 300. |
| Traceability | Every PH2-xx PR cites its owning WS spec and matrix AC. | `PH2-07` checklist. |

## 6. Minimal API / Data Contract (Scope Only)

WS1 does not introduce runtime endpoints. It scopes what `PH2-02` and `PH2-03` may add. Concrete payloads are written in those child specs, not here.

OpenAPI delta scope (`PH2-02`, additive only):

```text
GET    /api/v1/admin/ops/capacity-pressure        # consumed by WS5 ops UI, owned by WS3
GET    /api/v1/admin/ops/queues                   # owned by WS4
GET    /api/v1/admin/ops/report-freshness         # owned by WS5
GET    /api/v1/admin/reports?as_of=               # freshness envelope, owned by WS5
```

WS4 queue replay is intentionally a same-binary one-off admin process (`cets ops replay`), not a long-lived HTTP endpoint.

Report freshness envelope shape (`PH2-02`, normative):

```json
{
  "success": true,
  "data": { "...": "..." },
  "meta": { "as_of": "2026-05-19T12:34:56Z", "lag_seconds": 17, "source": "reporting_projection" },
  "error": null
}
```

Event contract v2 envelope (`PH2-03`, normative):

```json
{
  "event_id": "uuid",
  "event_type": "registration.confirmed.v2",
  "schema_version": 2,
  "occurred_at": "2026-05-19T12:34:56Z",
  "idempotency_key": "string",
  "partition_key": "event_id or employee_id",
  "payload": { "...": "..." }
}
```

Event type registry lives in `docs/specs/phase2-ws4-async-notification.md` §6 and is enforced by an architecture test added in `PH2-05`.

## 7. 12-Factor Notes

- **Codebase**: one repo, one binary; WS1 docs do not introduce a second runtime.
- **Config**: WS1 changes no env vars. `PH2-02` additions surface read-only ops endpoints; their auth uses existing `PROVIDER_TOKEN_SECRET` / `TOKEN_SIGNING_SECRET`.
- **Backing services**: no new attached resources. Existing PostgreSQL / Redis / MinIO / Mailhog remain.
- **Build / release / run**: same Docker image; release gate ordering is documented in `PH2-06`.
- **Processes**: no new process types from WS1; worker kind split is WS4's contract that WS1 publishes.
- **Logs**: docs guard does not change log surface. Reviewer checklist (`PH2-07`) requires consumers to confirm structured-JSON-to-stdout + redaction stays intact.
- **Admin processes**: `PH2-06` enumerates admin one-offs (migrate, seed, hr-sync, queue replay from WS4, projection rebuild from WS5) and the order they run during release.

## 8. Tests / Verification

- `cd services/api && go test ./internal/architecture -count=1` — Phase 1 + Phase 2 docs guards stay green; `PH2-05` adds event-type registry test and forbidden-language regression cases.
- `ruby scripts/check-openapi-contract.rb` — runs on every PR touching `docs/openapi*`.
- Manual: each PH2-xx PR lists owning WS spec, matrix AC, rollback path, evidence link (per `PH2-07`).
- `git diff --check` clean for any WS1 docs PR.

## 9. Rollback / Disable

WS1 outputs are docs + guard tests; no runtime feature flags apply. Disable paths:

- Docs guard tests: revert the test file or relax the forbidden list in a follow-up PR — does not affect running services.
- OpenAPI delta (once merged by `PH2-02`): consuming endpoints are owned by WS3/WS4/WS5 and each carries its own runtime disable path. WS1 has no runtime to roll back.
- Event contract v2: rollback is consumer-side (WS4 worker reads both v1 and v2 envelopes during migration window, see WS4 §9).

## 10. Non-Goals

- Do not write child issue specs for `PH2-02`..`PH2-47` inside the WS specs; child specs live in their own files when authored.
- Do not change runtime code, migrations, OpenAPI payloads, worker, or frontend in a WS1 PR (`PH2-02`, `PH2-03`, `PH2-05` are separate PRs even though WS1 owns them).
- Do not absorb Phase 1 backlog into Phase 2 ACs; reference only.
- Do not introduce Kafka, Kubernetes, service mesh, cross-region HA, or new independently-deployed services as Phase 2 deliverables.

## 11. Cross-Stream Dependencies

| Consumer WS | Consumes from WS1 | Where it lands |
| --- | --- | --- |
| WS2 | Acceptance matrix capacity targets (§1.1 of `phase2-scale-hardening.md`) | k6 thresholds, baseline report. |
| WS3 | OpenAPI delta `capacity-pressure` path | Capacity pressure admin API implementation. |
| WS4 | Event contract v2, worker kind config contract | Outbox envelope migration, worker split. |
| WS5 | Report freshness envelope, ops UI feed paths | Reports API, ops UI, projection rebuild admin. |
