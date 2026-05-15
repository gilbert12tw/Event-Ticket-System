# Stable Implementation Plan — P1-05 Backend + P1-06 Frontend Warning UX

## 0. Purpose

This plan implements the Phase 1 eligibility warning feature in **Linear-aligned, test-gated slices**:

1. `[Phase 1][P1-05] Backend eligibility claims and location warning policy`
2. `[Phase 1][P1-06] Employee event browse/detail warning UX`

The two issues should stay separate because their Non-Goals are different:

- **P1-05**: no frontend warning UI.
- **P1-06**: no backend eligibility changes.

The stable development strategy is:

```text
Backend contract first -> backend tests -> frontend consumes contract -> frontend tests -> integration verification
```

Each slice should be implemented incrementally. Do **not** finish a whole PR before testing. Implement a small part, run the relevant tests, then continue.

---

## 1. Issue Boundary

## P1-05 Backend Scope

### Summary

Move eligibility and location behavior onto provider/HR claims and return a non-blocking cross-city warning when employee city differs from event city.

### Backend Acceptance Criteria

- Given provider/HR claims include department, site, city, grade, and status, when eligibility is checked, then rules use those claims consistently.
- Given an employee city differs from event city, when event detail or eligibility is requested, then the API returns a clear warning without rejecting the employee.
- Given claims are missing, when eligibility or booking precheck runs, then the API returns a safe rejection or degraded reason without mutation.
- Given HR/provider claims change, when affected active registrations or tickets are identified, then pending impact review behavior remains compatible.
- Given structured logs/audit metadata include claims-related state, then full PII is redacted.

### Backend Impact Scope

- Eligibility domain / service / query code.
- Event summary / detail response models.
- Tests for city / site / claims behavior.

### Backend Non-Goals

- No frontend warning UI.
- No full provider sync adapter.
- No blocking distance / geocoding logic.
- City mismatch is advisory only.

---

## P1-06 Frontend Scope

### Summary

Update employee event browsing and detail pages to display limited/unlimited rules, family availability, eligibility reason, and non-blocking cross-city warnings.

### Frontend Acceptance Criteria

- Given event summaries include `capacity_type`, when an employee browses events, then limited and unlimited activities are visually distinct.
- Given an unlimited event allows family count, when the employee views details, then the UI explains family participation without implying transferable tickets.
- Given a city mismatch warning from the API, when the event card/detail renders, then the warning clearly states the activity city and does not block booking.
- Given an ineligible event or claims issue, when displayed, then eligibility reason remains visible and actionable.
- Given mobile and desktop viewports, when warnings and metadata render, then text does not overlap or overflow.

### Frontend Impact Scope

- Employee event list / detail pages.
- Formatting helpers and frontend API types.
- Frontend tests for warning and capacity-type rendering.

### Frontend Non-Goals

- No backend eligibility changes.
- No booking form / cancellation changes.
- No visual redesign beyond required states.

---

# 2. PR Split Recommendation

## Recommended Approach

Use **two Linear-aligned PRs**, but each PR should be implemented in smaller commits or internal checkpoints.

This means:

```text
PR 1 = P1-05 backend only
  commit/checkpoint A1 -> test
  commit/checkpoint A2 -> test
  commit/checkpoint A3 -> test
  final backend gate

PR 2 = P1-06 frontend only
  commit/checkpoint B1 -> test
  commit/checkpoint B2 -> test
  commit/checkpoint B3 -> test
  final frontend gate
```

Do not make one giant PR containing both P1-05 and P1-06 unless the reviewer explicitly asks for a full-stack PR. Since Linear already separates backend and frontend scopes, separate PRs are cleaner and easier to review.

## Why not one giant PR?

A single full-stack PR increases risk:

- backend contract changes and frontend UX changes become harder to review separately
- test failures are harder to localize
- P1-05 says “No frontend warning UI”
- P1-06 says “No backend eligibility changes”
- reviewer may reject it as scope creep

