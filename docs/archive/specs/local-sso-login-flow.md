# Development-Only: Local SSO Removed

## Summary

Local SSO cookie login has been removed. Non-production demos and automated tests now use mock provider metadata profiles that ask the backend to issue provider-format bearer tokens signed with `PROVIDER_TOKEN_SECRET`.

## Acceptance Criteria

- [ ] AC-1: Given `APP_ENV=local`, `demo`, or `test`, When `/api/v1/auth/bootstrap` is called, Then the API may return `mock_profiles_enabled=true` with mock profile metadata and no provider token.
- [ ] AC-2: Given a known mock profile in non-production, When `/api/v1/auth/mock-provider-token` is called, Then the API returns a provider bearer token, expiry, and complete claims.
- [ ] AC-3: Given `APP_ENV=production`, When `/api/v1/auth/mock-provider-token` is called, Then the API returns `404`.
- [ ] AC-4: Given any protected API request, When no valid provider bearer token is present, Then the API returns `401`.
- [ ] AC-5: Given `/api/v1/auth/login`, `/api/v1/auth/logout`, local session cookies, or legacy role headers, When they are used as auth paths, Then they do not authenticate the request.

## Development-Only API Contract

```text
POST /api/v1/auth/mock-provider-token
Content-Type: application/json

{ "profile_id": "E1001" }
```

Success response:

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

- Config: `PROVIDER_TOKEN_SECRET` is env-based and remains required/non-demo in production.
- Backing Services: No new backing service is introduced.
- Logs: Mock profile selection logs redacted actor references and roles only.
- Processes: Provider tokens are stateless bearer credentials and are held in frontend memory only.
- Dev / Prod Parity: Demo profiles exercise the same bearer verification path as production provider claims.
