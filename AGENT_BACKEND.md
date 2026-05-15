# Agent Task: Backend — Eligibility Claims Migration & Cross-City Warning

**Linear Issue:** `COR-19 / COR-20` (include in PR title and body)
**Scope:** `services/api/internal/ticketing/` · `services/api/internal/httpapi/`
**Validation command:** `go test ./services/api/... -count=1`

---

## Context

This system is an enterprise employee event-ticketing platform. Authentication is
provider-signed bearer tokens (HMAC-SHA256). The `providerClaims` struct in
`internal/httpapi/auth_provider.go` already carries `Department`, `Site`, `City`,
`RoleClaims`, and `ExpiresAt`. The `Actor` struct in `internal/ticketing/models.go`
currently only holds `ID` and `Role`.

The eligibility domain (`eligibility_service.go`, `eligibility_models.go`) today
reads employee attributes directly from the `employees` DB table. This task moves
the **employee-facing eligibility check** (`CheckEligibility`) onto provider/HR
claims already present in the request, and adds a **non-blocking cross-city
warning** when the employee's city differs from the event's city.

**Non-goals for this task:**
- No frontend warning UI changes (separate ticket).
- No full provider sync adapter or geocoding logic.
- No changes to admin `PreviewEligibility` / `UpdateEligibility` flows (they still
  count against the `employees` table).
- No blocking behaviour for city mismatch — advisory warning only.

---

## Architecture Constraints

- `Actor` is the only identity object available inside the ticketing domain layer.
  Provider claims must flow through it; do **not** add HTTP dependencies to the
  ticketing package.
- All new exported types go in `eligibility_models.go` (or a new
  `eligibility_claims_models.go` if the file grows large).
- PII rule: `employee_id` must be masked in structured logs and audit metadata.
  City and department are **not** PII but must not appear in error messages
  returned to the caller.
- Dual-write safety: any DB write that also emits an outbox event must remain in
  the same transaction (`insertOutbox` + business write inside one `tx.Commit`).

---

## Step-by-Step Implementation

### Step 1 — Extend `Actor` to carry provider claims

**File:** `services/api/internal/ticketing/models.go`

1. Locate the `Actor` struct (currently `ID string`, `Role string`).
2. Add a `Claims *ProviderClaims` field (pointer — nil for system/worker actors
   that have no HTTP context):

   ```go
   type Actor struct {
       ID     string
       Role   string
       Claims *ProviderClaims
   }
   ```

3. Add the new `ProviderClaims` struct **in the same file** (not in
   `auth_provider.go` — that package must not be imported by ticketing):

   ```go
   // ProviderClaims holds the verified identity attributes from the provider
   // bearer token. Fields mirror providerClaims in httpapi/auth_provider.go;
   // keep them in sync manually — no import cycle is allowed.
   type ProviderClaims struct {
       Department string
       Site       string
       City       string
   }
   ```

   > **Why a separate struct?** `httpapi.providerClaims` is unexported and lives
   > in the HTTP layer. Duplicating the three fields we need avoids an import
   > cycle and keeps the domain layer free of HTTP concerns.

4. Run `go build ./services/api/...` — expect zero new errors at this point.

---

### Step 2 — Populate `Actor.Claims` in the HTTP auth layer

**File:** `services/api/internal/httpapi/auth_provider.go`

1. Find `IdentityFromToken` where it builds `authIdentity`. It currently sets
   `Actor: ticketing.Actor{ID: claims.EmployeeID, Role: mappedRoles[0]}`.
2. Change that line to also populate `Claims`:

   ```go
   Actor: ticketing.Actor{
       ID:   claims.EmployeeID,
       Role: mappedRoles[0],
       Claims: &ticketing.ProviderClaims{
           Department: claims.Department,
           Site:       claims.Site,
           City:       claims.City,
       },
   },
   ```

3. Verify the existing `auth_provider_test.go` still passes:
   `go test ./services/api/internal/httpapi/... -run TestProviderVerifier -count=1`

---

### Step 3 — Add new domain types for the eligibility decision

**File:** `services/api/internal/ticketing/eligibility_models.go`

Add the following types. Place them after the existing `EligibilityImpactReview`
block so the file stays in declaration order (models → requests → responses →
events).

