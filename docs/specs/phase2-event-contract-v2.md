# Phase 2 Domain Event Contract v2

> Normative contract. Parent: `COR-47` / `[PH2-03]`. Owner: WS1 (Person A).
> Anchors: `docs/specs/phase2-ws1-contracts-release.md` §6, `docs/specs/phase2-ws4-async-notification.md` §6, `docs/specs/phase2-ws5-reporting-ops.md` §6, `CLAUDE.md` non-negotiable correctness rules.

## 1. Summary

PH2-03 freezes the wire shape of Phase 2 domain events written to the PostgreSQL `outbox_events` table. Every Phase 2 emitter writes a versioned envelope into the JSONB `payload` column. Every consumer (notification worker, reporting projection worker, compensation worker, queue replay) reads through this envelope.

Consumers stay idempotent because each envelope carries an `idempotency_key`. Workers can shard / parallelize without re-ordering across an aggregate because each envelope carries a `partition_key`. Replay (`PH2-34`) is safe because consumers deduplicate by `idempotency_key`, not by row id.

The PostgreSQL outbox stays the committed event source. No external broker is introduced.

## 2. Scope

In scope:

- Envelope v2 normative shape (§3) — wire format of the `outbox_events.payload` JSON document.
- Event type registry (§4) — closed set of `event_type` values consumers may handle.
- Per-event payload field schemas (§5) — minimal required keys and their meaning.
- Idempotency-key recipes (§6) — deterministic per-event recipe so re-emitting an event derives the same key.
- Partition-key recipes (§7) — per-event hint for sharded consumers and ordered consumers.
- Forbidden fields and patterns (§8) — what may never appear in a payload.
- Fixture set + validator test (§9) — `services/api/internal/eventcontract/testdata/*.json` covers every registry entry; `event_contract_test.go` enforces envelope, registry coverage, and safety.

Out of scope:

- Emitter rewrites in `internal/ticketing/*` — owned by the emitting workstream (WS3 for booking, WS4 for notifications, WS5 for projection update events).
- Outbox table migration (`PH2-30`, additive columns: `schema_version`, `idempotency_key`, `partition_key`, `dead_letter_at`, `retry_count`, `last_error`) — owned by WS4.
- Architecture test that rejects unknown `event_type` rows in the live `outbox_events` table — owned by `PH2-05`.
- Consumer-side dedup table or upsert strategy — owned by WS4 / WS5 per event family.
- Outbox v1 → v2 backfill — none required: emitters produce v2 from cutover, consumers tolerate both (WS4 §9).

## 3. Envelope v2 (normative)

```json
{
  "event_id": "uuid",
  "event_type": "registration.confirmed.v2",
  "schema_version": 2,
  "occurred_at": "2026-05-19T12:34:56Z",
  "idempotency_key": "registration.confirmed:reg_01HXXX",
  "partition_key": "evt_01HXXX",
  "payload": { "...": "..." }
}
```

Required keys (all top-level): `event_id`, `event_type`, `schema_version`, `occurred_at`, `idempotency_key`, `partition_key`, `payload`.

Constraints:

- `event_id` — RFC 4122 UUID or repo-internal id (`out_…`, `evt_…`); must be unique within `outbox_events`.
- `event_type` — must equal one entry in §4 registry; suffix `.v2` is normative.
- `schema_version` — integer literal `2` for this contract; `3+` reserved for future PH3 envelope.
- `occurred_at` — RFC 3339 UTC timestamp with `Z` suffix; no offsets, no naive timestamps.
- `idempotency_key` — non-empty string; recipe per event in §6. Two emissions of the same logical event MUST produce the same key. Workers MUST treat duplicate keys as a no-op.
- `partition_key` — non-empty string; recipe per event in §7. Same logical aggregate MUST produce the same partition key so sharded consumers see events in order.
- `payload` — object; per-event keys per §5. Forbidden keys per §8 MUST NOT appear.

