# Feature: COR-19 and COR-20 Provider Claims Auth Shell

## Summary

COR-19 replaces authentication with provider-signed employee claims. COR-20 updates the web shell so product users enter through the provider session first, while local, demo, and test use mock metadata profiles that issue provider-format bearer tokens.

## Acceptance Criteria

- [ ] AC-1: Given a request with a valid provider bearer token, When `/api/v1/auth/me` is called, Then the API returns complete employee claims with exactly one mapped application role.
- [ ] AC-2: Given a production protected API request, When only local session cookies or legacy actor headers are provided, Then the request is rejected with `401`.
- [ ] AC-3: Given a provider token with missing, expired, tampered, unmapped, or ambiguous role claims, When a protected API is called, Then authentication fails with `401` for invalid identity and `403` for role mapping rejection.
- [ ] AC-4: Given local, demo, or test app environments, When a mock profile is selected, Then the API issues a provider-format bearer token and complete claims.
- [ ] AC-5: Given the web app starts with valid provider claims, When `/auth/me` succeeds, Then the user lands in the product shell without local login cards or product logout controls.
- [ ] AC-6: Given the web app starts without a provider session, When `/auth/bootstrap` reports mock profiles disabled, Then the app shows an explicit SSO-required state.
- [ ] AC-7: Given the web app starts without a provider session, When `/auth/bootstrap` reports mock profiles enabled, Then the mock metadata profile selector is available.
- [ ] AC-8: Given an employee self-service request, When the caller supplies `employee_id` in query, path, or booking body, Then the API rejects the request and derives canonical identity only from provider claims.

## Edge Cases

| # | Scenario | Expected Behavior |
|---|----------|-------------------|
| E-1 | Empty or malformed Authorization header | Return `401 authentication required`; do not fall back to cookie or legacy headers. |
| E-2 | Provider token signature mismatch or expired `exp` | Return `401 authentication required`; do not log the raw token. |
| E-3 | Provider token maps to zero or multiple app roles | Return `403 role is not allowed`. |
| E-4 | Local session cookie or legacy role headers are provided | Return `401`; auth does not fall back to removed local paths. |
| E-5 | Production request calls mock profile token endpoint | Return `404`; production requires real provider bearer claims. |
| E-6 | Employee self-service request supplies `employee_id` | Return `400`; do not call the business service or mutate state. |

## Non-Functional Requirements

| Category | Requirement | Metric |
|----------|-------------|--------|
| Config | Provider token secret is runtime config only | `PROVIDER_TOKEN_SECRET` required and non-demo in production |
| Security | Provider tokens are not persisted by the web client | No localStorage/sessionStorage token writes |
| Observability | Auth logs redact identities and secrets | No raw token, employee ID, or full PII in logs |
| Compatibility | Mock profile auth remains isolated | Mock token endpoint only in local/demo/test |
| Dependencies | HMAC provider verification uses Go standard library | No new backend dependency |

## Minimal API Contract

```text
GET /api/v1/auth/me
Authorization: Bearer <provider-token>
```

Provider token format:

```text
base64url(json_claims).base64url(hmac_sha256(payload, PROVIDER_TOKEN_SECRET))
```

Required provider claims:

```json
{
  "employee_id": "E1001",
  "display_name": "Ariel Chen",
  "role_claims": ["employee"],
  "department": "Engineering",
  "site": "Taipei HQ",
  "city": "Taipei",
  "exp": 1770000000
}
```

Success response:

```json
{
  "success": true,
  "data": {
    "employee_id": "E1001",
    "display_name": "Ariel Chen",
    "job_title": null,
    "role_claims": ["employee"],
    "mapped_roles": ["employee"],
    "department": "Engineering",
    "site": "Taipei HQ",
    "city": "Taipei",
    "claims_status": "complete"
  },
  "error": null
}
```

Canonical employee self-service endpoints:

```text
GET  /api/v1/events
GET  /api/v1/events/{event_id}
GET  /api/v1/events/{event_id}/eligibility
POST /api/v1/events/{event_id}/bookings
GET  /api/v1/me/tickets
```

The server derives the employee identity from provider claims. These endpoints must reject caller-supplied `employee_id`; `/api/v1/employees/{employee_id}/tickets` is not a product route.

```text
GET /api/v1/auth/bootstrap
```

Success response:

```json
{
  "success": true,
  "data": {
    "mock_profiles_enabled": false,
    "mock_profiles": []
  },
  "error": null
}
```

```text
POST /api/v1/auth/mock-provider-token
Content-Type: application/json

{ "profile_id": "E1001" }
```

Development-only success response:

```json
{
  "success": true,
  "data": {
    "provider_token": "<redacted>",
    "expires_at": "2026-05-05T18:00:00Z",
    "claims": {
      "employee_id": "E1001",
      "display_name": "Ariel Chen",
      "role_claims": ["employee"],
      "mapped_roles": ["employee"],
      "department": "Engineering",
      "site": "Taipei HQ",
      "city": "Taipei",
      "claims_status": "complete"
    }
  },
  "error": null
}
```

## 12-Factor Compliance Notes

- Config: `PROVIDER_TOKEN_SECRET` is read from env and validated with production secret rules.
- Backing Services: No new backing service is introduced for HMAC provider verification.
- Processes: The web client keeps provider tokens in memory only; server processes remain stateless.
- Logs: Auth events use redacted actor references and role names only.
- Dev/Prod Parity: Docker Compose exposes provider secret config for local validation while production disables mock token issuance.
