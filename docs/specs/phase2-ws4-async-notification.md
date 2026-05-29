# Phase 2 WS4 — Async Platform and Notification Isolation

> Workstream-level spec. Parent: `COR-42` / `[PH2-WS4]`. Owner: Person D.
> Anchors: `docs/specs/phase2-scale-hardening.md` (§1.4, §3.4, §4), Phase 1 baseline `docs/specs/phase1-production-upper-bound.md` (AC-10), correctness rules in `CLAUDE.md`.

## 1. Summary

WS4 owns the Phase 2 async platform built on the existing PostgreSQL transactional outbox: outbox envelope v2 (versioned, idempotency + partition keys), same-binary worker kind split (notification, projection, compensation, …) configurable per process, notification isolation so a slow / failing SMTP path cannot starve booking-critical events, retry / backoff / dead-letter policy, queue replay admin process, worker graceful shutdown hardening, notification delivery admin visibility, and worker crash / retry integration tests.

WS4 stays inside the Phase 1 process model: one Go binary, one or more same-binary worker processes selected by env. No new message brokers, no independently deployed services as Phase 2 deliverables.

## 2. Scope

In scope (`PH2-30`..`PH2-37`):

- `PH2-30` Outbox envelope v2 migration (additive columns: `schema_version`, `idempotency_key`, `partition_key`, optional `dead_letter_at`; lease semantics preserved; consumers tolerate v1 + v2 during migration).
- `PH2-31` Worker kind split config (env-driven kind selection; one binary, many process roles).
- `PH2-32` Notification worker isolation (notification kind runs as its own process; SMTP backpressure / failures do not block projection or compensation queues).
- `PH2-33` Retry, backoff, and dead-letter policy (bounded retries, exponential backoff with jitter, dead-letter row with reason and last error, no silent drops).
- `PH2-34` Queue replay admin process (`cets ops replay --kind=… --from=… --to=…`) replays from dead-letter or by time window, with idempotent consumers.
- `PH2-35` Worker graceful shutdown hardening (SIGTERM → drain leased rows → release leases → exit; never partial commit / partial side effect).
- `PH2-36` Notification delivery admin visibility (read-only HTTP feed of recent deliveries, retries, dead-letter — consumed by WS5 ops UI).
- `PH2-37` Worker crash / retry integration tests (kill-9 mid-lease, duplicate delivery prevention, dead-letter visibility, replay idempotency).

Out of scope:

- Booking hot path, reservation, idempotency at the booking layer — WS3.
- Reporting projection schema + read model worker behavior — WS5 (WS4 provides the worker kind + envelope; WS5 implements the projection).
- Metric names / trace schema — WS2 (WS4 emits per-kind metrics defined in WS2 §6).
- Replacement of PostgreSQL outbox with an external broker — explicitly out of Phase 2.

## 3. Acceptance Criteria

| AC | Given | When | Then |
| --- | --- | --- | --- |
| WS4-AC-1 | Outbox v1 rows exist before deployment | v2 migration runs | Existing rows remain readable; new writes use v2 envelope; worker processes both versions during the migration window. |
| WS4-AC-2 | A single same-binary deployment | `WORKER_KINDS=notification,projection,compensation` env is set across multiple processes | Each process consumes only its assigned kinds; no kind is starved when another is slow. |
| WS4-AC-3 | SMTP is slow / failing | Notification worker backpressures | Projection and compensation workers continue to drain on time; outbox lag for non-notification kinds stays within acceptance matrix §1.4 (p95 < 60s, max < 180s). |
| WS4-AC-4 | An event fails delivery | Retry policy applies | Bounded retries with exponential backoff + jitter; on exhaustion, row moves to dead-letter state with `dead_letter_at`, `last_error`, retry_count; no silent drop. |
| WS4-AC-5 | An admin runs queue replay (`cets ops replay --kind=notification --from=… --to=…`) | Selected rows replay | Consumers are idempotent — duplicate delivery does not occur; audit row records the replay action and operator. |
| WS4-AC-6 | A worker receives SIGTERM mid-lease | Shutdown drain runs | Currently-leased rows are released safely (lease cleared or committed); no half-applied side effect; new leases are not acquired after SIGTERM. |
| WS4-AC-7 | A worker is killed (SIGKILL, OOM, container kill) mid-lease | Lease times out | Lease expiry returns the row to the pool; consumer idempotency prevents double side effect on the retry; integration test proves this. |
| WS4-AC-8 | An ops admin calls notification delivery feed | Recent deliveries / retries / dead-letters are queried | Read-only response with status, retry_count, last_error, redacted recipient; no full email body, no PII beyond Phase 1 redaction allowances. |
| WS4-AC-9 | Regression suite runs | Duplicate-delivery, dead-letter, replay, graceful-shutdown, crash-recovery scenarios are exercised | All assertions pass; new test added before any worker behavior change merges. |

