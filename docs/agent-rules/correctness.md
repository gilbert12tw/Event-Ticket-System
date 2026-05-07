# Correctness Rules

Ticketing correctness takes precedence over convenience. Cache and queue layers may improve responsiveness, but PostgreSQL remains the source of truth for committed booking, ticket, check-in, and audit state in Phase 1.

## Booking and Allocation

- Redis is only a fast reservation gate.
- Successful booking must be confirmed by PostgreSQL transactions, row locks or unique constraints, and commit results.
- Final booking must recheck eligibility, event state, capacity, booking window, and allocation policy.
- Event lists may cache eligibility summaries, but cached summaries cannot authorize a booking.
- Lottery and first-come-first-served allocation must preserve auditability and explain rejection or waitlist reasons.

## Idempotency and Async Work

- Booking, cancellation, ticket generation, notification, and check-in sync must use idempotency keys or equivalent deduplication.
- Ticket generation, notifications, and reporting read-model updates may run through queue or worker flows.
- Async side effects must not block the core booking transaction.
- Combine business data and async events with transactional outbox or an equivalent reliable pattern.
- Workers must be idempotent and safe to retry.

## Ticket and Check-in

- One ticket may be successfully redeemed only once.
- Database unique constraints are the final duplicate-scan guarantee.
- Online check-in must report duplicate scans clearly and preserve the original successful redemption.
- Offline check-in remains a future boundary and must sync with first-commit-wins conflict handling.
- Offline conflicts must preserve conflict audit data.

## Audit

Record sensitive actions, including event rule changes, eligibility rule changes, allocation, ticket revocation, report export, and check-in conflicts.

Logs must not contain full PII or secrets.
