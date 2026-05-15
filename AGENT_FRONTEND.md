# Agent Task: Frontend — Eligibility Warning UI, Contract Alignment & Frontend Hardening

**Linear Issue:** `COR-19 / COR-20` (include in PR title and body)  
**Scope:** `apps/web/src/` · `apps/web/e2e/` · `apps/web/e2e-live/` · `docs/openapi/`  
**Primary validation command:** confirm exact package scripts in `apps/web/package.json`, then run the equivalent of:

```bash
pnpm --filter web lint
pnpm --filter web test -- --run
pnpm --filter web e2e
pnpm --filter web build
```

If the workspace uses root-level Turbo scripts, prefer the project-standard command from `package.json` / `turbo.json`.

---

## Context

This system is an enterprise employee event-ticketing platform. The frontend is a Responsive Web + PWA interface for employees, event organizers, check-in staff, and system administrators.

The backend change for `COR-19 / COR-20` moves employee-facing eligibility checks from DB employee attributes to provider/HR claims carried in the authenticated request. It also introduces a typed `EligibilityDecision` response with non-blocking `warnings`, especially `cross_city`, when the employee's registered city differs from the event city.

The frontend task is to consume this typed eligibility contract safely and display advisory warnings in employee event flows without blocking booking unless `can_book=false`. The warning must improve clarity but must not create new security, performance, or maintainability risks.

---

## Product Goals

- Employees can understand whether they are eligible to register for an event.
- If the backend returns a `cross_city` warning, the UI shows a clear advisory message.
- A warning must not block booking by itself. Only `eligibility.can_book === false` or an explicit backend rejection blocks registration.
- Event list and event detail pages should avoid extra eligibility round-trips when the backend already embeds `eligibility` in `EventSummary`.
- The UI must remain fast during hot event windows and reliable under flaky network conditions.
- Frontend code must be readable, modular, typed, testable, and safe from avoidable client-side security issues.

---

## Existing Frontend Structure

Current relevant frontend layout:

```text
apps/web/src/
├── App.tsx
├── app/
│   ├── architecture-boundaries.test.ts
│   ├── routes.test.ts
│   └── routes.ts
├── components/
│   ├── layout/
│   ├── shared/
│   └── ui/
├── features/
│   ├── audit/
│   ├── auth/
│   ├── checkin/
│   ├── demo-runbook/
│   ├── events/
│   ├── hr-settings/
│   ├── notifications/
│   ├── registrations/
│   ├── reporting/
│   └── tickets/
├── lib/
│   ├── api/
│   │   ├── client.ts
│   │   ├── contracts.ts
│   │   └── index.ts
│   └── formatting/
└── test/
```

Architecture direction:

- `components/ui/`: reusable primitive UI components only.
- `components/shared/`: cross-feature reusable components with no business ownership.
- `features/*`: feature-level pages and domain-specific UI.
- `lib/api/`: API client, typed contracts, response parsing, and API boundary.
- `app/`: route definitions and architecture boundary tests.

---

## Non-Goals

- Do not re-implement eligibility logic in the frontend.
- Do not infer department/site/city eligibility client-side.
- Do not store full provider claims or employee profile attributes in browser storage.
- Do not add new global state management unless existing local state becomes clearly unmaintainable.
- Do not add a second eligibility fetch per event card if the event summary already contains `eligibility`.
- Do not change backend authorization, HR claims, SSO, or database behavior in this frontend task.
- Do not make `cross_city` a blocking condition.

---

## Architecture Constraints

### 1. Contract-first API boundary

All API response shapes must be typed in `apps/web/src/lib/api/contracts.ts`.

Add or update frontend contract types to match the backend JSON shape:

```ts
export type WarningCode = "cross_city" | string;

export interface EligibilityWarning {
  code: WarningCode;
  message: string;
  employee_city?: string;
  event_city?: string;
}

export interface NoShowCooldown {
  active: boolean;
  // Keep existing fields here if already defined by the backend contract.
  // Do not duplicate incompatible cooldown shapes.
}

export interface EligibilityDecision {
  event_id: string;
  eligible: boolean;
  can_book: boolean;
  reasons: string[];
  warnings: EligibilityWarning[];
  no_show_cooldown: NoShowCooldown;
}
```