## 4. Edge Cases

| # | Scenario | Expected behavior |
| --- | --- | --- |
| WS4-E-1 | Producer commits DB tx but worker has not yet picked up the row | Outbox guarantees at-least-once delivery; consumer idempotency (key on `idempotency_key` + consumer-side dedup table or upsert) prevents duplicate side effect. |
| WS4-E-2 | Worker picks up a row, side-effect succeeds, worker crashes before marking processed | Lease expires → row reprocessed → consumer detects already-applied via idempotency → marks processed without re-sending. |
| WS4-E-3 | SMTP returns 5xx transiently | Retry with backoff; after N retries → dead-letter; admin visibility shows the row. |
| WS4-E-4 | SMTP returns 5xx for hours (Mailhog down / prod relay down) | Backoff caps; queue grows; outbox-lag metric alerts; non-notification kinds unaffected by AC-3 isolation. |
| WS4-E-5 | A bad payload trips a consumer (panic / schema mismatch) | Consumer recovers, marks attempt failed with error reason, retries up to limit, then dead-letters. Worker process does not crash on a single bad row. |
| WS4-E-6 | Replay window overlaps with current live delivery | Idempotency dedup prevents duplicate; audit row distinguishes replay from primary delivery. |
| WS4-E-7 | Envelope v2 schema_version is unknown to the consumer | Consumer logs structured warning and dead-letters with `reason=unknown_schema_version`; never silently drops. |
| WS4-E-8 | Lease TTL is too long, slow worker holds rows under load | Lease TTL is config-bound (`OUTBOX_LEASE_TTL_SECONDS`); reviewer test asserts sane default; metric `cets_outbox_lease_held_seconds` exposes held time. |
| WS4-E-9 | Replay admin is invoked against the wrong window | Dry-run mode (`--dry-run`) prints affected count without enqueuing; default is dry-run; `--apply` is required for actual replay. |

## 5. Non-Functional Requirements

| Category | Requirement | Metric |
| --- | --- | --- |
| Reliability | At-least-once delivery; consumer idempotency makes it effectively once. | `PH2-37` duplicate-delivery test. |
| Isolation | Notification slowness does not raise non-notification kind lag or backlog. | Per-kind `cets_outbox_lag_seconds` and `cets_outbox_oldest_lag_seconds` (WS2). |
| Latency | Outbox lag p95 < 60s, max < 180s (acceptance matrix §1.4). | k6 + lag metric. |
| Recovery | Crash / SIGKILL never duplicates side effect. | `PH2-37` crash test. |
| Observability | Every retry, dead-letter, replay action is logged + auditable. | Structured log + audit row; ops API feed. |
| Disposability | SIGTERM drain completes within configured grace (default 30s) or escalates. | `PH2-35` test. |
| Backward compatibility | v1 + v2 envelopes coexist during migration. | Integration test consumes both. |

## 6. Minimal API / Data Contract

Outbox envelope v2 (normative, `PH2-30`):

```sql
ALTER TABLE outbox_events ADD COLUMN schema_version int NOT NULL DEFAULT 1;
ALTER TABLE outbox_events ADD COLUMN idempotency_key text;
ALTER TABLE outbox_events ADD COLUMN partition_key text;
ALTER TABLE outbox_events ADD COLUMN dead_letter_at timestamptz;
ALTER TABLE outbox_events ADD COLUMN last_error text;
ALTER TABLE outbox_events ADD COLUMN retry_count int NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX outbox_events_idem_key_idx
  ON outbox_events (event_type, idempotency_key) WHERE idempotency_key IS NOT NULL;
```

Envelope payload (wire shape produced by emitters, anchored in WS1 `PH2-03`):

