# Phase 2 WS5 — Reporting and Ops Control Plane

> Workstream-level spec. Parent: `COR-43` / `[PH2-WS5]`. Owner: Person E.
> Anchors: `docs/specs/phase2-scale-hardening.md` (§1.4, §3.5, §4), Phase 1 baseline `docs/specs/phase1-production-upper-bound.md` (AC-11, AC-12), correctness rules in `CLAUDE.md`.

## 1. Summary

WS5 owns the Phase 2 reporting read model and the ops control plane: a reporting projection spec, projection schema, read-model worker (running on the WS4 kind split), rebuild admin process, reports API freshness contract, exports that read from the projection, ops UI surfacing capacity pressure (WS3), queue health (WS4), and report freshness, plus a conditional check-in cache decision gate that only escalates to implementation if check-in is proven to be a Phase 2 bottleneck.

Read model is derived, disposable, rebuildable, never authoritative. Booking, eligibility, ticket redemption, and audit truth stay on the operational PostgreSQL schema.

## 2. Scope

In scope (`PH2-40`..`PH2-47`):

- `PH2-40` Reporting read model spec (consumed events from envelope v2, projection invariants, rebuild semantics, freshness contract).
- `PH2-41` Reporting projection schema (denormalized tables for HR/admin reports, columns mirroring Phase 1 whitelist + Phase 2 freshness metadata).
- `PH2-42` Read model worker (WS4 worker_kind `projection`, idempotent consumer, snapshot + incremental updates).
- `PH2-43` Read model rebuild admin process (`cets ops projection-rebuild --from=epoch|timestamp`).
- `PH2-44` Reports API freshness contract (every report response carries `as_of` + `lag_seconds`; stale-beyond-threshold marks response degraded but still served).
- `PH2-45` Exports read from reporting projection (CSV exports use projection rows, not operational tables, to avoid scan contention).
- `PH2-46` Ops UI surfacing capacity pressure (WS3), queue health (WS4), report freshness (WS5) with redacted, role-gated views.
- `PH2-47` Conditional check-in cache decision gate (no implementation unless check-in-specific evidence demands it).

Out of scope:

- Booking, eligibility, ticket, check-in commit-path code — WS3 / Phase 1.
- Worker engine (`PH2-31`/`PH2-32`/`PH2-33`/`PH2-34`/`PH2-35`) — WS4. WS5 consumes the kind contract, does not own it.
- Event envelope shape (`PH2-30`) — WS4 publishes; WS5 consumes.
- Metric names, trace schema — WS2.
- New product domain features beyond Phase 1 report whitelist.

## 3. Acceptance Criteria

| AC | Given | When | Then |
| --- | --- | --- | --- |
| WS5-AC-1 | Outbox envelope v2 is in production | Read model worker subscribes to relevant event types | Projection rows are written idempotently per `(event_type, idempotency_key)`; duplicate events do not double-count. |
| WS5-AC-2 | A reporting projection schema migration runs against existing data | Rebuild from epoch is invoked | All projection rows are recomputed from the operational source-of-truth tables (not from the prior projection state); resulting counts match a fresh-DB equivalent. |
| WS5-AC-3 | A reports API is called | Response is returned | Response envelope includes `meta.as_of` and `meta.lag_seconds`; `lag_seconds` > `REPORTS_FRESHNESS_DEGRADED_SECONDS` flags `meta.degraded=true` but still serves the response. |
| WS5-AC-4 | HR runs an export | Export job reads | Reads the projection (not the operational tables); on missing projection rows, export marks the row as `pending_projection` rather than fabricating zeros. |
| WS5-AC-5 | An ops admin opens the ops UI | Page loads | Capacity pressure, queue depth + lag + dead-letter counts, report freshness, and recent replay actions are visible; payload is redacted per Phase 1 rules; role gating denies non-admin actors with audit row on attempted access. |
| WS5-AC-6 | A booking, ticket redemption, or audit lookup is invoked anywhere in the system | Code path is inspected | None of these paths read the reporting projection as authority. Architecture test enforces no imports of the projection package from booking/check-in/audit/ticket modules. |
| WS5-AC-7 | Projection lag exceeds threshold | `meta.degraded=true` is returned | Ops UI shows a "reports degraded" banner and a link to rebuild docs; alert metric `cets_reporting_lag_seconds` exposed via WS2 wiring. |
| WS5-AC-8 | Check-in performance data from `PH2-16` is reviewed | Evidence does not show check-in as the bottleneck | `PH2-47` stays a decision gate; no cache code is shipped. If evidence shifts, a new spec is written before any implementation. |

## 4. Edge Cases