```go
// WarningCode identifies a non-blocking advisory returned alongside an
// eligibility decision.
type WarningCode string

const (
    // WarningCrossCity is emitted when the employee's provider city differs
    // from the event city. It is advisory — it must never set Eligible=false
    // or CanBook=false on its own.
    WarningCrossCity WarningCode = "cross_city"
)

// EligibilityWarning is a structured, non-blocking advisory. It must never
// prevent booking on its own.
type EligibilityWarning struct {
    Code         WarningCode `json:"code"`
    Message      string      `json:"message"`
    EmployeeCity string      `json:"employee_city,omitempty"`
    EventCity    string      `json:"event_city,omitempty"`
}

// EligibilityDecision is the response returned to an employee for a single
// event eligibility check. It replaces the previous map[string]interface{}
// returned by CheckEligibility.
type EligibilityDecision struct {
    EventID       string               `json:"event_id"`
    Eligible      bool                 `json:"eligible"`
    CanBook       bool                 `json:"can_book"`
    Reasons       []string             `json:"reasons"`
    Warnings      []EligibilityWarning `json:"warnings"`
    NoShowCooldown NoShowCooldown      `json:"no_show_cooldown"`
}
```

> `NoShowCooldown` is already defined in `models.go` (or wherever the existing
> field lives — confirm with `grep -n NoShowCooldown services/api/internal/ticketing/*.go`
> before adding a duplicate).

---

### Step 4 — Implement `CheckEligibilityFromClaims` in the eligibility service

**File:** `services/api/internal/ticketing/eligibility_service.go`

This is the new claims-based path. Do **not** modify the existing
`CheckEligibility` signature yet — that is handled in Step 5.

Add the following function **above** `CheckEligibility`:

```go
// CheckEligibilityFromClaims evaluates employee eligibility using only the
// provider/HR claims already present on actor.Claims. It does not query the
// employees table for the employee's attributes, which means it is safe to call
// on the hot path without a DB round-trip for attribute lookup.
//
// Cross-city mismatch produces a non-blocking EligibilityWarning; it never sets
// Eligible or CanBook to false on its own.
//
// Returns ErrMissingClaims (a safe degraded rejection) when actor.Claims is nil
// or any required field is empty, so callers never panic on missing claims.
func (s *Service) CheckEligibilityFromClaims(
    ctx context.Context,
    actor Actor,
    eventID string,
) (EligibilityDecision, error) {
    // --- claims guard ---
    if actor.Claims == nil ||
        actor.Claims.Department == "" ||
        actor.Claims.Site == "" ||
        actor.Claims.City == "" {
        return EligibilityDecision{}, ErrMissingClaims
    }

    // --- load event and its eligibility rule ---
    event, err := s.loadEventWithRule(ctx, eventID)
    if err != nil {
        return EligibilityDecision{}, err
    }

    // --- evaluate rule against claims ---
    // Re-use existing EvaluateEligibility but construct a synthetic Employee
    // from claims so we avoid a DB round-trip for attribute lookup.
    synthetic := Employee{
        Department:       actor.Claims.Department,
        Site:             actor.Claims.Site,
        EmploymentStatus: "active", // claims do not carry status; treat as active
    }
    eligible, reason := EvaluateEligibility(synthetic, event.Rule)

    var reasons []string
    if reason != "" {
        reasons = append(reasons, reason)
    }

    // --- cross-city warning (non-blocking) ---
    var warnings []EligibilityWarning
    if event.EventCity != "" && actor.Claims.City != event.EventCity {
        warnings = append(warnings, EligibilityWarning{
            Code: WarningCrossCity,
            Message: fmt.Sprintf(
                "This event is in %s; your registered city is %s.",
                event.EventCity, actor.Claims.City,
            ),
            EmployeeCity: actor.Claims.City,
            EventCity:    event.EventCity,
        })
    }

    // --- no-show cooldown (read-only check; does not modify CanBook here) ---
    cooldown, err := s.activeNoShowCooldown(ctx, actor.ID, eventID)
    if err != nil {
        return EligibilityDecision{}, err
    }

    canBook := eligible && !cooldown.Active
    return EligibilityDecision{
        EventID:        eventID,
        Eligible:       eligible,
        CanBook:        canBook,
        Reasons:        reasons,
        Warnings:       warnings,
        NoShowCooldown: cooldown,
    }, nil
}

// ErrMissingClaims is returned when provider/HR claims are absent or
// incomplete. Callers must treat this as a safe degraded rejection (do not
// mutate state) and must not log the raw claims.
var ErrMissingClaims = errors.New("provider claims are missing or incomplete")
```

