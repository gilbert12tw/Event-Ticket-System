# Feature: COR-36 Backend Notification and Audit Metadata for Governance Events

## Summary

Normalize notification outbox and audit metadata for cancellation, no-show cooldown, and ticket holder mismatch or transfer rejection events. The backend must retain enough event/device/reason context for notification delivery and audit queries while omitting full names, email addresses, raw tokens, QR payloads, and other unnecessary PII.

## Acceptance Criteria

- [ ] AC-1: Given cancellation or no-show occurs, When outbox and audit rows are created, Then payloads include event context and omit full names, raw tokens, email addresses, and QR payloads.
- [ ] AC-2: Given holder mismatch or transfer rejection occurs, When audit logs are queried, Then metadata is redacted and includes reason, device, and event context.
- [ ] AC-3: Given the notification worker processes cancellation or no-show events, Then generated messages include activity context when available and remain retry-safe.

## Edge Cases

| # | Scenario | Expected Behavior |
|---|----------|-------------------|
| E-1 | Event context is partially unavailable | Persist IDs and any safe known fields; worker falls back to generic copy without failing the event. |
| E-2 | Duplicate cancellation or no-show processing is retried | Existing idempotency behavior prevents duplicate audit/outbox effects; worker delivery remains safe to retry. |
| E-3 | Audit metadata contains sensitive keys | Query responses redact sensitive fields before returning metadata. |
| E-4 | Holder mismatch is caused by a transfer attempt | Audit metadata records reason, event context, device context, and safe actor identifiers only. |

## Non-Functional Requirements

| Category | Requirement | Metric |
|----------|-------------|--------|
| Privacy | Full names, email addresses, raw tokens, QR payloads, provider tokens, and signatures are not persisted in these metadata payloads. | Unit or integration assertions inspect stored and queried metadata. |
| Observability | Cancellation, no-show, and check-in rejection metadata keeps event, reason, and device identifiers where available. | Audit/outbox rows contain safe context fields. |
| Failure Handling | Missing optional context does not fail notification processing. | Worker tests cover fallback message generation. |
| Idempotency | Notification worker retry behavior is unchanged. | Existing outbox status and delivery retry tests continue to pass. |

## Internal Contract

No new public HTTP endpoint or frontend contract is introduced.

Outbox/audit payloads for affected events must prefer this minimal shape when context is available:

```json
{
  "event_id": "evt_...",
  "event_title": "Activity title",
  "starts_at": "2026-05-19T10:00:00Z",
  "registration_id": "reg_...",
  "ticket_id": "tkt_...",
  "employee_id": "E1001",
  "reason": "holder_mismatch",
  "device_id": "gate-a"
}
```

The payload must not include full names, email addresses, raw signed tokens, QR payloads, provider tokens, or unredacted audit blobs.

## 12-Factor Compliance Notes

- Config: No new deploy-time config is required.
- Backing Services: PostgreSQL remains the source of truth for audit, registration, ticket, and outbox state.
- Logs: No new file logging or full PII logging is introduced.
- Processes: Worker behavior remains stateless and retry-safe through persisted outbox rows.
