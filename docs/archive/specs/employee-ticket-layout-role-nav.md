# Feature: Employee Ticket Layout and Role Navigation Cleanup

## Summary

This change makes employee ticket access safer and clearer by replacing the two-column ticket workspace with a single-column ticket list and an explicit ticket detail URL. It also removes duplicated event/ticket proof from the employee event detail action rail and hides workspace switch options that the current role cannot access.

## Acceptance Criteria

- [ ] AC-1: Given an employee opens `/user/tickets`, when tickets load, then the page shows a single-column ticket list and no QR code is displayed until a ticket is selected.
- [ ] AC-2: Given an employee clicks a ticket row, when navigation completes, then `/user/tickets?ticket_id=<ticket_id>` opens a detail view for that exact ticket.
- [ ] AC-3: Given a ticket detail URL references a missing or forbidden ticket, when the API returns an error, then the page shows a recoverable error and does not fall back to another ticket.
- [ ] AC-4: Given an active ticket has no QR payload or signed token, when its detail view renders, then the UI shows a QR-unavailable recovery message instead of implying entry is ready.
- [ ] AC-5: Given booking success or an existing event ticket handoff includes a ticket ID, when the user chooses to view the ticket, then the app opens the exact ticket detail URL.
- [ ] AC-6: Given an employee views event detail, when the action rail renders, then it keeps booking, waitlist, cancellation, family-count, and result controls without repeating the full left-side event proof or embedding a ticket QR.
- [ ] AC-7: Given a role can access only one workspace, when the sidebar renders, then inaccessible workspace options are not visible.
- [ ] AC-8: Given ticket summary counts render, when tickets include active, QR-pending, redeemed, revoked, or unknown states, then the counts and badges use distinct employee-facing labels instead of treating every non-active ticket as redeemed.
- [ ] AC-9: Given a ticket detail API response returns a different ticket ID than the requested query ID, when the page renders, then it shows a recoverable error and does not show a QR or fallback ticket.
- [ ] AC-10: Given an employee can book or join waitlist from event browsing, when they choose the primary action, then the app opens event detail for final confirmation instead of submitting from the list row.
- [ ] AC-11: Given an employee cancels a confirmed or waitlisted registration, when cancellation succeeds, then the result copy states that the registration is cancelled, an issued ticket is invalidated, capacity or waitlist may update, and rebooking depends on current eligibility and capacity.
- [ ] AC-12: Given keyboard users use the shell and ticket detail, when they activate skip links or open detail via SPA navigation, then focus lands on the intended main workspace or ticket detail heading.

## Edge Cases

| #   | Scenario                                     | Expected Behavior                                                                   |
| --- | -------------------------------------------- | ----------------------------------------------------------------------------------- |
| E-1 | No tickets returned                          | Show an empty state with a path back to event discovery.                            |
| E-2 | Ticket detail API fails                      | Show the error with a return-to-list action; do not select the first ticket.        |
| E-3 | Ticket is redeemed or revoked                | Show status and unavailable copy without a QR.                                      |
| E-4 | Active ticket has no QR data                 | Show `二維碼尚未產生` recovery copy without raw token text.                         |
| E-5 | Current role cannot access another workspace | Do not render that workspace option in desktop or mobile navigation.                |
| E-6 | Ticket API returns another ticket ID         | Treat as an invalid detail response and do not render ticket content.               |
| E-7 | Booking would join waitlist                  | Show waitlist policy, hidden-position copy, notification path, and deadline policy. |
| E-8 | User cancels a registration                  | Preserve a clear cancellation result and refresh event state.                       |

## Role Route Matrix

| Role           | Accessible routes                                                                                              |
| -------------- | -------------------------------------------------------------------------------------------------------------- |
| employee       | `/user/events`, `/user/events/detail`, `/user/tickets`, `/user/notifications`                                  |
| activity_admin | `/admin/events`, `/admin/registrations`, `/admin/notifications`, `/admin/flow-check` when mock demo is enabled |
| checkin_staff  | `/admin/checkin`, `/admin/checkin/offline`                                                                     |
| hr_admin       | `/admin/reports`, `/admin/hr-settings`, `/admin/audit`                                                         |
| system_admin   | `/admin/reports`, `/admin/hr-settings`, `/admin/audit`, `/admin/notifications`                                 |

Roles with only one accessible workspace do not see the workspace switch. Forbidden deep links must show an explicit recoverable unauthorized state and route filtering must keep inaccessible pages out of desktop and mobile navigation.

## Ticket Status Vocabulary

| Ticket state                              | List badge  | Detail behavior                                                                                                  |
| ----------------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------------------- |
| active with QR payload or signed token    | `可入場`    | Show QR inside the same one-column ticket detail flow, one entry instruction, and redacted-token copy only once. |
| active without QR payload or signed token | `待產生 QR` | No QR; show retry refresh and organizer/support recovery copy.                                                   |
| redeemed                                  | `已核銷`    | No QR; show one non-entry message.                                                                               |
| revoked                                   | `已撤銷`    | No QR; show one non-entry message and revoked reason when available.                                             |
| unknown non-active state                  | `不可入場`  | No QR; show a generic support recovery message.                                                                  |

Ticket IDs may be visually shortened in list rows, but detail URLs and API calls always use the full ID.

## Non-Functional Requirements

| Category          | Requirement                                                                 | Metric                                                  |
| ----------------- | --------------------------------------------------------------------------- | ------------------------------------------------------- |
| Accessibility     | Ticket rows use link semantics, visible focus, and stable accessible names. | Keyboard can open ticket detail and return to list.     |
| Security          | Raw `signed_token` and `qr_payload` remain hidden from ordinary text.       | Unit tests assert token strings are not visible.        |
| Responsive Layout | Ticket list/detail and event detail stay contained across target widths.    | No horizontal overflow at 375, 768, 1024, and 1440px.   |
| Compatibility     | No backend, schema, config, or dependency changes.                          | Existing ticket APIs are reused.                        |
| Keyboard Access   | SPA navigation and skip links move focus to useful landmarks.               | Unit/E2E checks cover link semantics and focus targets. |

## Minimal UI Contract

```text
GET /api/v1/me/tickets
GET /api/v1/tickets/{ticket_id}

List URL:
/user/tickets

Detail URL:
/user/tickets?ticket_id=<ticket_id>
```

Ticket list rows display event title, event time or issue time, location when available, ticket status, entry readiness, and a safe ticket ID. Ticket detail uses one content column: status, title, entry state, QR or unavailable state, holder, location, start time, ticket ID, issued time, optional expiry or revocation reason, and link-style actions to return to the list or open event detail. The UI treats mismatched ticket IDs from the detail API as an error.

## 12-Factor Compliance Notes

- Config: No new environment configuration.
- Backing Services: No new backing service dependency.
- Logs: No new frontend logging layer.
- Processes: No local persistent state; ticket detail state is URL-derived.
- Build/Release/Run: Existing Vite build and static serving model remain unchanged.
