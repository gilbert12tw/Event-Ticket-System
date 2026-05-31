# Feature: Phase 1 Compose Gate Hardening

## Summary

Harden the Phase 1 production-completion gate after the `cets-verify` no-go run. The fix keeps the Phase 1 Go modular monolith and same-binary worker boundary while addressing live Playwright overflow, k6 release waitlist semantics, report export outbox backlog, and local SMTP log privacy.

## Acceptance Criteria

- [ ] AC-1: Given a live Compose event detail page with a generated `evt_...` identifier, When Playwright checks the required viewport projects, Then the page has no horizontal overflow.
- [ ] AC-2: Given a capacity-one event with one confirmed and one waitlisted employee, When the confirmed employee cancels, Then k6 release verifies cancellation-triggered auto-promotion by observing the promoted employee ticket.
- [ ] AC-3: Given release-load notification backlog and a pending report export, When the worker runs with configured batch processing, Then the report export becomes ready within the release gate wait window.
- [ ] AC-4: Given local Compose uses Mailhog, When notification emails are sent, Then local container logs do not expose employee-specific recipient addresses.
- [ ] AC-5: Given a fresh `cets-verify` runtime, When the full Phase 1 Compose verification run executes, Then config, build, migration, app/worker startup, Playwright, k6 smoke, k6 release, log scan, and cleanup all pass.
- [ ] AC-6: Given a `checkin_staff` user opens the live offline check-in page, When the page loads events for package selection, Then it can read the event list without gaining event create, update, state-change, duplicate, or archive permissions.

## Edge Cases

| # | Scenario | Expected Behavior |
|---|----------|-------------------|
| E-1 | Ticket title falls back to a long event id | Wrap within the panel without increasing document width. |
| E-2 | Worker batch size is zero, negative, or malformed | Fail fast during config load/validation. |
| E-3 | Report export and older notification outbox rows are both pending | Claim `report.export.requested` first, preserving FIFO inside each priority class. |
| E-4 | SMTP redirect is unset | Use the original recipient so production behavior is unchanged. |
| E-5 | SMTP redirect is set | Use the redirect address for the SMTP envelope and `To:` header while preserving business delivery records in PostgreSQL. |

## Non-Functional Requirements

| Category | Requirement | Metric |
|----------|-------------|--------|
| UI Stability | Ticket panels must not create horizontal scroll at 375, 768, 1024, or 1440 widths. | Playwright overflow assertion passes. |
| Worker Throughput | Worker drains bounded batches without local durable state. | k6 release report export ready check passes under default 30s profile. |
| Config | New behavior is environment-configured. | `WORKER_BATCH_SIZE` and `MAILER_REDIRECT_TO` are parsed and validated at startup. |
| Observability | Logs remain stdout/stderr streams without employee-specific mail recipients in local Compose. | App/worker/Mailhog log scan has no `e1001@cets.local` or `e1002@cets.local` recipient leakage. |
| Reliability | Outbox processing remains idempotent and retry-safe. | DB-backed worker tests cover multi-row processing, priority, retry, and redirect behavior. |

## Minimal API Contract

No public HTTP API contract changes. Existing envelopes remain unchanged:

```json
{ "success": true, "data": {}, "error": null }
```

New runtime config:

```text
WORKER_BATCH_SIZE=25
MAILER_REDIRECT_TO=notifications@cets.local
```

## 12-Factor Compliance Notes

- Config: Worker batch size and SMTP redirect are environment variables.
- Backing Services: PostgreSQL remains the outbox source of truth; SMTP and MinIO remain attached resources.
- Processes: Worker state remains in PostgreSQL, not memory or local files.
- Concurrency: Bounded batch claims use row locks and preserve idempotency.
- Logs: App and worker keep structured stdout logs; local Mailhog recipient leakage is removed through redirect config.
- Admin Processes: Full verification still uses explicit migrate as a one-off same-image process.