## Why not too many tiny PRs?

Too many tiny PRs can also slow development because each PR needs review and merge coordination. The balanced approach is:

- **2 PRs total**: one backend, one frontend
- **multiple commits/checkpoints inside each PR**
- run tests after each checkpoint

---

# 3. Milestone A — P1-05 Backend

Implement backend first because P1-06 depends on API response fields from P1-05.

---

## Phase A0 — Backend Baseline Verification

### Goal

Verify the backend test suite before changing eligibility behavior.

### Commands

```bash
go build ./services/api/...
go vet ./services/api/...
go test ./services/api/... -count=1
```

### Stop Condition

If baseline tests already fail, document:

```text
Known baseline failure:
- Command:
- Error:
- Existing or caused by current work:
```

Do not start implementation until the baseline state is clear.

---

## Phase A1 — Extend Actor with Provider/HR Claims

### Linear Issue

```text
[Phase 1][P1-05] Backend eligibility claims and location warning policy
```

### Related Backend Agent Sections

- Step 1 — Extend `Actor` to carry provider claims.
- Step 2 — Populate `Actor.Claims` in the HTTP auth layer.

### Files

```text
services/api/internal/ticketing/models.go
services/api/internal/httpapi/auth_provider.go
```

### Required Work

Add provider claims to the ticketing domain actor:

```go
type Actor struct {
    ID     string
    Role   string
    Claims *ProviderClaims
}
```

Add domain-level claims type:

```go
type ProviderClaims struct {
    Department       string
    Site             string
    City             string
    Grade            string
    EmploymentStatus string
}
```

Populate `Actor.Claims` from the verified provider token in the HTTP auth layer.

### Important Note

Linear says claims include:

```text
department, site, city, grade, and status
```

So do not only include department/site/city if the current issue requires grade and status too.

### Tests After Phase A1

```bash
go build ./services/api/...
go test ./services/api/internal/httpapi/... -run TestProviderVerifier -count=1
go test ./services/api/... -count=1
```

### Stability Checks

- `ticketing` must not import `httpapi`.
- `ProviderClaims` must be in the domain layer.
- `Claims` must be nullable for system / worker actors.
- No provider claims should be logged.

### Suggested Commit

```text
git commit -m "P1-05: carry provider claims on ticketing actor"
```

---

## Phase A2 — Add Typed Eligibility Decision and Warning Model

### Linear Issue

```text
[Phase 1][P1-05] Backend eligibility claims and location warning policy
```

### Related Backend Agent Sections

- Step 3 — Add new domain types for the eligibility decision.

### Files

```text
services/api/internal/ticketing/eligibility_models.go
docs/openapi/components/schemas/eligibility.yaml
docs/openapi/components/schemas/events.yaml
docs/openapi/components/schemas/common.yaml
```

### Required Work

Add a typed warning model:

```go
type WarningCode string

const (
    WarningCrossCity WarningCode = "cross_city"
)
```

Add warning payload:

```go
type EligibilityWarning struct {
    Code         WarningCode `json:"code"`
    Message      string      `json:"message"`
    EmployeeCity string      `json:"employee_city,omitempty"`
    EventCity    string      `json:"event_city,omitempty"`
}
```

Add typed eligibility decision:

```go
type EligibilityDecision struct {
    EventID        string               `json:"event_id"`
    Eligible       bool                 `json:"eligible"`
    CanBook        bool                 `json:"can_book"`
    Reasons        []string             `json:"reasons"`
    Warnings       []EligibilityWarning `json:"warnings"`
    NoShowCooldown NoShowCooldown      `json:"no_show_cooldown"`
}
```

### Tests After Phase A2

```bash
go build ./services/api/...
go test ./services/api/internal/ticketing/... -count=1
ruby scripts/check-openapi-contract.rb
```

### Stability Checks