If `EventSummary` already exists, update it to include:

```ts
eligibility?: EligibilityDecision;
```

Use `eligibility?` during migration only if the backend may still return old flat fields in local/dev data. New code should prefer `event.eligibility`.

### 2. No business logic leakage into generic UI

Do not put `cross_city` handling inside `components/ui/alert.tsx` or other primitives.

Preferred placement:

```text
apps/web/src/features/events/
├── employee-pages.tsx
├── employee-pages.test.tsx
└── eligibility-warning.tsx       # create if the page grows too large
```

If the warning is reused by registrations or tickets, move it to:

```text
apps/web/src/components/shared/eligibility-warning.tsx
```

Only move it after reuse is real, not speculative.

### 3. Feature modules must respect boundaries

- `features/events/*` may import from `components/ui/*`, `components/shared/*`, `lib/api/*`, and `lib/formatting/*`.
- `components/ui/*` must not import from `features/*` or `lib/api/*`.
- `lib/api/*` must not import React components.
- Keep architecture boundary tests updated in `apps/web/src/app/architecture-boundaries.test.ts`.

### 4. Security and privacy

- Never store raw employee city, department, site, or employee ID in `localStorage`, `sessionStorage`, IndexedDB, or service worker caches.
- Do not log full API payloads containing employee-specific attributes.
- Do not expose claims or eligibility reasons in URL query strings.
- Do not use `dangerouslySetInnerHTML` for backend messages.
- Render warning messages as plain text.
- Treat backend strings as untrusted display text.
- Do not show hidden admin-only information to employee routes.
- If analytics are added, log only event-level metadata such as `event_id`, route, action, and warning code. Do not log `employee_city`, `event_city`, department, site, or employee ID.

### 5. Performance and scalability

The frontend must support hot activity windows where many users refresh event list/detail pages at the same time.

Rules:

- Prefer server-provided `event.eligibility` over per-card eligibility requests.
- Avoid N+1 requests in event lists.
- Memoize expensive list transformations only where measurable.
- Split large route chunks with lazy route loading if route bundles become large.
- Keep QR/check-in scanner pages isolated from employee event list code.
- Use skeleton/empty/error states instead of blocking whole-page rendering.
- Use request cancellation or stale-response protection when users switch filters quickly.
- Use CDN-friendly static assets and avoid bundling unnecessary large dependencies.
- Keep PWA/offline caches scoped: cache static assets and offline ticket/check-in data only where explicitly required.

### 6. Reliability and failure handling

- API failures must show actionable user messages, not raw stack traces.
- Eligibility fetch or event detail failure should not crash the app.
- If eligibility is missing from an event summary during migration, display a neutral fallback and require the booking endpoint to be the source of truth.
- Registration submit must handle:
  - already registered / idempotent replay,
  - sold out,
  - ineligible,
  - no-show cooldown,
  - network timeout,
  - unknown server error.
- Check-in PWA must preserve existing offline boundary tests and not accidentally import online-only APIs into offline logic.

---

## Step-by-Step Implementation

### Step 1 — Confirm frontend package scripts

Open:

```bash
cat apps/web/package.json
cat package.json
cat turbo.json
```

Record the exact commands for:

- lint
- typecheck
- unit tests
- e2e tests
- build

Do not invent scripts in CI or documentation. If scripts are missing, add minimal scripts using existing project tooling rather than introducing a new framework.

---

### Step 2 — Update API contracts

**File:** `apps/web/src/lib/api/contracts.ts`

1. Add `EligibilityWarning`, `EligibilityDecision`, and `WarningCode`.
2. Update `EventSummary` / event detail response types to include `eligibility`.
3. Keep deprecated flat fields only if current mock data still uses them:

```ts
eligible?: boolean;
eligibility_reason?: string;
```

