# Feature: Local SSO Login Flow

## Summary

Phase 1 replaces browser-trusted demo actor headers with a server-issued local SSO session. The Go app signs a stateless `HttpOnly` cookie, the React SPA reads only `/api/v1/auth/me`, and existing ticketing APIs continue receiving a `ticketing.Actor` from server-side authentication. This is a local SSO simulation for mentor/demo usage, not real enterprise OIDC.

## Acceptance Criteria

- [ ] AC-1: Given a known local principal, When the SPA posts `principal_id` to `/api/v1/auth/login`, Then the API returns actor/session metadata and sets a `cets_session` cookie with `HttpOnly` and `SameSite=Lax`.
- [ ] AC-2: Given an unknown or empty principal, When login is submitted, Then the API returns `401` and does not set a valid session.
- [ ] AC-3: Given no valid session cookie, When the SPA calls a protected `/api/v1/*` ticketing endpoint, Then the API returns `401`.
- [ ] AC-4: Given a valid session cookie, When the SPA calls ticketing APIs, Then the server resolves the actor from the cookie and role checks remain enforced by application services.
- [ ] AC-5: Given a valid session, When `/api/v1/auth/logout` is called, Then the response expires `cets_session` and `/api/v1/auth/me` returns `401`.
- [ ] AC-6: Given `APP_ENV=production`, When a request provides only `X-Actor-ID` / `X-Role`, Then the API rejects it instead of trusting legacy demo headers.
- [ ] AC-7: Given `APP_ENV=local`, `demo`, or `test`, When existing tests or demo helpers use legacy headers, Then they continue to work for backward compatibility.
- [ ] AC-8: Given the React SPA is unauthenticated, When it loads any browser route, Then it shows the quiet local SSO login screen instead of privileged workspace data.
- [ ] AC-9: Given the user changes role in the Demo Runbook, When each runbook step runs, Then the web UI switches local sessions through login calls and completes the existing AC-9 flow.

## Edge Cases

| # | Scenario | Expected Behavior |
|---|---|---|
| E-1 | Empty JSON, malformed JSON, or missing `principal_id` | Return `400` for malformed JSON or `401` for unknown principal. |
| E-2 | Tampered, expired, or malformed `cets_session` | Return `401`, do not call the ticketing service, and do not leak token content. |
| E-3 | Role mismatch for a valid session | Existing service authorization returns `403`. |
| E-4 | Logout without a valid session | Return success and clear the cookie idempotently. |
| E-5 | Demo seed in production | Preserve existing hidden behavior and return `404`. |
| E-6 | Browser refresh after login | `/auth/me` restores the authenticated SPA state from the cookie. |

## Non-Functional Requirements

| Category | Requirement | Metric |
|---|---|---|
| Timeout | Auth endpoints run inside the existing request timeout middleware. | Same `REQUEST_TIMEOUT_MS`, default 5000ms. |
| Security | Session cookie is `HttpOnly`, `SameSite=Lax`, signed with HMAC-SHA256, and redacted from API activity. | No raw session token in JS state or logs. |
| Config | Auth secret, TTL, and secure-cookie behavior come from env vars. | Production rejects demo/default secrets and insecure cookies. |
| Observability | Login/logout emit structured logs without PII beyond principal and role. | One log event per login/logout. |
| Statelessness | Session data is contained in signed claims; no local files or in-memory session maps. | App remains horizontally scalable. |

## Minimal API Contract

```text
POST /api/v1/auth/login
Content-Type: application/json

{ "principal_id": "E1001" }
```

Success response:

```json
{
  "success": true,
  "data": {
    "actor": { "id": "E1001", "role": "employee" },
    "expires_at": "2026-05-05T18:00:00Z"
  },
  "error": null
}
```

Error response:

```json
{ "success": false, "data": null, "error": "invalid local SSO principal" }
```

```text
GET /api/v1/auth/me
Cookie: cets_session=<signed claims>
```

```text
POST /api/v1/auth/logout
Cookie: cets_session=<signed claims>
```

## 12-Factor Compliance Notes

- Config: `AUTH_SESSION_SECRET`, `AUTH_SESSION_TTL_MINUTES`, and `AUTH_COOKIE_SECURE` are env-based. Production must not use local/demo secrets and must enable secure cookies.
- Backing Services: No new backing service is introduced; sessions are stateless signed cookies.
- Logs: Auth events use structured logs to stdout and do not log cookie values.
- Processes: No session map, sticky process, or local state is introduced.
- Build / Release / Run: Docker image remains immutable; auth behavior varies only by env config.
- Dev / Prod Parity: Local Compose exercises the same cookie-based auth path with non-secure cookies for localhost.
