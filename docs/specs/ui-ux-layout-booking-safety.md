# Feature: UI/UX Layout and Booking Safety Cleanup

## Summary

This change hardens the employee booking flow and role workspaces. The UI must stay visually balanced across primary role routes, employee self-cancellation must require explicit confirmation, and repeated booking attempts must be shown as existing booking state rather than fresh success.

## Acceptance Criteria

- [ ] AC-1: Given any primary role route is opened at 375, 768, 1024, or 1440px, when the page finishes loading, then the workspace has no horizontal overflow, no clipped visible button text, and no toolbar children escaping their container.
- [ ] AC-2: Given `/admin/checkin/offline` renders before an offline package is downloaded, when the first step is visible, then the package form and preview use balanced columns and the empty preview does not create a large unused right rail.
- [ ] AC-3: Given `/admin/notifications` renders with collapsed API activity, when the delivery table is visible, then the collapsed `介接紀錄` panel does not cover table rows, retry controls, or status filters.
- [ ] AC-4: Given an employee clicks `取消報名`, when the registration is cancellable, then a confirmation dialog appears and the cancellation API is not called until `確認取消報名` is chosen.
- [ ] AC-5: Given a cancellation confirmation dialog is visible, when it renders, then it names the event, current registration state, selected reason, ticket invalidation consequence, capacity/waitlist impact, and rebooking caveat.
- [ ] AC-6: Given the employee dismisses the cancellation dialog, when the dialog closes, then no cancellation request is sent and focus returns to the cancellation control.
- [ ] AC-7: Given a booking request resolves to an existing registration for the same employee and event, when the API responds, then `duplicate: true` is present and no new registration is created.
- [ ] AC-8: Given the frontend receives a duplicate confirmed booking with a ticket, when the result renders, then it says the user already booked and links to the exact ticket detail URL.
- [ ] AC-9: Given the frontend receives a duplicate waitlist response, when the result renders, then it says the user is already waitlisted and sends the user to notifications or registered events.
- [ ] AC-10: Given event detail already has `confirmed`, `waitlisted`, or a current ticket, when the primary booking control would otherwise submit, then the frontend does not POST and shows the existing state instead.
- [ ] AC-11: Given an employee event action is unavailable, when the event appears in a list or detail rail, then the UI shows a disabled blocker button with the unavailable state label instead of hiding the action or showing a primary-looking CTA.

## Interface Changes

`BookingResponse` gains one optional field:

```json
{
  "duplicate": true
}
```

`duplicate` is `true` only when the request returns an existing registration for the same employee/event, including idempotency replay, stale UI retry, duplicate click, or another tab that already booked. New successful bookings omit the field.

No backend route, database schema, environment variable, dependency, or RouteKey changes are required.

## Edge Cases

| Scenario                                                                 | Expected Behavior                                                                        |
| ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------- |
| User cancels then closes dialog                                          | Preserve registration; do not call cancel API.                                           |
| User confirms cancellation while request is pending                      | Disable confirm and show pending label.                                                  |
| Ticket exists during cancellation                                        | Dialog and result mention that issued ticket becomes invalid.                            |
| Booking detail is stale but API returns existing confirmed registration  | Show already-booked copy and ticket handoff when available.                              |
| Booking detail is stale but API returns existing waitlist registration   | Show already-waitlisted copy and notification/registered handoff.                        |
| Idempotency key belongs to different request                             | Show localized recoverable error: refresh event detail before retrying.                  |
| Collapsed API activity is visible                                        | It must not overlap interactive controls or table rows.                                  |
| Event is ineligible, closed, already booked, waitlisted, or not yet open | Render a disabled blocker button with the state label and keep explanatory copy visible. |

## Non-Functional Requirements

- Accessibility: Confirmation dialog has title, description, cancel and confirm buttons, keyboard dismissal, focus restore, and text consequences that do not rely on color.
- Security: Ticket tokens remain redacted; duplicate handling must not expose raw idempotency keys beyond existing API activity redaction.
- Correctness: PostgreSQL remains the booking source of truth; duplicate handling is a response signal only and does not replace server-side uniqueness or transactions.
- Responsive layout: All primary role routes stay contained at 375, 768, 1024, and 1440px.
- 12-Factor: No config, backing service, dependency, logging, or process model changes.

## Verification Plan

- Unit tests for cancellation confirmation open/dismiss/confirm, duplicate booking result copy, and stale detail pre-submit guard.
- Go tests for repeated booking and idempotency replay returning `duplicate: true` without creating extra registrations.
- E2E checks for role routes, no overflow, no clipped buttons, no toolbar leaks, no API-panel overlap, cancellation confirmation, and duplicate booking response.
- Commands: `pnpm --filter cets-web test`, `pnpm --filter cets-web type-check`, `pnpm --filter cets-web lint`, `pnpm --filter cets-web exec playwright test e2e/core-role-routes.spec.ts`, `go test ./services/api/...`, and `git diff --check`.