**Helper: `loadEventWithRule`** — add a small private helper that loads the event
row plus its eligibility rule in a single query. Place it at the bottom of
`eligibility_service.go`:

```go
type eventWithRule struct {
    EventID   string
    EventCity string
    Rule      EligibilityRule
}

func (s *Service) loadEventWithRule(ctx context.Context, eventID string) (eventWithRule, error) {
    var ev eventWithRule
    err := s.db.QueryRow(ctx, `
        SELECT e.event_id, COALESCE(e.event_city, ''),
               COALESCE(r.department, ''), COALESCE(r.site, ''),
               COALESCE(r.min_grade, 0), COALESCE(r.employment_status, '')
        FROM events e
        LEFT JOIN eligibility_rules r ON r.event_id = e.event_id
        WHERE e.event_id = $1 AND e.archived_at IS NULL
    `, eventID).Scan(
        &ev.EventID, &ev.EventCity,
        &ev.Rule.Department, &ev.Rule.Site,
        &ev.Rule.MinGrade, &ev.Rule.EmploymentStatus,
    )
    if errors.Is(err, pgx.ErrNoRows) {
        return ev, notFound("event not found")
    }
    return ev, err
}
```

> **Note:** `event_city` must exist on the `events` table. Confirm with
> `\d events` in psql. If the column is missing, a migration is needed (see
> Step 6).

---

### Step 5 — Replace `CheckEligibility` signature in the service and interface

The existing `CheckEligibility` returns `map[string]interface{}`, which is
untyped and untestable. Replace it with the new typed return.

**File:** `services/api/internal/ticketing/eligibility_service.go`

Replace the existing `CheckEligibility` function body:

```go
// CheckEligibility is the employee-facing eligibility entry point. It
// delegates to CheckEligibilityFromClaims and returns EligibilityDecision.
// Callers that previously used map[string]interface{} must be updated to use
// EligibilityDecision fields directly.
func (s *Service) CheckEligibility(
    ctx context.Context,
    actor Actor,
    eventID string,
    _ string, // employeeID arg retained for interface compat; ignored — use actor.Claims
) (EligibilityDecision, error) {
    return s.CheckEligibilityFromClaims(ctx, actor, eventID)
}
```

**File:** `services/api/internal/httpapi/contracts.go`

Update the `EligibilityService` interface to match:

```go
type EligibilityService interface {
    CheckEligibility(ctx context.Context, actor ticketing.Actor, eventID string, employeeID string) (ticketing.EligibilityDecision, error)
    // ... rest unchanged
}
```

**File:** `services/api/internal/httpapi/handlers_eligibility.go`

`handleEligibility` already calls `service.CheckEligibility` and passes the
result to `writeServiceResult`. Since `writeServiceResult` serialises any value,
**no change is needed** to the handler itself — the typed struct serialises
correctly.

Run `go build ./services/api/...` and resolve any compilation errors before
continuing.

---

### Step 6 — DB migration (if `event_city` column is absent)

Check first:

```bash
grep -r "event_city" services/api/internal/postgres/ services/api/internal/ticketing/
```

If `event_city` does not appear in any migration file:

1. Create `services/api/internal/postgres/migrations/NNNN_add_event_city.sql`
   (replace `NNNN` with the next sequential number):

   ```sql
   -- +migrate Up
   ALTER TABLE events ADD COLUMN IF NOT EXISTS event_city TEXT NOT NULL DEFAULT '';

   -- +migrate Down
   ALTER TABLE events DROP COLUMN IF EXISTS event_city;
   ```

2. The migration must be **backward compatible** — `DEFAULT ''` ensures existing
   rows are not broken. The application reads `COALESCE(e.event_city, '')` so an
   empty string is safe.

3. Run `go test ./services/api/internal/postgres/... -count=1` to confirm
   migration applies cleanly.

---

### Step 7 — PII redaction in logs and audit metadata

Search for all places that log or audit `employee_id` in the eligibility path:

```bash
grep -n "employee_id\|EmployeeID\|actor\.ID" \
  services/api/internal/ticketing/eligibility_service.go \
  services/api/internal/httpapi/handlers_eligibility.go
```

For each audit `insertAudit` call in `eligibility_service.go`:

- Replace `"employee_id": actor.ID` with `"employee_ref": maskID(actor.ID)`.
- `maskID` should already exist or be added as:

  ```go
  // maskID returns the first 4 characters of id followed by "****" to
  // prevent full employee IDs from appearing in audit metadata.
  func maskID(id string) string {
      if len(id) <= 4 {
          return "****"
      }
      return id[:4] + "****"
  }
  ```

