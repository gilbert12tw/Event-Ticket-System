# Local Environment Rules

Local development uses `services/api/deploy/compose.yaml` as the entry point for the Go app plus backing services. It is not the production topology.

## Backing Services

Required Phase 1 backing services:

- PostgreSQL: source of truth for bookings, tickets, check-in records, and audit logs.
- Redis: reservation gate, idempotency keys, and short-TTL cache. Redis is not the final transaction truth.
- MinIO: local object storage for report export artifacts and admin-managed event posters in Phase 1 production. Event attachments and ticket files require a separate asset spec before they become production scope.
- Mailhog: local mock email provider to prevent accidental real email delivery.

`services/api/deploy/.env.example` contains committable example settings. `services/api/deploy/.env` is local override state and must not be committed. If a local port conflicts, override it with environment variables instead of hardcoding ports in code.

## 12-Factor Rules

- Codebase: use one repository; local, staging, and production differ only by config.
- Dependencies: declare dependencies in package manifests and lockfiles.
- Config: use environment variables for deploy-specific values.
- Backing services: inject PostgreSQL, Redis, object storage, queue, mail, HR, and SSO through URLs or credentials.
- Build / Release / Run: build artifacts or images first; run starts processes without rebuilding.
- Processes: app and worker processes must be stateless.
- Port binding: read the HTTP port from env and let the app start its own listener.
- Concurrency: scale app and worker processes horizontally; use Compose to simulate this in Phase 1.
- Disposability: start quickly and shut down gracefully.
- Dev / prod parity: use PostgreSQL, Redis, object storage, and mail mock locally.
- Logs: write structured logs to stdout / stderr; do not write local log files or full PII.
- Admin processes: run migrations, seeds, lottery batches, and data repairs as one-off commands.

## Verification Checklist

Check at least the following before delivery when relevant:

- `docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config`
- Local backing services start, and PostgreSQL / Redis healthchecks pass when required for the task.
- Core flow tests cover event creation, eligibility checks, booking, oversell prevention, ticket generation, check-in, and reporting.
- Negative tests cover ineligible users, duplicate booking, full capacity, duplicate check-in, notification failure, queue backlog, Redis failure, and DB failure when relevant.
- Logs do not contain full PII or secrets.
- Phase 1 docs do not claim that microservices, Kafka, Kubernetes, or cross-region HA are already complete.
