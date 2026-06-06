# Product

## Purpose

The Corporate Event Ticketing System lets internal employees find eligible
events, book seats or join waitlists, receive signed electronic tickets, and
check in once at the venue. Activity admins manage events and eligibility, while
HR/system admins review participation reports and audit records.

## Users

- Employees browse eligible events, book, cancel during the allowed window, and
  present tickets.
- Activity admins create events, publish schedules, manage eligibility, handle
  waitlists, and review notifications.
- Check-in staff redeem signed tickets online or through bounded offline sync.
- HR/system admins inspect aggregate reports, exports, and audit logs without
  exposing unnecessary personal data.

## Core Journeys

- Event setup: create or update event details, capacity, registration window,
  eligibility rules, and publication state.
- Booking: recheck eligibility and event state during final booking, prevent
  oversell, and return confirmed or waitlisted status.
- Ticketing and check-in: issue employee-bound signed tickets and allow exactly
  one successful redemption.
- Operations: process notifications, export aggregate reports, and preserve
  immutable audit trails for sensitive actions.

## Product Boundaries

- The product consumes external provider claims; local/demo auth helpers are not
  product login/logout flows.
- PostgreSQL is the final source of truth. Redis, projections, and queues must
  not authorize committed booking, ticket, check-in, or audit state.
- Phase 1 does not require microservices, Kafka, service mesh, cross-region HA,
  external payments, or managed cloud services.

## Design Principles

- Make operational truth visible: eligibility, capacity, ticket state, check-in
  result, and audit metadata appear where decisions happen.
- Preserve role clarity across employee, admin, check-in, and HR workflows.
- Use clear loading, success, warning, error, and denied states for every
  mutating action.
- Keep sensitive data quiet: signed tokens, provider tokens, and full PII stay
  redacted in ordinary UI, logs, and API activity.
- Target WCAG AA with visible focus, keyboard operation, reduced motion support,
  and responsive layouts at 375px, 768px, 1024px, and 1440px.