| # | Scenario | Expected behavior |
| --- | --- | --- |
| WS5-E-1 | Read model worker crashes mid-batch | WS4 lease expires; row reprocessed; consumer idempotency (per `event_type + idempotency_key`) prevents duplicate row mutation. |
| WS5-E-2 | A schema migration changes projection shape | Rebuild admin process runs as part of release; reports return `degraded=true` until rebuild catches up. |
| WS5-E-3 | An export is requested while a rebuild is in progress | Export marks affected rows `pending_projection`; never serves partial counts as final. Ops UI shows rebuild progress %. |
| WS5-E-4 | Operational tables and projection drift after a manual data fix | Rebuild from epoch (or scoped time window) restores consistency; audit row records the rebuild operator + scope. |
| WS5-E-5 | A new event type is added by WS3/WS4 but not by WS5 projection | Architecture test (added in `PH2-05`) fails when an unhandled event type appears in production traffic; alternatively, projection ignores it explicitly with a registered "no-op" handler. |
| WS5-E-6 | Reports request asks for `as_of` beyond projection horizon | Returns 400 with explanation; never silently returns stale or empty data. |
| WS5-E-7 | Ops UI is loaded by a non-admin (route guard bypass attempt) | API returns 403; UI shows unauthorized state; audit row recorded. |
| WS5-E-8 | Check-in latency spikes during a non-Phase-2 hot event | `PH2-47` decision gate explicitly requires check-in-specific evidence under Phase 2 load profile, not generic latency spikes; does not promote to implementation. |

## 5. Non-Functional Requirements

| Category | Requirement | Metric |
| --- | --- | --- |
| Non-authoritative | Read model never used for booking / check-in / audit truth. | Architecture test (`PH2-05`); reviewer checklist. |
| Freshness | Reports lag p95 < 60s. | `cets_reporting_lag_seconds` (acceptance matrix §1.4). |
| Disposability | Full rebuild from epoch completes within an operationally acceptable window for 50k employees + hot-event load. | Documented in `PH2-43`; integration test runs a scoped rebuild. |
| Privacy | Exports use the same Phase 1 whitelist; no PII fields added by Phase 2. | `PH2-45` regression test + log scan. |
| Backpressure isolation | Projection worker slowness does not slow notification or compensation. | WS4 isolation (`PH2-32`) + per-kind metrics. |
| Ops visibility | Ops UI surfaces freshness, queue depth, capacity pressure, dead-letter, replay history. | `PH2-46` Playwright test. |
| Decision discipline | `PH2-47` cache only shipped on check-in-specific evidence. | Spec gating; no code in WS5 unless approved. |

## 6. Minimal API / Data Contract

Projection schema sketch (normative for `PH2-41`):

```sql
CREATE TABLE reporting_event_rollup (
  event_id           uuid PRIMARY KEY,
  title              text NOT NULL,
  capacity_type      text NOT NULL,
  capacity           int,
  confirmed_count    int NOT NULL DEFAULT 0,
  waitlist_count     int NOT NULL DEFAULT 0,
  employee_count     int NOT NULL DEFAULT 0,
  family_count       int NOT NULL DEFAULT 0,
  total_attendee     int NOT NULL DEFAULT 0,
  ticket_count       int NOT NULL DEFAULT 0,
  checkin_count      int NOT NULL DEFAULT 0,
  remaining_capacity int,
  city_distribution  jsonb NOT NULL DEFAULT '{}'::jsonb,
  starts_at          timestamptz,
  projection_as_of   timestamptz NOT NULL,
  projection_lag_ms  int NOT NULL DEFAULT 0
);

CREATE TABLE reporting_consumed_events (
  event_type      text NOT NULL,
  idempotency_key text NOT NULL,
  consumed_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (event_type, idempotency_key)
);

CREATE TABLE reporting_projection_state (
  name          text PRIMARY KEY,        -- 'rollup'
  last_applied  timestamptz NOT NULL,
  rebuild_in_progress boolean NOT NULL DEFAULT false,
  rebuild_started_at timestamptz
);
```

Reports API freshness envelope (normative for `PH2-44`; OpenAPI added by WS1 `PH2-02`):

```json
{
  "success": true,
  "data": { "...": "..." },
  "meta": {
    "as_of": "2026-05-19T12:34:56Z",
    "lag_seconds": 17,
    "degraded": false,
    "source": "reporting_projection"
  },
  "error": null
}
```

`degraded=true` when `lag_seconds > REPORTS_FRESHNESS_DEGRADED_SECONDS`. Response still served.

Ops UI feed (additive endpoints, OpenAPI added by WS1 `PH2-02`):

```text
GET /api/v1/admin/ops/dashboard
  Response: { capacity_pressure: <from WS3>, queues: <from WS4>, reports_freshness: <from WS5>, dead_letter_recent: [...], replay_recent: [...] }
  Auth: activity_admin (read subset) or hr_admin (full)
  Behavior: aggregates other ops endpoints; redacted; never authoritative.
```

Rebuild admin process (`PH2-43`):

```text
cets ops projection-rebuild --name=rollup [--from=epoch|<iso8601>] [--dry-run]
  Behavior: locks reporting_projection_state.rebuild_in_progress=true (idempotent)
  Source: replays from operational PostgreSQL source-of-truth tables (preferred) OR from outbox replay
  Resume: safe; reads watermark from reporting_projection_state
  Exit: 0 on success; non-zero with structured error log otherwise
```

## 7. 12-Factor Notes

