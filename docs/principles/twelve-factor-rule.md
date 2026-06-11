# Twelve-Factor Rule

Use these rules when adding runtime behavior, deployment code, workers, storage adapters,
notification adapters, or operational tooling for the event ticketing system.

The baseline source is [The Twelve-Factor App](https://12factor.net/). This document narrows those
principles to this repository's Go app, React build, Docker Compose services, privacy, and
deterministic-test constraints.

## Project Rule

Runtime code must be portable, explicitly configured, stateless unless state is held in an attached
resource, and safe to run in local tests without real SSO, HR, SMTP, object-storage credentials, or
network access beyond configured local backing services.

## Factors

1. **Codebase**
   Keep required behavior in versioned project files. Do not rely on untracked notebooks, local
   scripts, shell history, manually exported schemas, or machine-local model state for the app to run.

2. **Dependencies**
   Declare Go, Node, and frontend dependencies in project metadata or lock files. Do not depend on
   globally installed packages, developer-specific tool state, or implicit SDK availability.

3. **Config**
   Store deploy-specific values in environment variables or typed settings. Do not hard-code
   database URLs, Redis URLs, object-storage buckets, mail hosts, SSO/HR endpoints, ticket signing
   secrets, capacity thresholds, or retention periods in domain logic.

4. **Backing services**
   Treat PostgreSQL, Redis, MinIO/object storage, mail, HR, SSO, and queue/outbox resources as
   attached services behind adapters. Domain modules should accept contracts and ports, not concrete
   clients.

5. **Build, release, run**
   Keep build steps, release configuration, and runtime execution separate. Do not mutate schemas,
   create buckets, seed data, or run migrations at package initialization time.

6. **Processes**
   Keep API, worker, projection, compensation, and cleanup processes stateless. Persist booking,
   ticket, check-in, idempotency, report export, notification, and audit state in explicit stores.

7. **Port binding**
   Services must expose explicit ports or handler entrypoints. Avoid hidden background daemons or
   import-time listeners. Containerized HTTP services should preserve the documented health,
   readiness, and API routes.

8. **Concurrency**
   Scale by process type. API, notification workers, reporting projections, compensation, and cleanup
   should remain independently runnable and horizontally replaceable where practical.

9. **Disposability**
   Start fast, stop safely, and handle interrupted processing through idempotent retries. Workers must
   tolerate duplicate outbox events and must not leave half-applied notification, export, booking,
   ticket, check-in, or audit state.

10. **Dev/prod parity**
    Keep local, test, staging, and production flows aligned through the same contracts and adapters.
    Tests should use deterministic fakes and local backing services rather than weakening production
    interfaces.

11. **Logs**
    Write operational facts to stdout/stderr or structured log sinks. Do not hide important state in
    local files, ad hoc reports, or side-channel printouts that production cannot collect.

12. **Admin processes**
    Run migrations, seeds, report rebuilds, retention cleanup, benchmark generation, and one-off
    repair tasks as explicit admin commands. They must share project settings and contracts with the
    app.

For a factor-by-factor map of where the current codebase implements these rules, see
[`twelve-factor-compliance.md`](./twelve-factor-compliance.md).

## Review Checklist

- New runtime config is represented in typed settings or environment handling.
- External systems remain behind adapters and are replaceable in tests.
- Startup has no network calls, schema mutations, bucket creation, or model downloads unless it is an
  explicit run step.
- Workers are idempotent and can be killed between events without corrupting booking, ticket,
  notification, report export, check-in, privacy, or audit state.
- Tests remain deterministic and do not require real SSO, HR, SMTP, object-storage credentials, or
  external network access.
