# Code Space Reduction

## Summary

Shrink tracked source while preserving the Corporate Event Ticketing System behavior. Prefer deleting
dead code, generated artifacts, duplicated helpers, and broad unused abstractions before changing
runtime flows.

## Acceptance Criteria

- Given a clean checkout, when source lines are counted excluding generated build output, dependency
  caches, and lockfiles, then tracked code moves toward fewer than 10,000 lines.
- Given production Docker builds, when the web app is built, then generated SPA assets are still
  embedded into the Go binary through the existing Vite output contract.
- Given local development, when the frontend runs through Vite or Compose dev overlay, then API,
  health, and ready proxies still work without committed build assets.
- Given ticketing correctness paths, when booking, ticket generation, notification, reporting,
  check-in, and audit code is reduced, then PostgreSQL remains the source of truth and idempotency
  behavior is preserved.
- Given UI code is reduced, when role routes render, then employee, admin, check-in, HR, audit, and
  demo flows keep their route contracts and labels.

## Non-Goals

- No feature removal.
- No microservice split, Kafka, Kubernetes promotion, or new runtime dependency.
- No minifying hand-written source just to manipulate line counts.

## Verification

- Count tracked production and test code after each slice.
- Run `git diff --check` for touched files.
- Run focused tests for touched modules; if local filesystem reads hang, record the exact blocked
  command and continue with narrower checks.
- Before push, rerun the project-required `act push` gate.