The envelope is the document stored in `outbox_events.payload`. The columns `outbox_id`, `aggregate_id`, `event_type` continue to exist; emitters MUST keep `outbox_events.event_type` in sync with `payload.event_type`, and `outbox_events.aggregate_id` in sync with the dominant aggregate id (`registration_id`, `ticket_id`, `event_id`, `export_id`, `batch_id`, `review_id`, depending on family — see §7).

## 4. Event Type Registry

| Event type | Producing surface |
| --- | --- |
| `registration.confirmed.v2` | Booking commit (FCFS confirm + lottery winner promotion). |
| `registration.cancelled.v2` | Self-cancel, admin cancel, system cancel from no-show cooldown. |
| `registration.waitlisted.v2` | Booking commit when capacity full and policy = waitlist. |
| `registration.promoted.v2` | Waitlist → confirmed (cancellation backfill or lottery promotion). |
| `ticket.issued.v2` | Ticket row created after confirmed registration. |
| `ticket.revoked.v2` | Admin revoke, event cancellation cascade. |
| `ticket.expired.v2` | Event closed past redemption window without check-in. |
| `checkin.recorded.v2` | Online scan commit or offline batch sync commit. |
| `notification.requested.v2` | Application service requests a delivery (email or in-app); consumed by notification worker. |
| `reservation.compensation.release_required.v2` | Redis reservation hold must be released after DB rollback, timeout, or non-confirmed booking outcome. |
| `report.export.requested.v2` | Admin enqueues a CSV export. |
| `report.export.completed.v2` | Export worker writes the CSV to object store. |
| `report.export.failed.v2` | Export worker exhausts retries. |
| `hr_sync.batch.completed.v2` | HR sync admin process commits a batch (`cets hr-sync`). |
| `eligibility.impact_review.created.v2` | Rule change creates a review row impacting existing registrations. |
| `reporting.projection.update_required.v2` | Any domain event that requires a downstream projection re-derivation (WS5 read model). |

Total: 16 types. `PH2-05` adds an architecture test that fails if `outbox_events.event_type` contains a value outside this registry once Phase 2 is live; until then the test asserts only that every registry entry has a fixture.

## 5. Per-Event Payload Schemas

All payloads are JSON objects. Field types: `id` = string; `uuid` = RFC 4122 string; `ts` = RFC 3339 UTC; `int` = JSON integer; `enum` = closed string; `obj` = nested JSON object. Optional fields are marked `(opt)`; everything else is required.

### 5.1 `registration.confirmed.v2`

```json
{ "registration_id": "reg_…", "event_id": "evt_…", "employee_id": "EMP-…",
  "attendee_count": 1, "policy": "first_come_first_served|lottery",
  "city": "Taipei", "confirmed_at": "2026-05-19T12:34:56Z" }
```

### 5.2 `registration.cancelled.v2`

```json
{ "registration_id": "reg_…", "event_id": "evt_…", "employee_id": "EMP-…",
  "reason": "employee_initiated|admin|no_show_cooldown|event_cancelled",
  "cancelled_at": "2026-05-19T12:34:56Z" }
```

### 5.3 `registration.waitlisted.v2`

```json
{ "registration_id": "reg_…", "event_id": "evt_…", "employee_id": "EMP-…",
  "waitlist_position": 3, "waitlisted_at": "2026-05-19T12:34:56Z" }
```

### 5.4 `registration.promoted.v2`

```json
{ "registration_id": "reg_…", "event_id": "evt_…", "employee_id": "EMP-…",
  "from_status": "waitlisted", "to_status": "confirmed",
  "reason": "capacity_freed|lottery_winner",
  "promoted_at": "2026-05-19T12:34:56Z" }
```

### 5.5 `ticket.issued.v2`

```json
{ "ticket_id": "tkt_…", "registration_id": "reg_…", "event_id": "evt_…",
  "employee_id": "EMP-…", "issued_at": "2026-05-19T12:34:56Z" }
```

Forbidden: `qr_token`, `signed_token`, `qr_payload`, any base64 ticket body. Consumers fetch the signed QR on demand from the ticket service.

### 5.6 `ticket.revoked.v2`

```json
{ "ticket_id": "tkt_…", "event_id": "evt_…",
  "reason": "admin_action|event_cancelled",
  "revoked_at": "2026-05-19T12:34:56Z" }
```