- City and department are **not** PII but must not appear in error strings
  returned to the caller — confirm no `fmt.Errorf` uses `actor.Claims.City`
  or `actor.Claims.Department` in an error returned to HTTP.

---

### Step 8 — Update `EventSummary` to include `EligibilityDecision`

The `EventSummary` returned by `ListEvents` and `GetEvent` must embed the new
typed decision so the frontend can display warnings without a separate round-trip.

**File:** `services/api/internal/ticketing/models.go` (or wherever `EventSummary`
is defined — confirm with `grep -n "EventSummary" services/api/internal/ticketing/*.go`)

Add the `Eligibility` field:

```go
type EventSummary struct {
    // ... existing fields ...
    Eligibility EligibilityDecision `json:"eligibility"`
}
```

In `events_service.go` (or wherever `ListEvents` / `GetEvent` populate
`EventSummary`), call `s.CheckEligibilityFromClaims` for each event and populate
`Eligibility`. This replaces any existing flat `eligible`/`eligibility_reason`
fields on `EventSummary`.

> **Migration note:** the existing flat fields (`eligible bool`,
> `eligibility_reason string`) can be kept as deprecated shims that read from
> `Eligibility.Eligible` and `Eligibility.Reasons[0]` for one release cycle, then
> removed.

---

## Validation Plan

### Unit tests — `services/api/internal/ticketing/`

Create `services/api/internal/ticketing/eligibility_claims_test.go`:

| Test name | Scenario | Expected |
|---|---|---|
| `TestCheckEligibilityFromClaims_Eligible` | Claims match rule, same city | `Eligible=true`, `Warnings=[]` |
| `TestCheckEligibilityFromClaims_IneligibleDept` | Claims department mismatch | `Eligible=false`, `Reasons` contains department reason |
| `TestCheckEligibilityFromClaims_CrossCityWarning` | Eligible, but `actor.Claims.City != event.EventCity` | `Eligible=true`, `Warnings` contains one `cross_city` entry with both cities |
| `TestCheckEligibilityFromClaims_CrossCityDoesNotBlock` | City mismatch + no-show cooldown inactive | `CanBook=true` (city warning alone must not set `CanBook=false`) |
| `TestCheckEligibilityFromClaims_NilClaims` | `actor.Claims == nil` | Returns `ErrMissingClaims`; no DB mutation |
| `TestCheckEligibilityFromClaims_EmptyCityField` | `actor.Claims.City == ""` | Returns `ErrMissingClaims` |
| `TestCheckEligibilityFromClaims_MissingEvent` | Event ID does not exist | Returns `notFound` error |
| `TestMaskID_Short` | `id = "AB"` | Returns `"****"` |
| `TestMaskID_Normal` | `id = "E1001"` | Returns `"E100****"` |

Run with: `go test ./services/api/internal/ticketing/... -run TestCheckEligibility -v -count=1`

### Integration tests

In `service_integration_test.go` (DB-backed), add:

| Test | Scenario |
|---|---|
| `TestCheckEligibilityFromClaims_DBBacked` | Real event + rule in DB; verify query returns correct city from `events.event_city` |
| `TestCheckEligibilityImpactReview_ClaimsChange` | After HR claims change, existing impact reviews remain in `pending` state and are not auto-resolved |

Run with: `go test ./services/api/... -tags integration -count=1`

### Build gate

```bash
go build ./services/api/...
go vet ./services/api/...
go test ./services/api/... -count=1
```

All three must exit 0 before the PR is opened.

---

## Observability Checklist

Before opening the PR, verify:

- [ ] No raw `actor.ID` appears in any `slog` / `log` call inside the eligibility
  path — use `maskID(actor.ID)` or `"employee_ref"`.
- [ ] No `actor.Claims.City`, `.Department`, or `.Site` appear in error strings
  returned to HTTP callers.
- [ ] `ErrMissingClaims` is mapped to HTTP 400 (not 500) in
  `writeServiceResult` / `service_response.go` — confirm the error sentinel is
  handled or add a case.
- [ ] The new `cross_city` warning appears in the OpenAPI schema
  (`docs/openapi/components/schemas/common.yaml` — `Warning.code` enum already
  includes `cross_city`; confirm it is not removed).