- **Config**: `REPORTS_FRESHNESS_DEGRADED_SECONDS`, `REPORTS_FRESHNESS_HARD_LIMIT_SECONDS`, `PROJECTION_REBUILD_BATCH_SIZE`, `OPS_UI_ENABLED`. Env-only, documented in `.env.example`.
- **Backing services**: PostgreSQL only — projection tables live in the same database as operational tables. No new datastore. MinIO continues to host export artifacts (unchanged from Phase 1).
- **Build / release / run**: Same binary. Projection worker runs as `cets worker --kinds=projection` (WS4 contract). Rebuild as `cets ops projection-rebuild ...`.
- **Processes**: Stateless. Watermark + rebuild state in `reporting_projection_state`.
- **Port binding**: Ops UI is served by existing `cets serve`; no new port.
- **Concurrency**: Multiple projection workers scale by lease; consumer idempotency on `(event_type, idempotency_key)` keeps writes safe.
- **Disposability**: Projection is disposable by definition — full rebuild is supported and tested.
- **Logs**: Structured JSON to stdout; rebuild logs include scope + operator; redacted per Phase 1 rules. No PII columns added.
- **Admin processes**: `cets ops projection-rebuild ...` and (existing) `cets seed`, `cets migrate`. All one-off same-binary commands.
- **Dev/prod parity**: Same projection tables in dev/CI/prod; CI exercises rebuild on seeded data.

## 8. Tests / Verification

- `go test ./internal/ticketing/reporting -count=1` — projection update idempotency (`(event_type, idempotency_key)` dedup), rollup correctness, edge counts.
- `go test ./internal/ticketing/reporting -run TestProjectionRebuild -count=1` — rebuild from epoch produces identical results to incremental application; rebuild-in-progress flag behavior.
- `go test ./internal/httpapi -run TestReportsFreshnessEnvelope -count=1` — `meta.as_of`, `meta.lag_seconds`, `meta.degraded` rules.
- `go test ./internal/httpapi -run TestExportFromProjection -count=1` — exports read projection; `pending_projection` row state preserved.
- `go test ./internal/architecture -run TestReportingProjectionNotAuthoritative -count=1` (added by `PH2-05`) — booking / check-in / audit / ticket packages do not import the projection package.
- Playwright: ops UI route loads with `activity_admin` and `hr_admin`, redirects + audits for other roles, no horizontal overflow at 375/768/1024/1440.
- Live Compose gate: lag stays within NFR while a hot event runs; freshness envelope visible in API responses.

## 9. Rollback / Disable

- Reporting projection schema: additive tables; rollback = drop tables + revert migration; operational tables untouched.
- Read model worker: a kind from `WORKER_KINDS` (WS4); disable by removing `projection` from the list. Reports fall back to operational queries (Phase 1 behavior) via `REPORTS_SOURCE=projection|operational` (default `projection`; setting `operational` restores Phase 1 source).
- Freshness contract: `REPORTS_FRESHNESS_DEGRADED_SECONDS` and `REPORTS_FRESHNESS_HARD_LIMIT_SECONDS` are env tunables; setting both very high silences the degraded flag for emergency rollback (still served).
- Exports from projection: `EXPORTS_SOURCE=projection|operational` toggles back to Phase 1 source.
- Ops UI: `OPS_UI_ENABLED=false` hides the route in router registration; backend endpoints follow `OPS_API_ENABLED` (shared with WS3/WS4).
- Rebuild admin: dry-run by default; `--apply` required. Rebuild is idempotent and resumable; abort sets `rebuild_in_progress=false` and leaves data consistent.
- `PH2-47` check-in cache: not implemented — nothing to roll back. Decision gate document is the only artifact.

## 10. Non-Goals

- Do not use reporting projection as the source of truth for booking, eligibility, ticket redemption, check-in, or audit.
- Do not add PII columns beyond Phase 1 export whitelist.
- Do not ship check-in cache without check-in-specific bottleneck evidence (`PH2-47`).
- Do not introduce a separate analytics warehouse, read replica, BI tool, materialized-view-as-API, or external reporting service as a Phase 2 deliverable.
- Do not change OpenAPI shapes outside `PH2-02` scope; payload deltas in §6 are normative input.
- Do not couple ops UI controls to operational mutation paths (no UI button that bypasses standard admin endpoints with their own validation + audit).

## 11. Cross-Stream Dependencies

| Direction | Stream | Contract consumed / produced |
| --- | --- | --- |
| Consumes | WS1 | OpenAPI delta (`PH2-02`), event contract v2 (`PH2-03`), reviewer checklist (`PH2-07`). |
| Consumes | WS2 | Lag / queue metrics names, trace schema, `PH2-16` baseline (gates `PH2-47`). |
| Consumes | WS3 | Capacity pressure API payload (§6 of WS3) — surfaced in ops UI. |
| Consumes | WS4 | Outbox envelope v2, worker kind config, queue admin feed payload, dead-letter rows. |
| Produces | WS1 | Report freshness envelope (normative; `PH2-02` encodes it into OpenAPI). |
| Produces | WS3 / WS4 | None (consumer-only stream for upstream contracts). |