- Do not use `map[string]interface{}` for employee-facing eligibility response.
- `cross_city` is warning data, not an error.
- `cross_city` must not make `eligible=false`.
- `cross_city` must not make `can_book=false`.

### Suggested Commit

```text
git commit -m "P1-05: add typed eligibility decision and warning contract"
```

---

## Phase A3 — Implement Claims-based Eligibility

### Linear Issue

```text
[Phase 1][P1-05] Backend eligibility claims and location warning policy
```

### Related Backend Agent Sections

- Step 4 — Implement `CheckEligibilityFromClaims`.
- Step 5 — Replace `CheckEligibility` signature.
- Step 6 — DB migration if `event_city` is absent.

### Files

```text
services/api/internal/ticketing/eligibility_service.go
services/api/internal/httpapi/contracts.go
services/api/internal/httpapi/handlers_eligibility.go
services/api/internal/postgres/migrations/NNNN_add_event_city.sql
```

### Required Behavior

`CheckEligibilityFromClaims` must:

1. Read department, site, city, grade, and status from `actor.Claims`.
2. Return safe rejection / degraded reason when claims are missing.
3. Avoid mutation when claims are missing.
4. Evaluate eligibility from claims consistently.
5. Load event city from event summary/detail data source.
6. Return `cross_city` warning when employee city differs from event city.
7. Not reject employee because of city mismatch alone.
8. Keep impact review behavior compatible when claims change.

### Missing Claims Behavior

Missing claims must return safe degraded response or sentinel error.

Example intent:

```text
claims missing -> no mutation -> actionable reason -> not 500
```

### Cross-city Behavior

Correct:

```text
eligible = true
can_book = true
warnings = [{ code: "cross_city" }]
```

Incorrect:

```text
eligible = false because city differs
can_book = false because city differs
```

### Tests After Phase A3

```bash
go build ./services/api/...
go test ./services/api/internal/ticketing/... -run TestCheckEligibility -v -count=1
go test ./services/api/internal/httpapi/... -count=1
go test ./services/api/... -count=1
```

### Required Backend Unit Tests

```text
TestCheckEligibilityFromClaims_Eligible
TestCheckEligibilityFromClaims_IneligibleDepartment
TestCheckEligibilityFromClaims_IneligibleSite
TestCheckEligibilityFromClaims_IneligibleGrade
TestCheckEligibilityFromClaims_IneligibleStatus
TestCheckEligibilityFromClaims_CrossCityWarning
TestCheckEligibilityFromClaims_CrossCityDoesNotBlock
TestCheckEligibilityFromClaims_MissingClaims
TestCheckEligibilityFromClaims_MissingEvent
```

### Required Backend Integration Tests

```text
TestCheckEligibilityFromClaims_DBBacked
TestCheckEligibilityFromClaims_MissingClaimsNoMutation
TestEligibilityImpactReview_ClaimsChangeCompatible
```

### Suggested Commit

```text
git commit -m "P1-05: evaluate eligibility from provider claims"
```

---

## Phase A4 — Event Summary / Detail Response Models

### Linear Issue

```text
[Phase 1][P1-05] Backend eligibility claims and location warning policy
```

### Related Backend Agent Sections

- Step 8 — Update `EventSummary` to include `EligibilityDecision`.

### Files

```text
services/api/internal/ticketing/models.go
services/api/internal/ticketing/events_service.go
docs/openapi/components/schemas/events.yaml
```

### Required Work

Event summary and event detail API responses must include enough data for frontend P1-06:

```text
capacity_type
family availability fields
eligibility decision
eligibility reasons
cross_city warnings
```

Suggested shape:

```go
type EventSummary struct {
    // existing event fields
    CapacityType string `json:"capacity_type"`

    // family-related fields, if already part of domain model
    AllowsFamilyCount bool `json:"allows_family_count,omitempty"`
    MaxFamilyCount    int  `json:"max_family_count,omitempty"`

    Eligibility EligibilityDecision `json:"eligibility"`
}
```

