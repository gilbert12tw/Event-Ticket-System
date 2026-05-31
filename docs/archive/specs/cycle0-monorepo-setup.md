# Cycle 0 Monorepo Setup

## Summary

Establish the repository-level monorepo foundation so apps, services, and shared packages can be managed, built, tested, and linted through one package manager and task runner.

## Acceptance Criteria

- AC-1: Given a blank checkout, when `pnpm install` runs, then all workspace packages resolve through one root lockfile without duplicate package-manager installs.
- AC-2: Given one package changes, when `pnpm build` runs, then Turborepo rebuilds only changed packages and packages affected by their dependency graph.
- AC-3: Given any service or app workspace, when `pnpm test` runs at the root, then all workspace unit tests run and Turborepo prints a task summary.
- AC-4: Given the root `package.json`, when `pnpm lint` runs, then all workspace packages use the shared root ESLint configuration.

## Implementation Scope

- Add `pnpm-workspace.yaml` with only `apps/*`, `services/*`, and `packages/*`.
- Add `turbo.json` with build, test, lint, and dev tasks, local cache settings, and remote cache readiness.
- Add root `package.json` scripts for `dev`, `build`, `test`, `lint`, and `format`.
- Add `.npmrc` with `shamefully-hoist=false` and strict peer dependency checks.
- Update ignore files for Node dependencies, build output, Turbo cache, and environment files.
- Keep Go service code under `services/api`; pnpm workspaces own JavaScript package orchestration.

## Non-Functional Requirements

| Category | Requirement |
| --- | --- |
| Build speed | `pnpm build` cold start should remain under 60 seconds locally for the current workspace set. |
| Incremental | Unchanged package builds should be restored from Turborepo cache. |
| CI compatibility | Turbo local cache is stored under `.turbo/cache` so GitHub Actions cache can persist it; remote cache credentials stay outside source control. |

## Verification

- `pnpm install`
- `pnpm lint`
- `pnpm test`
- `pnpm build` twice to verify cache behavior
- `docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config`
