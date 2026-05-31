# Fast Local Testing Goal

## Objective

Keep the Phase 3 branch focused on fast, repeatable local and CI testing. Do not deploy cloud
infrastructure for this goal.

## Scope

- Preserve the local Phase 3 Compose/LGTM simulation as the deployment-like test surface.
- Keep GitHub Actions and `act push` stable enough to catch regressions without spending excessive
  local time.
- Prefer fewer duplicated expensive checks over maximum parallelism when that improves reliability.
- Keep live Compose-backed Playwright and k6 checks opt-in unless a release gate explicitly needs
  them.

## Acceptance Criteria

- Repo contains no tracked cloud deployment directory or cloud deployment helper scripts.
- `act push` can complete using the local machine without long Playwright retry storms.
- Backend tests, frontend unit/build checks, OpenAPI checks, Compose contract checks, and mocked
  Playwright viewport checks remain available.
- Any skipped live or release-only checks are documented by workflow inputs rather than hidden.

## Guardrails

- Do not add cloud credentials, account IDs, Terraform state, local secret files, or provider tokens.
- Do not create, update, or rely on cloud resources for regular testing.
- Keep commits squashed into larger coherent changes when updating the PR.