### 5.7 `ticket.expired.v2`

```json
{ "ticket_id": "tkt_…", "event_id": "evt_…",
  "expired_at": "2026-05-19T12:34:56Z" }
```

### 5.8 `checkin.recorded.v2`

```json
{ "checkin_id": "chk_…", "ticket_id": "tkt_…", "event_id": "evt_…",
  "device_id": "dev_…", "staff_id": "EMP-…",
  "source": "online|offline_batch",
  "scanned_at": "2026-05-19T12:34:56Z",
  "synced_at": "2026-05-19T12:35:01Z" }
```

`employee_id` (ticket owner) is intentionally omitted — consumers join via `ticket_id`. No PII surfaces here.

### 5.9 `notification.requested.v2`

```json
{ "notification_id": "ntf_…",
  "category": "registration_confirmed|registration_cancelled|waitlist_promoted|reminder|export_ready",
  "channel": "email|in_app",
  "recipient_employee_id": "EMP-…",
  "template_key": "registration.confirmed.v1",
  "data_refs": { "registration_id": "reg_…", "event_id": "evt_…" },
  "requested_at": "2026-05-19T12:34:56Z" }
```

Forbidden: `recipient_email`, `subject`, `body`, `html_body`, rendered text of any kind. The notification worker re-resolves recipient address and template body at delivery time from authoritative tables.

### 5.10 `reservation.compensation.release_required.v2`

```json
{ "reservation_id": "resv_…",
  "event_id": "evt_…",
  "reason": "db_rollback|expired_hold|non_confirmed_booking",
  "requested_at": "2026-05-19T12:34:56Z" }
```

Forbidden: employee identifiers, recipient details, Redis keys, or raw Redis values. The compensation worker derives authoritative capacity from PostgreSQL before releasing an advisory hold.

### 5.11 `report.export.requested.v2`

```json
{ "export_id": "exp_…", "report_type": "participation",
  "requested_by_employee_id": "EMP-…",
  "filters": { "from": "2026-05-01T00:00:00Z", "to": "2026-05-31T23:59:59Z" },
  "requested_at": "2026-05-19T12:34:56Z" }
```

### 5.12 `report.export.completed.v2`

```json
{ "export_id": "exp_…", "report_type": "participation",
  "object_key": "exports/participation/2026/05/19/exp_01HXXX.csv",
  "row_count": 1234, "byte_size": 482910,
  "completed_at": "2026-05-19T12:34:56Z" }
```

Forbidden: signed download URL, presigned S3 URL, bearer token of any kind. Consumers (e.g. notification worker shipping "export ready" email) resolve a fresh signed URL from object store at send time.

### 5.13 `report.export.failed.v2`

```json
{ "export_id": "exp_…", "report_type": "participation",
  "reason_code": "query_timeout|object_store_unavailable|unexpected",
  "failed_at": "2026-05-19T12:34:56Z" }
```

Forbidden: stack trace, raw SQL, raw provider error body. `reason_code` is a closed enum maintained by WS5; messages live in logs.

### 5.14 `hr_sync.batch.completed.v2`

```json
{ "batch_id": "hrb_…", "source": "hr_csv|hr_api",
  "employee_count": 4231, "created_count": 12,
  "updated_count": 87, "deactivated_count": 3,
  "completed_at": "2026-05-19T12:34:56Z" }
```

### 5.15 `eligibility.impact_review.created.v2`

```json
{ "review_id": "rev_…", "event_id": "evt_…",
  "rule_version": 7, "impacted_registration_count": 14,
  "created_at": "2026-05-19T12:34:56Z" }
```

### 5.16 `reporting.projection.update_required.v2`

```json
{ "projection_name": "event_participation_summary",
  "aggregate_type": "event|registration|ticket|checkin",
  "aggregate_id": "evt_…",
  "trigger_event_id": "out_…",
  "trigger_event_type": "checkin.recorded.v2",
  "requested_at": "2026-05-19T12:34:56Z" }
```

## 6. Idempotency Key Recipes (normative)

