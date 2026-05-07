# Product

## Register

product

## Users

Corporate employees use the system to find eligible internal events, book available seats, join waitlists, and present electronic tickets at the venue. Activity admins configure events, capacity, booking windows, and eligibility rules. Check-in staff redeem signed ticket tokens during live entry. HR and system admins inspect participation reports and audit records without unnecessary personal data exposure.

The primary usage context is an internal operations workflow on office laptops, with check-in staff also using tablets or laptops at an event entrance. Users need clear status, predictable controls, and fast confidence in whether an action succeeded.

## Product Purpose

The Corporate Event Ticketing System demonstrates the Phase 1 ticketing flow for a modular monolith: event creation, eligibility, booking, waitlist, signed tickets, one-time check-in, reporting, and audit. Success means each role can complete its workflow without manual database edits, while the UI makes correctness boundaries visible: eligibility rechecks, capacity, token redaction, duplicate scan protection, and auditability.

## Brand Personality

Precise, calm, trustworthy.

The interface should feel like a mature internal operations product: focused, dense enough for repeated work, and visually controlled. It should earn confidence through hierarchy and behavior, not decoration.

## Anti-references

Avoid pale grey card piles where every surface has the same weight and the user's next action is unclear. Avoid marketing-page composition, oversized slogans, decorative hero art, and feature-explainer blocks. Avoid dark neon command-center styling, gratuitous animation, glassmorphism, and visual effects that make an internal tool feel less credible.

## Design Principles

- Make operational truth visible: show eligibility, capacity, ticket state, check-in result, and audit metadata where decisions happen.
- Preserve role clarity: employee, activity admin, check-in staff, and HR surfaces share one product system while keeping their workflows distinct.
- Prioritize task confidence: every mutating action needs a clear disabled, loading, success, warning, or error state.
- Keep sensitive data quiet: signed tokens and session details stay redacted in ordinary UI and API activity.
- Use density with discipline: tables, forms, and runbooks should scan quickly without collapsing into visual sameness.

## Accessibility & Inclusion

Target WCAG AA for contrast, focus visibility, keyboard operation, reduced motion support, and responsive layouts. Verify 375px, 768px, 1024px, and 1440px viewports. Color must always be paired with readable text labels for eligibility, booking, ticket, warning, error, duplicate scan, and audit states.