```json
{
  "event_id": "uuid",
  "event_type": "registration.confirmed.v2",
  "schema_version": 2,
  "occurred_at": "iso8601",
  "idempotency_key": "string",
  "partition_key": "event_id|employee_id|...",
  "payload": { "...": "..." }
}
```

Event type registry (normative; `PH2-05` adds an architecture test that fails on unregistered types in `outbox_events.event_type`):

```text
registration.confirmed.v2
registration.cancelled.v2
registration.waitlisted.v2
registration.promoted.v2
ticket.issued.v2
ticket.revoked.v2
ticket.expired.v2
checkin.recorded.v2
notification.requested.v2
reservation.compensation.release_required.v2
report.export.requested.v2
report.export.completed.v2
report.export.failed.v2
hr_sync.batch.completed.v2
eligibility.impact_review.created.v2
reporting.projection.update_required.v2
```

Worker kind config (normative, `PH2-31`):

```text
WORKER_KINDS=notification,projection,compensation,export   # comma-separated
WORKER_CONCURRENCY_NOTIFICATION=4
WORKER_CONCURRENCY_PROJECTION=2
WORKER_CONCURRENCY_COMPENSATION=1
WORKER_CONCURRENCY_EXPORT=1
OUTBOX_LEASE_TTL_SECONDS=60
OUTBOX_BATCH_SIZE=100
OUTBOX_BACKOFF_BASE_MS=500
OUTBOX_BACKOFF_MAX_MS=60000
OUTBOX_RETRY_MAX=10
```

Ops admin endpoints (additive; OpenAPI added by WS1 `PH2-02`):

```text
GET  /api/v1/admin/ops/queues
  Response: per kind { name, pending, in_flight, dead_letter, p95_age_seconds, last_processed_at }
  Known kinds: notification, projection, compensation, export; unregistered or unsafe event_type rows are reported under read-only kind `unknown` and are not valid WORKER_KINDS values.

cets ops replay --kind=<kind> --from=<iso8601> --to=<iso8601> [--event-type=<type>] [--apply]
  Auth: one-off admin process using the same binary and database config
  Behavior: default dry-run returns affected count; --apply enqueues replays and writes audit row

GET  /api/v1/admin/ops/notification-deliveries?status=&cursor=&limit=
  Response: redacted delivery rows for ops visibility
  worker_kind uses the same observed kind bucket as queue status; unregistered or unsafe event_type rows are reported as `unknown` and are not retry-eligible.
```

## 7. 12-Factor Notes

- **Config**: `WORKER_KINDS`, `WORKER_CONCURRENCY_*`, `OUTBOX_LEASE_TTL_SECONDS`, `OUTBOX_BATCH_SIZE`, `OUTBOX_BACKOFF_*`, `OUTBOX_RETRY_MAX`. All env-only. Documented in `services/api/deploy/.env.example`.
- **Backing services**: PostgreSQL outbox table is the queue; no Kafka, RabbitMQ, SQS as Phase 2 deliverable. SMTP (Mailhog in dev) remains an attached resource.
- **Build / release / run**: Same Go binary. Worker process selected by `cets worker --kinds=...` (or `WORKER_KINDS` env). Migrations run as `cets migrate`. Local / CI isolation checks can layer `services/api/deploy/compose.worker-isolation.yaml` on top of `compose.yaml` with the `worker-isolation` profile to run one worker process per kind without introducing a new service codebase.
- **Processes**: Stateless workers; lease state in PostgreSQL. Multiple processes can run in parallel safely due to row-level lease + unique idempotency index.
- **Port binding**: Workers do not bind a public port. Admin ops endpoints are served by the existing `cets serve` process.
- **Concurrency**: Scale by adding worker processes of a kind. No new lock files, no shared in-memory queues.
- **Disposability**: SIGTERM → stop polling → drain in-flight leases → exit ≤ `WORKER_SHUTDOWN_GRACE_SECONDS` (default 30); escalation logs structured shutdown reason.
- **Logs**: Structured JSON to stdout; fields include `trace_id`, `event_id`, `event_type`, `worker_kind`, `retry_count`, `outcome`, `latency_ms`. No payload contents that include PII or signed tokens.
- **Admin processes**: `cets ops replay --kind=… --from=… --to=… [--apply]`, `cets ops outbox-stats`, `cets migrate`. Same binary, one-off.
- **Dev/prod parity**: Same outbox table semantics in dev/CI/prod; CI exercises crash recovery + replay.