The recipe is deterministic: given the same logical event, an emitter — including a retry, a replay, or a parallel attempt — MUST produce the same key. Workers MUST treat duplicates as a no-op via consumer-side upsert or dedup table.

| Event type | Recipe |
| --- | --- |
| `registration.confirmed.v2` | `registration.confirmed:{registration_id}` |
| `registration.cancelled.v2` | `registration.cancelled:{registration_id}:{cancelled_at}` |
| `registration.waitlisted.v2` | `registration.waitlisted:{registration_id}` |
| `registration.promoted.v2` | `registration.promoted:{registration_id}:{promoted_at}` |
| `ticket.issued.v2` | `ticket.issued:{ticket_id}` |
| `ticket.revoked.v2` | `ticket.revoked:{ticket_id}` |
| `ticket.expired.v2` | `ticket.expired:{ticket_id}` |
| `checkin.recorded.v2` | `checkin.recorded:{ticket_id}` |
| `notification.requested.v2` | `notification.requested:{notification_id}` |
| `reservation.compensation.release_required.v2` | `reservation.compensation.release_required:{reservation_id}` |
| `report.export.requested.v2` | `report.export.requested:{export_id}` |
| `report.export.completed.v2` | `report.export.completed:{export_id}` |
| `report.export.failed.v2` | `report.export.failed:{export_id}` |
| `hr_sync.batch.completed.v2` | `hr_sync.batch.completed:{batch_id}` |
| `eligibility.impact_review.created.v2` | `eligibility.impact_review.created:{review_id}` |
| `reporting.projection.update_required.v2` | `reporting.projection.update_required:{projection_name}:{aggregate_id}:{trigger_event_id}` |

Events that legitimately recur for the same aggregate (`cancelled`, `promoted`) include a discriminating timestamp so a future re-cancel after a re-confirm is not silently dropped. `ticket.expired.v2` keys on `ticket_id` because expiry happens at most once per ticket. `reporting.projection.update_required.v2` keys on the triggering outbox row so a single domain event fans out to projections at most once.

Outbox v2 enforces uniqueness at the DB layer:

```sql
CREATE UNIQUE INDEX outbox_events_idem_key_idx
  ON outbox_events (event_type, idempotency_key) WHERE idempotency_key IS NOT NULL;
```

Emitters MAY catch unique-violation and treat it as success (idempotent emit).

## 7. Partition Key Recipes (normative)

| Family | Partition key | Rationale |
| --- | --- | --- |
| `registration.*` | `event_id` | Capacity / waitlist / promotion ordering within an event. |
| `ticket.*` | `event_id` | Ticket lifecycle ordering within an event. |
| `checkin.recorded.v2` | `event_id` | Per-event projection consistency. |
| `notification.requested.v2` | `recipient_employee_id` | Per-recipient ordering avoids out-of-order "cancelled then confirmed" emails. |
| `reservation.compensation.release_required.v2` | `event_id` | Reservation compensation must preserve capacity ordering within an event. |
| `report.export.*` | `export_id` | One export's request/complete/fail must observe order. |
| `hr_sync.batch.completed.v2` | `batch_id` | One batch is its own partition. |
| `eligibility.impact_review.created.v2` | `event_id` | Reviews scoped to one event. |
| `reporting.projection.update_required.v2` | `projection_name` | Projection-worker fan-out groups by projection. |

Partition key is advisory for sharding; correctness still relies on `idempotency_key` + consumer-side dedup. Single-worker deployments (Phase 1 baseline) ignore partition key.

## 8. 12-Factor Safety: Forbidden Fields and Patterns

Payloads are persisted in `outbox_events.payload`, replayed (`PH2-34`), logged in structured worker logs, and exposed (redacted) by `/admin/ops/notification-deliveries`. They are the highest-leakage surface in Phase 2.

The following keys MUST NOT appear anywhere in a payload (top-level or nested), in any envelope, for any event type:

```
email, recipient_email, email_address, sender_email,
full_name, display_name, given_name, family_name,
phone, phone_number, mobile, address, street, postal_code,
qr_token, signed_token, provider_token, bearer_token, access_token,
refresh_token, jwt, session_token, qr_payload,
password, passcode, pin, secret, credentials, private_key,
download_url, signed_url, presigned_url
```

