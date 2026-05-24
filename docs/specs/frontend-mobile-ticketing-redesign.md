# Feature: Frontend Mobile Ticketing Redesign

## Summary

Redesign the `apps/web` React shell and core ticketing pages so desktop remains a dense internal operations console while phones and small tablets use a task-native mobile layout. The change adds all-role mobile bottom navigation, a compact mobile top bar, a Debug utility sheet, and mobile-first layouts for event discovery, booking and waitlist states, tickets, notifications, check-in, reporting, audit, and admin event governance. No backend API, deployment, config, or backing-service contract changes are introduced.

## Acceptance Criteria

- [ ] AC-1: Given an authenticated user at 375px or 768px, When any allowed role route is loaded, Then the desktop sidebar is hidden, a mobile top bar and bottom tab bar are visible, the active route is marked, and the main content is not covered by fixed navigation.
- [ ] AC-2: Given an authenticated user at 1024px or 1440px, When any allowed role route is loaded, Then the persistent desktop sidebar is visible, mobile navigation is hidden, and the workspace keeps a dense operations layout.
- [ ] AC-3: Given any tested viewport of 375, 768, 1024, or 1440, When employee, admin, check-in, HR, and audit routes render, Then the document has no horizontal overflow and visible buttons do not clip text.
- [ ] AC-4: Given a role with more mobile routes than fit in the bottom bar, When the user taps the More control, Then a route sheet opens with the remaining accessible routes and uses ordinary links that preserve URL state.
- [ ] AC-5: Given Debug mode is available but disabled, When a user opens mobile utilities, Then Debug can be enabled without showing API activity in the normal page chrome.
- [ ] AC-6: Given Debug mode is enabled, When a user opens mobile utilities, Then identity switching, service status, and redacted API activity are available in a sheet without reserving workspace columns.
- [ ] AC-7: Given an employee opens My Tickets on mobile, When at least one ticket exists, Then the selected ticket detail, QR or unavailable state, event time, location, attendee, and check-in instruction appear before the ticket list.
- [ ] AC-8: Given HR opens Reports on mobile, When participation data exists, Then aggregate summary cards and export actions appear before detailed data, and detailed rows are readable as mobile cards or contained table overflow.
- [ ] AC-9: Given check-in staff opens Check-in on mobile, When a token is submitted, Then scan input and result states are prioritized above supporting context and duplicate, invalid, revoked, and success outcomes are text-labeled.
- [ ] AC-10: Given activity admins configure events, When required operational fields, eligibility impact, capacity, booking window, ticket readiness, or notification state are incomplete, Then readiness warnings remain visible before publish or sensitive changes.

## Edge Cases

| #   | Scenario                                                   | Expected Behavior                                                                                                   |
| --- | ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| E-1 | Employee is ineligible after list eligibility was cached   | Final booking remains rejected by backend; UI names the eligibility reason and recovery path.                       |
| E-2 | Capacity sells out during submit                           | Booking fails or offers waitlist according to policy; input state is preserved and no duplicate booking is created. |
| E-3 | Duplicate booking retry                                    | Existing idempotent outcome is shown and the UI does not imply another seat was consumed.                           |
| E-4 | Ticket is active, redeemed, revoked, or missing QR payload | Ticket detail shows a distinct status, hides raw signed token, and explains entry eligibility.                      |
| E-5 | Duplicate or invalid check-in token                        | Duplicate shows first redemption context when available; invalid remains visually and textually distinct.           |
| E-6 | HR data is stale or sync review is open                    | HR settings and affected workflows show review state before users rely on eligibility or reports.                   |
| E-7 | Notification delivery fails                                | Admin notification views show retry/failure state and do not hide booking or ticket truth.                          |
| E-8 | API request fails                                          | Alert or inline failure state is shown with retry path; mobile fixed navigation remains usable.                     |
| E-9 | Debug mode is off                                          | API activity panel is absent from page layout and no test expects it to reserve columns.                            |

## Non-Functional Requirements

| Category          | Requirement                                                            | Metric                                                                      |
| ----------------- | ---------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Responsive Layout | Tested desktop and mobile layouts have no document overflow            | 375, 768, 1024, 1440 all pass E2E overflow gate                             |
| Accessibility     | Mobile nav and sheet controls remain keyboard and screen-reader usable | Links/buttons are semantic, `aria-current` on active route, sheet has title |
| Touch Ergonomics  | Mobile primary navigation targets are tappable                         | Bottom tab and sheet controls are at least 44px tall                        |
| Security          | Ticket and API surfaces keep signed tokens redacted                    | No raw signed token in ordinary ticket UI or API activity                   |
| Observability     | Debug tools remain available without polluting formal UI               | API activity appears only when Debug is enabled                             |
| Build Contract    | Static frontend output remains served by the Go app                    | Vite `outDir` remains `services/api/internal/httpapi/static`                |

## Minimal Interface Contract

No backend API changes are introduced.

Frontend contracts:

```text
apps/web/src/app/routes.ts
  Route metadata adds mobileLabel, mobileOrder, mobilePrimary, and mobileOverflow.

apps/web/src/components/layout
  AuthenticatedShell composes desktop and mobile navigation from route metadata.
  ApiActivity accepts a display mode for floating desktop panel or mobile sheet.

apps/web/src/components/shared
  ResponsiveTable accepts optional mobileCards content while retaining table semantics.

apps/web/src/components/layout
  Mobile behavior stays in the shell and CSS breakpoint system; avoid adding a separate viewport hook
  unless JavaScript needs behavior that CSS cannot express.
```

## 12-Factor Compliance Notes

- Codebase: all changes stay in this repository under `apps/web` and `docs/specs`.
- Dependencies: no new dependency is planned; any future dependency must be declared in `apps/web/package.json` and the lockfile.
- Config: no new environment variables or hardcoded deploy values.
- Backing Services: no new backing services; PostgreSQL, Redis, MinIO, mail, HR, and SSO contracts are unchanged.
- Build / Release / Run: Vite build remains an immutable static artifact emitted to the existing Go static directory.
- Processes: frontend remains stateless; persisted ticketing truth stays in backend services.
- Port Binding: no frontend server contract changes.
- Concurrency: no singleton browser assumptions; retry-sensitive operations continue to rely on backend idempotency.
- Disposability: no long-running browser work or local persistent state is introduced.
- Dev / Prod Parity: verification uses the existing Vite, Vitest, and Playwright flows.
- Logs: API activity is UI-only, redacted, and does not write files.
- Admin Processes: no migrations or one-off admin jobs.