4. Add a small adapter only if required for migration:

```ts
export function getEligibilityDecision(event: EventSummary): EligibilityDecision | undefined {
  if (event.eligibility) return event.eligibility;

  // Temporary compatibility shim. Remove after backend always returns eligibility.
  if (typeof event.eligible === "boolean") {
    return {
      event_id: event.event_id,
      eligible: event.eligible,
      can_book: event.eligible,
      reasons: event.eligibility_reason ? [event.eligibility_reason] : [],
      warnings: [],
      no_show_cooldown: { active: false },
    };
  }

  return undefined;
}
```

Keep this adapter in `lib/api/contracts.ts` or a small `features/events/eligibility.ts` helper. Do not spread fallback logic across page components.

---

### Step 3 — Add a focused eligibility warning component

Create one of the following depending on reuse:

```text
apps/web/src/features/events/eligibility-warning.tsx
```

Suggested component behavior:

```tsx
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import type { EligibilityWarning } from "@/lib/api/contracts";

interface EligibilityWarningListProps {
  warnings: EligibilityWarning[];
}

export function EligibilityWarningList({ warnings }: EligibilityWarningListProps) {
  if (warnings.length === 0) return null;

  return (
    <div aria-label="Eligibility warnings" className="space-y-2">
      {warnings.map((warning, index) => (
        <Alert key={`${warning.code}-${index}`} role="status">
          <AlertTitle>{warning.code === "cross_city" ? "Cross-city event notice" : "Eligibility notice"}</AlertTitle>
          <AlertDescription>{warning.message}</AlertDescription>
        </Alert>
      ))}
    </div>
  );
}
```

Requirements:

- Use existing UI primitives.
- Render backend messages as plain text.
- Use `role="status"` or equivalent accessible semantics.
- Do not make warning visually identical to hard errors.
- Do not disable booking only because `warnings.length > 0`.

---

### Step 4 — Update employee event list and detail pages

**Files:**

```text
apps/web/src/features/events/employee-pages.tsx
apps/web/src/features/events/employee-pages.test.tsx
```

Implementation rules:

- Show `EligibilityWarningList` on event detail when `event.eligibility.warnings.length > 0`.
- On event cards, show a compact badge only if it does not clutter the list. Prefer detail-page warning if card space is limited.
- Registration CTA disabled state must be derived from `eligibility.can_book === false`, not from warning presence.
- If `eligibility.eligible === false`, show reasons from `eligibility.reasons`.
- If `eligibility.no_show_cooldown.active === true`, show a clear cooldown message and disable booking.
- If `eligibility` is missing, do not assume eligibility. Show neutral copy such as: `Eligibility will be verified when you register.`

Pseudo-decision table:

| Backend response | UI behavior |
|---|---|
| `eligible=true`, `can_book=true`, `warnings=[]` | Enable Register |
| `eligible=true`, `can_book=true`, `warnings=[cross_city]` | Enable Register + show advisory warning |
| `eligible=false`, `can_book=false`, `reasons=[...]` | Disable Register + show reason |
| `no_show_cooldown.active=true` | Disable Register + show cooldown |
| `eligibility` missing | Do not crash; show neutral fallback; booking endpoint remains source of truth |

---

### Step 5 — Keep registration submit authoritative

**Files:**

```text
apps/web/src/features/registrations/pages.tsx
apps/web/src/lib/api/client.ts
apps/web/src/api.test.ts
```

Rules:

- The event list/detail UI is advisory. The booking API response is authoritative.
- Generate and send an idempotency key for booking if the existing API requires it.
- Prevent double-submit with local pending state, but do not rely on local state for correctness.
- Handle 409 / sold-out / duplicate booking responses gracefully.
- Handle 403 / ineligible responses by showing backend-provided safe reason text.
- Do not retry non-idempotent booking submits unless an idempotency key is present and the API contract allows retry.

---

### Step 6 — Preserve PWA/offline ticket and check-in behavior

**Files:**