Exact field names should follow existing project conventions and OpenAPI schema.

### Tests After Phase A4

```bash
go build ./services/api/...
go test ./services/api/internal/ticketing/... -run "TestListEvents|TestGetEvent|TestCheckEligibility" -v -count=1
go test ./services/api/... -count=1
ruby scripts/check-openapi-contract.rb
```

### Stability Checks

- Event list/detail response includes warning data.
- Frontend should not need one eligibility request per event card.
- Keep response backward-compatible if existing UI depends on old fields.
- OpenAPI and Go JSON tags must match.

### Suggested Commit

```text
git commit -m "P1-05: include eligibility decision in event responses"
```

---

## Phase A5 — PII Redaction and Safe Error Handling

### Linear Issue

```text
[Phase 1][P1-05] Backend eligibility claims and location warning policy
```

### Related Backend Agent Sections

- Step 7 — PII redaction in logs and audit metadata.
- Observability Checklist.

### Files

```text
services/api/internal/ticketing/eligibility_service.go
services/api/internal/httpapi/handlers_eligibility.go
services/api/internal/httpapi/service_response.go
```

### Required Work

Search for raw employee ID and claims logging:

```bash
grep -n "employee_id\|EmployeeID\|actor\.ID\|Claims\|Department\|Site\|City" \
  services/api/internal/ticketing/eligibility_service.go \
  services/api/internal/httpapi/handlers_eligibility.go
```

Use masked employee reference in logs/audit metadata:

```go
"employee_ref": maskID(actor.ID)
```

Do not log:

```text
raw employee_id
raw provider token
full provider claims
```

### Required Tests

```text
TestMaskID_Short
TestMaskID_Normal
TestMissingClaimsMapsToSafeClientError
```

### Tests After Phase A5

```bash
go test ./services/api/internal/ticketing/... -run "TestMaskID|TestCheckEligibility" -v -count=1
go test ./services/api/internal/httpapi/... -count=1
go test ./services/api/... -count=1
```

### Suggested Commit

```text
git commit -m "P1-05: redact PII in eligibility logs and audit metadata"
```

---

## Phase A6 — Backend Final Gate for P1-05

### Required Commands

```bash
go build ./services/api/...
go vet ./services/api/...
go test ./services/api/... -count=1
ruby scripts/check-openapi-contract.rb
```

Optional DB-backed tests:

```bash
go test ./services/api/... -tags integration -count=1
```

### P1-05 Done Criteria

- Claims-based eligibility works.
- Cross-city warning is returned by API.
- Cross-city warning does not reject employee.
- Missing claims produce safe rejection / degraded reason.
- Event summary/detail response models include required frontend fields.
- PII is redacted in logs/audit metadata.
- Backend tests pass.
- OpenAPI contract is updated.
- No frontend UI code changed.

---

# 4. Milestone B — P1-06 Frontend UX

Start P1-06 only after P1-05 API contract is stable or mocked by fixture data matching the final OpenAPI contract.

---

## Phase B0 — Frontend Baseline Verification

### Commands

Use the actual package name from the repo. Linear says:

```bash
pnpm --filter cets-web test
pnpm --filter cets-web build
```

If the repo currently uses `web` instead of `cets-web`, use the package name in `apps/web/package.json`.

### Baseline Commands

```bash
pnpm --filter cets-web test
pnpm --filter cets-web build
```

Optional if configured:

```bash
pnpm --filter cets-web lint
```

---

## Phase B1 — Frontend API Types

### Linear Issue

```text
[Phase 1][P1-06] Employee event browse/detail warning UX
```

### Related Frontend Agent Sections

- Contract-first API boundary.
- Frontend API types.
- Code quality checklist.

### Files

```text
apps/web/src/lib/api/contracts.ts
apps/web/src/api.test.ts
```

