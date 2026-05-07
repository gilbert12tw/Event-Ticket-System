# Frontend Layout Hardening

## Summary

Phase 1 frontend layout must read as a calm internal operations console across employee, activity admin, check-in, HR, and system admin workflows. The hardening work preserves the current visual identity while fixing crowding, inconsistent action spacing, and page proportions.

## Acceptance Criteria

- Required viewport projects, 375, 768, 1024, and 1440, have no horizontal document overflow.
- Workspace context bands keep text, identity controls, filters, and KPI groups inside their panels without child leakage.
- Toolbars, row actions, status selectors, form actions, and section actions wrap predictably without clipped buttons or overlapping text.
- API Activity is available for local/demo debugging but does not reserve a desktop rail by default. The main workspace gets priority below wide desktop.
- Event cards, ticket panels, check-in forms, reports, audit, notifications, registrations, and HR settings use stable proportions and readable density.
- Tables may scroll only inside `.table-scroll`; surrounding pages must remain horizontally contained.

## Non-Goals

- No backend API or data contract changes.
- No brand redesign, palette change, new navigation model, or marketing-style composition.
- No change to Phase 1 role scope, auth behavior, ticketing correctness, or audit semantics.

## Verification

- `pnpm --filter cets-web lint`
- `pnpm --filter cets-web test`
- `pnpm --filter cets-web build`
- `CI=true pnpm --filter cets-web test:e2e:mock`
- Representative screenshot review at 375, 768, 1024, and 1440 for user and admin routes.