```text
apps/web/src/features/tickets/pages.tsx
apps/web/src/features/tickets/qr.tsx
apps/web/src/features/tickets/ticket-panel.test.tsx
apps/web/src/features/checkin/pages.tsx
apps/web/src/features/checkin/offline-boundary.test.tsx
apps/web/src/features/checkin/checkin-token.ts
```

Rules:

- Do not cache employee eligibility claims in offline storage.
- Offline ticket display may cache ticket QR/token data only according to existing ticket design.
- Offline check-in must not depend on employee event-list eligibility UI.
- Maintain separation between check-in token verification logic and display pages.
- Keep `offline-boundary.test.ts` passing; add assertions if new imports risk pulling online APIs into offline code.

---

### Step 7 — Update OpenAPI-driven frontend docs or schema references

Relevant schema files:

```text
docs/openapi/components/schemas/common.yaml
docs/openapi/components/schemas/eligibility.yaml
docs/openapi/components/schemas/events.yaml
docs/openapi/paths/eligibility.yaml
docs/openapi/paths/employee-events.yaml
```

Checklist:

- `EligibilityDecision` appears in the schema.
- `EligibilityWarning.code` includes `cross_city`.
- `EventSummary` includes `eligibility`.
- Deprecated flat eligibility fields, if retained, are marked deprecated.
- Frontend TypeScript contracts match OpenAPI names and JSON casing.

Do not change API names only on the frontend. If a contract mismatch exists, update OpenAPI/backend or add a clearly temporary adapter.

---

## Testing Plan

### Unit tests

Add or update tests near the code being changed:

```text
apps/web/src/features/events/employee-pages.test.tsx
apps/web/src/features/events/eligibility-warning.test.tsx
apps/web/src/api.test.ts
apps/web/src/components/shared/shared.test.tsx
apps/web/src/app/architecture-boundaries.test.ts
```

Required test cases:

| Test | Expected |
|---|---|
| renders cross-city warning | warning message visible; register button still enabled when `can_book=true` |
| warning does not block booking | `warnings.length > 0` alone does not disable CTA |
| ineligible reason blocks booking | `eligible=false` / `can_book=false` disables CTA and displays reason |
| cooldown blocks booking | cooldown active disables CTA and displays cooldown state |
| missing eligibility fallback | page does not crash and shows neutral fallback |
| API contract parses warnings | typed contract accepts `warnings: [{ code: "cross_city" }]` |
| no raw PII logging | tests or lint guard prevent direct `console.log(payload)` in touched files |
| architecture boundary remains valid | UI primitives do not import feature/api modules |

Use React Testing Library style assertions focused on user-visible behavior.

---

### E2E tests

Update or add:

```text
apps/web/e2e/core-role-routes.spec.ts
apps/web/e2e-live/phase1-live-flow.spec.ts
```

Recommended flows:

1. Employee opens event detail with `cross_city` warning.
2. Warning is visible.
3. Register button remains enabled.
4. Submit registration.
5. Final state is success / received / waitlisted depending on fixture.
6. Employee opens ineligible event.
7. Register button is disabled or submit returns safe rejection.
8. Check-in route still loads and offline boundary still works.

---

### Accessibility tests

Minimum requirements:

- Warning content is announced through semantic status/alert region.
- Buttons have accessible names.
- Error and warning colors are not the only signal.
- Keyboard navigation can reach registration CTA and dismissible dialogs.
- QR/ticket pages preserve readable contrast and responsive layout.

---

### Performance tests

Frontend checks before merge:

```bash
pnpm --filter web build
```

Then inspect bundle output if available.

Manual or automated checks:

- Event list does not issue one eligibility API request per event card.
- Event detail page avoids duplicate eligibility fetch on first render if data is already loaded.
- Registration CTA state changes do not re-render the whole app tree.
- Largest route chunks are justified.
- Static assets remain CDN-cacheable.
- No unnecessary large dependency is introduced.