### Required Work

Add or update frontend types for:

```text
capacity_type
family availability
eligibility decision
eligibility reasons
eligibility warnings
cross_city warning
```

Example:

```ts
export type WarningCode = "cross_city" | string;

export interface EligibilityWarning {
  code: WarningCode;
  message: string;
  employee_city?: string;
  event_city?: string;
}

export interface EligibilityDecision {
  event_id: string;
  eligible: boolean;
  can_book: boolean;
  reasons: string[];
  warnings: EligibilityWarning[];
}

export interface EventSummary {
  capacity_type?: "limited" | "unlimited" | string;
  eligibility?: EligibilityDecision;
}
```

Exact fields should match OpenAPI.

### Tests After Phase B1

```bash
pnpm --filter cets-web test -- api.test.ts
pnpm --filter cets-web test
pnpm --filter cets-web build
```

### Stability Checks

- Do not invent frontend-only eligibility rules.
- Do not infer city mismatch in frontend.
- Frontend consumes warning returned by API.
- Missing `eligibility` should not crash UI during rollout.

### Suggested Commit

```text
git commit -m "P1-06: add frontend event eligibility contract types"
```

---

## Phase B2 — Capacity Type Rendering

### Linear Issue

```text
[Phase 1][P1-06] Employee event browse/detail warning UX
```

### Acceptance Criteria Covered

```text
Given event summaries include capacity_type,
When an employee browses events,
Then limited and unlimited activities are visually distinct.
```

### Files

```text
apps/web/src/features/events/employee-pages.tsx
apps/web/src/features/events/employee-pages.test.tsx
apps/web/src/lib/formatting/index.ts
apps/web/src/lib/formatting/formatting.test.ts
```

### Required Work

Render limited/unlimited status on employee event cards.

Example UI meaning:

```text
Limited capacity
Unlimited activity
```

Use existing UI components such as badge/card if available.

### Tests After Phase B2

```bash
pnpm --filter cets-web test -- employee-pages
pnpm --filter cets-web test -- formatting
pnpm --filter cets-web build
```

### Required Tests

```text
renders limited capacity event distinctly
renders unlimited event distinctly
unknown capacity_type has safe fallback
```

### Suggested Commit

```text
git commit -m "P1-06: render limited and unlimited event capacity states"
```

---

## Phase B3 — Family Availability Explanation

### Linear Issue

```text
[Phase 1][P1-06] Employee event browse/detail warning UX
```

### Acceptance Criteria Covered

```text
Given an unlimited event allows family count,
When the employee views details,
Then the UI explains family participation without implying transferable tickets.
```

### Files

```text
apps/web/src/features/events/employee-pages.tsx
apps/web/src/features/events/employee-pages.test.tsx
apps/web/src/lib/formatting/index.ts
apps/web/src/lib/formatting/formatting.test.ts
```

### Required Work

When an event allows family participation, display copy that explains family count without suggesting tickets are transferable.

Good wording:

```text
Family participation allowed. Family count is used for planning and entry support; tickets remain tied to the employee registration.
```

Avoid wording like:

```text
You can transfer these tickets to family members.
```

### Tests After Phase B3

```bash
pnpm --filter cets-web test -- employee-pages
pnpm --filter cets-web test -- formatting
pnpm --filter cets-web build
```

### Required Tests

```text
renders family participation explanation
does not use transferable-ticket language
handles no family participation state
```

### Suggested Commit

```text
git commit -m "P1-06: explain family participation without transfer wording"
```

---

## Phase B4 — Cross-city Warning UI

### Linear Issue

```text
[Phase 1][P1-06] Employee event browse/detail warning UX
```

### Acceptance Criteria Covered

```text
Given a city mismatch warning from the API,
When the event card/detail renders,
Then the warning clearly states the activity city and does not block booking.
```

### Files