## 8. Tests / Verification

- `go test ./internal/ticketing/outbox -count=1` — envelope v1+v2 read/write, schema migration, idempotency dedup.
- `go test ./internal/worker -count=1` — kind selection, concurrency, lease lifecycle, retry/backoff, dead-letter promotion.
- `go test ./internal/worker -run TestNotificationIsolation -count=1` — slow notification handler does not raise lag of other kinds.
- `go test ./internal/worker -run TestGracefulShutdown -count=1` — SIGTERM drain within grace, lease release, no partial commit.
- `go test ./internal/worker -run TestCrashRecovery -count=1` — simulated crash mid-lease; consumer idempotency prevents duplicate side effect.
- `go test ./internal/httpapi -run TestOpsQueueAdmin -count=1` — RBAC, replay dry-run vs apply, audit row written.
- `go test ./internal/architecture -run TestOutboxEventTypeRegistry -count=1` (added by `PH2-05`) — fails on unregistered event_type in code or fixtures.
- Live Compose gate (`live-gates` CI job) runs k6 with worker isolation enabled and asserts non-notification lag stays in NFR bounds. Use `docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml -f services/api/deploy/compose.worker-isolation.yaml --profile worker-isolation ...` for the process-isolated local topology.

## 9. Rollback / Disable

Every WS4 runtime change ships disable-capable:

- Envelope v2 migration: additive columns + nullable; rollback = drop columns via reverse migration. Consumers tolerate v1 + v2 throughout the migration window — never gated on "v2 only."
- Worker kind split (`PH2-31`): default `WORKER_KINDS=notification,projection,compensation,export` mirrors Phase 1 behavior; setting `WORKER_KINDS=*` runs all kinds in one process (Phase 1 parity). Disable a kind by removing it from the list.
- Notification isolation (`PH2-32`): isolation is purely a process-topology choice driven by `WORKER_KINDS`; rollback = redeploy with a single worker process running all kinds.
- Retry / backoff / dead-letter (`PH2-33`): tunables are env vars; emergency-disable retry by `OUTBOX_RETRY_MAX=0` (one attempt, immediate dead-letter); reviewer test asserts no silent drops in any setting.
- Replay admin (`PH2-34`): no long-lived HTTP replay route; the same binary exposes `cets ops replay` as a break-glass one-off admin process.
- Graceful shutdown (`PH2-35`): grace period via `WORKER_SHUTDOWN_GRACE_SECONDS`; setting `0` falls back to immediate exit (Phase 1 baseline) for emergency rollback.
- Notification delivery admin feed (`PH2-36`): route gated by `OPS_API_ENABLED=true` (shared with WS3/WS5 ops surfaces).
- All schema changes are reversible by a follow-up migration; no destructive `DROP TABLE` of historical rows.

## 10. Non-Goals

- Do not introduce Kafka, RabbitMQ, NATS, SQS, or any external broker as a Phase 2 deliverable. PostgreSQL outbox is the boundary.
- Do not split workers into independently deployed services. Same binary, multiple processes.
- Do not move booking commit truth out of PostgreSQL. Outbox is for side effects, not booking authority.
- Do not change OpenAPI shapes outside `PH2-02` scope; payload deltas in §6 are normative input.
- Do not log payloads containing PII, signed tokens, recipient email bodies, or provider tokens.
- Do not introduce a separate worker codebase; consumers live in `services/api/internal/ticketing` and adjacent packages.

## 11. Cross-Stream Dependencies

| Direction | Stream | Contract consumed / produced |
| --- | --- | --- |
| Consumes | WS1 | Event contract v2 (`PH2-03`), OpenAPI delta (`PH2-02`), reviewer checklist (`PH2-07`). |
| Consumes | WS2 | Outbox lag + per-kind metrics names; trace schema for worker hops. |
| Consumes | WS3 | Reservation compensation kind name; idempotency-key shape for booking-side emitters. |
| Produces | WS3 | Worker kind config (`PH2-31`) so reservation compensation can run as a kind. |
| Produces | WS5 | Outbox envelope v2 + worker kind contract — required by read model worker (`PH2-42`). Queue admin feed payload — consumed by ops UI (`PH2-46`). |