For live or staging gates, use existing `k6/phase1-release-gate.js` and `k6/phase1-production-gate.js` from the repository when validating end-to-end API and user-flow performance.

---

## Observability and Operations Checklist

Before opening the PR:

- [ ] Frontend errors are caught by route/page error boundaries or safe local error states.
- [ ] User-facing errors are safe and do not expose raw backend internals.
- [ ] No raw employee ID, city, department, site, bearer token, ticket token, or full API payload is logged.
- [ ] Network failures produce actionable copy.
- [ ] Booking double-click is prevented locally.
- [ ] Booking correctness still relies on backend idempotency and DB transaction, not UI state.
- [ ] PWA/offline ticket and check-in behavior remains covered by tests.
- [ ] Event list avoids N+1 eligibility requests.
- [ ] `cross_city` warning display is covered by unit and E2E tests.
- [ ] `pnpm-lock.yaml` changes are intentional and minimal.
- [ ] PR body explains user-visible behavior, testing performed, and any migration shims.

---

## Code Quality Checklist

- [ ] Components are small and named by responsibility.
- [ ] Domain-specific code stays inside `features/events` or a justified shared component.
- [ ] Generic UI primitives remain business-agnostic.
- [ ] API types are centralized in `lib/api/contracts.ts`.
- [ ] Temporary compatibility shims are marked with removal criteria.
- [ ] No duplicated eligibility decision logic across pages.
- [ ] No `any` unless there is a documented boundary reason.
- [ ] No broad `try/catch` that hides errors without user feedback.
- [ ] No hardcoded mock-only behavior in production paths.
- [ ] Tests assert behavior, not implementation details.
- [ ] All changed files are formatted consistently.

---

## Security Review Checklist

- [ ] Backend-provided warning messages are rendered as text, never HTML.
- [ ] No `dangerouslySetInnerHTML`.
- [ ] Tokens are never printed or persisted outside approved storage.
- [ ] Eligibility claims are not stored client-side.
- [ ] Admin routes remain RBAC-protected in UI and still rely on backend authorization.
- [ ] File upload UI, if touched, still validates size/type and does not trust extension alone.
- [ ] QR token UI does not expose token internals beyond required QR rendering.
- [ ] CSRF/idempotency behavior for booking remains aligned with API contract.
- [ ] No new dependency with known security concern is added without justification.

---

## PR Template

Use this structure in the PR body:

```md
## Summary
- Added typed frontend support for EligibilityDecision warnings.
- Display cross-city advisory warning without blocking registration.
- Preserved booking as backend-authoritative source of truth.

## User-visible behavior
- Employees see a notice when an event is in a different city.
- Registration remains available when `can_book=true`.
- Ineligible and cooldown states still block booking with safe explanations.

## Architecture / maintainability
- API contracts centralized in `lib/api/contracts.ts`.
- Warning UI isolated in events feature component.
- No business logic added to generic UI primitives.

## Security / privacy
- No eligibility claims persisted client-side.
- Backend messages rendered as text.
- No raw employee identifiers or tokens logged.

## Tests
- [ ] Unit tests
- [ ] API contract tests
- [ ] E2E route flow
- [ ] Offline/check-in boundary tests
- [ ] Build

## Commands run
```bash
pnpm --filter web lint
pnpm --filter web test -- --run
pnpm --filter web e2e
pnpm --filter web build
```

## Risks / follow-up
- Remove temporary flat eligibility field shim after backend rollout is complete.
```

---

## Definition of Done

This task is done only when:

1. Frontend contracts match backend `EligibilityDecision`.
2. Employee event UI displays `cross_city` warnings.
3. Warning presence never blocks booking by itself.
4. Ineligible and cooldown states still block correctly.
5. No extra per-event eligibility request is introduced in lists.
6. Unit and E2E tests cover warning and blocking behavior.
7. Offline ticket/check-in behavior still passes existing tests.
8. No raw PII, tokens, or full claim payloads are logged or persisted.
9. Build, lint, typecheck, and tests pass.
10. PR body includes validation commands and explains maintainability/security decisions.