```text
apps/web/src/features/events/eligibility-warning.tsx
apps/web/src/features/events/eligibility-warning.test.tsx
apps/web/src/features/events/employee-pages.tsx
apps/web/src/features/events/employee-pages.test.tsx
```

### Required Work

Create a small warning component:

```text
EligibilityWarningBanner
```

Responsibilities:

```text
Input: EligibilityWarning[]
Output: warning UI
Do not decide booking state
Do not fetch data
Do not mutate warnings
```

Warning must clearly state:

```text
activity city
employee city if available
non-blocking nature if appropriate
```

Example copy:

```text
This activity is in Taipei. Your registered city is Hsinchu. You may still continue if you are eligible.
```

### Booking Button Rule

Correct:

```ts
const isBookingDisabled = event.eligibility?.can_book === false;
```

Incorrect:

```ts
const isBookingDisabled = event.eligibility?.warnings.length > 0;
```

### Tests After Phase B4

```bash
pnpm --filter cets-web test -- eligibility-warning
pnpm --filter cets-web test -- employee-pages
pnpm --filter cets-web build
```

### Required Tests

```text
renders cross_city warning
warning includes activity city
warning does not disable booking
unknown warning code does not crash
empty warnings render nothing
```

### Suggested Commit

```text
git commit -m "P1-06: render non-blocking cross-city event warnings"
```

---

## Phase B5 — Eligibility Reason / Claims Issue UI

### Linear Issue

```text
[Phase 1][P1-06] Employee event browse/detail warning UX
```

### Acceptance Criteria Covered

```text
Given an ineligible event or claims issue,
When displayed,
Then eligibility reason remains visible and actionable.
```

### Files

```text
apps/web/src/features/events/employee-pages.tsx
apps/web/src/features/events/employee-pages.test.tsx
apps/web/src/components/ui/alert.tsx
```

### Required Work

Display eligibility reasons when:

```text
eligibility.eligible = false
eligibility.can_book = false
eligibility.reasons is non-empty
claims issue / degraded reason is returned
```

Reason should be visible near the event action area.

Actionable copy examples:

```text
You are not eligible for this activity because your department does not match the rule.
Please contact HR or the event organizer if this looks incorrect.
```

### Tests After Phase B5

```bash
pnpm --filter cets-web test -- employee-pages
pnpm --filter cets-web build
```

### Required Tests

```text
ineligible reason remains visible
claims issue reason remains visible
booking is disabled when can_book=false
warning and reason can render together
```

### Suggested Commit

```text
git commit -m "P1-06: keep eligibility reasons visible and actionable"
```

---

## Phase B6 — Responsive Warning / Metadata Layout

### Linear Issue

```text
[Phase 1][P1-06] Employee event browse/detail warning UX
```

### Acceptance Criteria Covered

```text
Given mobile and desktop viewports,
When warnings and metadata render,
Then text does not overlap or overflow.
```

### Files

```text
apps/web/e2e/core-role-routes.spec.ts
apps/web/src/features/events/employee-pages.test.tsx
apps/web/src/layout-hardening.css
apps/web/src/styles.css
```

### Required Viewports

```text
375px
768px
1024px
1440px
```

### Required Tests

```text
event cards do not overflow at 375px
detail warning does not overlap action area at 375px
metadata wraps correctly at 768px
desktop layout remains readable at 1024px and 1440px
```

### Test Commands

```bash
pnpm --filter cets-web test
pnpm --filter cets-web build
```

Optional if Playwright is configured:

```bash
pnpm --filter cets-web exec playwright test apps/web/e2e/core-role-routes.spec.ts
```

### Suggested Commit

```text
git commit -m "P1-06: harden responsive event metadata layout"
```

---

## Phase B7 — Frontend Final Gate for P1-06

### Required Commands

```bash
pnpm --filter cets-web test
pnpm --filter cets-web build
```

Optional if configured:

```bash
pnpm --filter cets-web lint
pnpm --filter cets-web exec playwright test
```

### P1-06 Done Criteria

- Limited and unlimited activities are visually distinct.
- Family participation is explained without implying transferable tickets.
- Cross-city warning is displayed from API data.
- Cross-city warning does not block booking.
- Ineligible / claims issue reason remains visible and actionable.
- Mobile and desktop layouts do not overlap or overflow.
- Frontend tests pass.
- No backend eligibility code changed.

---

# 5. Cross-Issue Integration Verification

After both P1-05 and P1-06 are complete, run a final integration check.

## Full Backend Gate

```bash
go build ./services/api/...
go vet ./services/api/...
go test ./services/api/... -count=1
ruby scripts/check-openapi-contract.rb
```

## Full Frontend Gate

```bash
pnpm --filter cets-web test
pnpm --filter cets-web build
```

## Manual Integration Scenarios

### Scenario 1 — Eligible Same City

```text
Given employee claims match event rules
And employee city equals event city
Then event detail shows eligible state
And no cross-city warning appears
And booking is not blocked
```

### Scenario 2 — Eligible Cross City

```text
Given employee claims match event rules
And employee city differs from event city
Then API returns cross_city warning
And frontend displays activity city
And booking is not blocked
```

### Scenario 3 — Ineligible Rule

```text
Given employee claims do not match event rules
Then API returns eligible=false or can_book=false
And frontend shows reason
And booking is blocked
```

### Scenario 4 — Missing Claims

```text
Given provider claims are missing or incomplete
Then API returns safe rejection / degraded reason
And no mutation occurs
And frontend shows actionable reason
```

### Scenario 5 — Responsive Layout

```text
Given event card/detail contains capacity metadata, family explanation, warning, and reason
When rendered at 375px, 768px, 1024px, and 1440px
Then text does not overlap or overflow
```

---

# 6. Recommended PR Structure

## PR 1 — P1-05 Backend

Title:

```text
[P1-05] Backend eligibility claims and location warning policy
```

Body must include:

```text
Linear: P1-05
```

Includes:

```text
Actor.Claims
ProviderClaims
EligibilityDecision
EligibilityWarning
cross_city warning
Event summary/detail response models
PII redaction
Backend tests
OpenAPI schema update
```

Must not include:

```text
Frontend warning UI
Frontend page changes
Booking form/cancellation changes
```

Suggested internal checkpoints:

```text
A1 Actor claims -> test
A2 EligibilityDecision contract -> test
A3 Claims-based eligibility -> test
A4 Event response models -> test
A5 PII redaction -> test
A6 final backend gate
```

---

## PR 2 — P1-06 Frontend

Title:

```text
[P1-06] Employee event browse/detail warning UX
```

Body must include:

```text
Linear: P1-06
```

Includes:

```text
Frontend API types
Capacity type rendering
Family availability copy
Cross-city warning UI
Eligibility reason UI
Responsive tests
Frontend tests
```

Must not include:

```text
Backend eligibility logic changes
Booking flow changes
Cancellation changes
```

Suggested internal checkpoints:

```text
B1 Frontend API types -> test
B2 Capacity type rendering -> test
B3 Family availability copy -> test
B4 Cross-city warning UI -> test
B5 Eligibility reason UI -> test
B6 Responsive layout -> test
B7 final frontend gate
```

---

# 7. Agent Execution Rule

The agent must follow this rule:

```text
Implement one phase at a time.
Run the phase-specific tests after each phase.
Do not proceed when tests fail.
Do not combine P1-05 and P1-06 into one uncontrolled change.
Respect each Linear issue's Non-Goals.
```

---

# 8. One-line Success Criteria

```text
P1-05 makes the API return claims-based eligibility and non-blocking cross_city warnings safely;
P1-06 makes the employee event list/detail UI display capacity, family availability, warnings, and reasons clearly without blocking booking incorrectly.
```
