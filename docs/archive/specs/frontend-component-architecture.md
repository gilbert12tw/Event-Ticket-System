# Feature: Frontend Component Architecture

## Summary

Refactor the `apps/web` Vite React SPA into reusable component layers while preserving the existing Corporate Event Ticketing System Phase 1 browser flows. The change keeps reuse scoped to `apps/web`, initializes shadcn/ui as the local primitive component source, and separates app composition, layout, shared product components, feature modules, API contracts, and formatting helpers.

## Acceptance Criteria

- [ ] AC-1: Given the frontend source tree, When a developer looks for reusable primitives, Then shadcn-generated files live only under `apps/web/src/components/ui`.
- [ ] AC-2: Given a cross-feature product component, When it is imported by a feature page, Then it comes from `apps/web/src/components/shared` or `apps/web/src/components/layout` and does not import from `features/*`.
- [ ] AC-3: Given a feature page, When it calls the backend, Then API calls are imported from `apps/web/src/lib/api` and typed with shared contracts.
- [ ] AC-4: Given existing routes such as `/user/events`, `/admin/demo`, `/admin/checkin`, `/admin/reports`, and `/admin/audit`, When the app builds and runs, Then route selection, role guards, and legacy aliases remain compatible with the current Phase 1 demo.
- [ ] AC-5: Given the React app build, When `pnpm --filter cets-web build` runs, Then Vite still outputs static assets to `services/api/internal/httpapi/static`.
- [ ] AC-6: Given component-level regression tests, When `pnpm --filter cets-web test` runs, Then shared components, route guards, status mapping, ticket display, check-in result rendering, audit filters, and empty/loading states have focused coverage.

## Edge Cases

| # | Scenario | Expected Behavior |
|---|----------|-------------------|
| E-1 | A business component is useful across two features | Move it to `components/shared` and keep it free of feature imports. |
| E-2 | A feature component needs module-specific API state | Keep it under `features/<module>` and import API helpers from `lib/api`. |
| E-3 | A shadcn component needs local adjustment | Edit the generated file in `components/ui` only for primitive behavior or styling consistency, never for ticketing business rules. |
| E-4 | Route is unavailable for the current role | Route guard returns the current role's default route without changing backend state. |
| E-5 | API activity payload contains signed ticket tokens or sessions | API observer redacts sensitive values before rendering logs. |
| E-6 | Test environment renders browser-only components | Components must render under jsdom without relying on real network, camera, or local backing services. |

## Non-Functional Requirements

| Category | Requirement | Metric |
|----------|-------------|--------|
| Reuse | Shared components expose stable props and avoid hidden feature coupling. | No `components/shared` import from `features/*`. |
| Build | The refactor must not change the static output contract used by the Go app. | `apps/web/vite.config.ts` keeps `outDir` pointing at `services/api/internal/httpapi/static`. |
| Testability | Feature pages delegate reusable UI to components that can be rendered in isolation. | New component tests run through Vitest + React Testing Library. |
| Accessibility | shadcn and layout components preserve accessible labels, headings, and status regions. | Tests assert critical labels/roles where practical. |
| Security | UI logs and ticket displays do not expose signed token values except in the explicit check-in handoff control. | API log tests keep redaction coverage. |
| Maintainability | `App.tsx` becomes app composition and route selection, not a component warehouse. | Route metadata, guards, helpers, and feature pages move into modules. |

## Minimal Interface Contract

No backend API contract changes are introduced. Frontend module contracts are:

```text
apps/web/src/lib/api
  API functions, ApiError, ApiObserver, employees demo constants

apps/web/src/app
  route metadata, route guards, App component composition

apps/web/src/components/ui
  shadcn-generated UI primitives only

apps/web/src/components/shared
  business-neutral reusable product components

apps/web/src/features/<module>
  feature pages, module widgets, hooks, and local helpers
```

## 12-Factor Compliance Notes

- Codebase: Reuse stays in the existing monorepo and `apps/web`; `packages/ui` is deferred until another frontend app exists.
- Dependencies: New React testing and shadcn/Tailwind dependencies are declared in `apps/web/package.json` and the root lockfile.
- Config: No deploy-specific frontend values are hardcoded; the Vite dev proxy remains local development configuration.
- Backing Services: Browser tests mock network calls and do not introduce local in-memory substitutes for production data.
- Build / Release / Run: Vite build output remains an immutable static artifact served by the Go app.
- Processes: Browser state remains client-local only; registrations, tickets, check-ins, reports, and audit logs stay in PostgreSQL through backend APIs.
- Logs: API activity UI uses redacted payloads and does not write local log files.

## Test Mapping

- AC-1 and AC-2: Static import boundary tests for shared/layout components.
- AC-3 and E-5: Existing and updated API client tests.
- AC-4: Route guard tests for employee, activity admin, check-in staff, and HR admin routes.
- AC-5: `pnpm --filter cets-web build`.
- AC-6: React Testing Library tests for shared components and key feature widgets.