The following patterns MUST NOT match anywhere in the serialized envelope JSON for any fixture:

- Email-shaped substring: `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`.
- JWT-shaped substring: `eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`.

Authorized references use opaque ids only (`recipient_employee_id`, `export_id`, `object_key`). Consumers re-resolve PII / signed material at use time from authoritative tables and object store.

Note: `EMP-…` employee identifiers are HR-system identifiers, not PII, and are allowed; full names, emails, and phone numbers are not.

## 9. Tests / Verification

Authoritative tests live in `services/api/internal/eventcontract/`:

- `TestRegistryHasFixtureForEveryEntry` — every entry in §4 has a corresponding `testdata/<event-type>.json`; no orphan fixtures.
- `TestFixtureEnvelopeShape` — each fixture parses; required top-level keys present; `schema_version == 2`; `event_type` matches filename; `occurred_at` parses as RFC 3339 UTC; `idempotency_key` and `partition_key` non-empty.
- `TestFixtureNoForbiddenKeys` — recursive walk; no §8 forbidden key appears at any depth.
- `TestFixtureNoForbiddenPatterns` — serialized envelope JSON does not match the email or JWT regex.
- `TestValidatorRejectsMissingSchemaVersion` — negative: mutate a known-good fixture to drop `schema_version`; validator returns error.
- `TestValidatorRejectsForbiddenEmail` — negative: mutate a known-good payload to inject an email-shaped string; validator returns error.
- `TestValidatorRejectsForbiddenJWT` — negative: mutate a known-good payload to inject a JWT-shaped string; validator returns error.

Run:

```bash
cd services/api && go test ./internal/eventcontract -count=1
cd services/api && go test ./internal/architecture -count=1
```

The architecture run verifies the forbidden-language doc guard (`TestPhase2DocsDoNotClaimDeferredInfraIsRequired`) stays green after this contract lands.

## 10. Rollback / Disable

PH2-03 ships docs + fixtures + a Go test. There is no runtime feature flag.

- Disable path: revert this PR. No data is written by this change; no consumer is wired to the contract until WS3 / WS4 / WS5 emitters land.
- Forward compatibility: emitters writing v2 envelopes coexist with consumers tolerating both v1 (raw payload, no envelope) and v2 (envelope) during the WS4 migration window per `phase2-ws4-async-notification.md` §9. Until envelope v2 is emitted in production, the validator only enforces fixture shape, not runtime row shape.
- Breaking changes (e.g. renaming a payload key, removing a registry entry) are forbidden in place; emit a new `.v3` event type, register it, ship the new fixture, and migrate consumers per the staged process in WS4 §9.

## 11. Non-Goals

- Do not introduce Kafka, NATS, RabbitMQ, SQS, or any external broker. PostgreSQL `outbox_events` is the queue.
- Do not emit `.v2` events in production code as part of this PR — emitter rewrites are in their owning workstream's backlog.
- Do not add Go structs or generated types for envelopes here. Concrete types may follow in a downstream PR if WS4 / WS5 consumers need them; the contract is fixture-driven so non-Go clients (e.g. a future read-replica consumer) can comply.
- Do not log full envelope payloads — structured logs include `trace_id`, `event_id`, `event_type`, `idempotency_key`, `partition_key`, `outcome`, `latency_ms`; never the `payload` body.
- Do not change Phase 1 v1 event types in this PR; legacy types coexist until each consumer migrates per WS4 §9.

## 12. Open Questions

- Should `event_id` be re-derived from `outbox_id` at insert time to drop one source of drift, or kept as an emitter-supplied UUID for cross-system tracing? Pending WS4 emitter design.
- `reporting.projection.update_required.v2` may need a `version_hint` int for projection idempotency at the row level. Pending WS5 `PH2-42` (read model worker) design.
- A future `notification.delivered.v2` / `notification.dead_lettered.v2` pair may be needed to feed audit / ops UI from worker side; deferred to WS4 `PH2-36` design.
