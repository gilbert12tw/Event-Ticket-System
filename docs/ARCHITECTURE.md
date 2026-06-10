# Architecture

## Current Shape

CETS is a Go modular monolith with a React SPA. Phase 1 runs through Docker
Compose with PostgreSQL, Redis, MinIO, and Mailhog as backing services. The app,
worker, migration, seed, and admin commands use the same Go command binary.

The system is organized by product modules:

- Auth and RBAC
- Event management
- Eligibility
- Registration and allocation
- Ticketing
- Check-in
- Notification
- Reporting
- Audit and admin operations

Controllers own HTTP input/output and authorization. Application services
coordinate use cases. Domain code owns business rules. Repositories and adapters
wrap PostgreSQL, Redis, object storage, mail, HR, and SSO/provider dependencies.

## Consistency Rules

- PostgreSQL is the final truth for committed bookings, tickets, check-ins,
  reports, and audit records.
- Booking, cancellation, ticket generation, notification delivery, and check-in
  sync must be idempotent or deduplicated.
- Limited-capacity booking must prevent oversell with PostgreSQL transactions,
  row locks, unique constraints, or equivalent database guarantees.
- Eligibility and event state are rechecked during final booking, not only in
  cached event lists.
- One ticket may be redeemed successfully only once. Database constraints are
  the final guarantee.
- Redis reservation, reporting projections, and worker queues are optimization
  or read/side-effect boundaries; they never replace committed database truth.

## Runtime And Deployment

Base Compose services are `app`, `worker`, `postgres`, `redis`, `minio`, and
`mailhog`. The app serves the API and production SPA. The worker consumes
PostgreSQL outbox work and can be split by `WORKER_KINDS` into same-binary
processes such as `notification`, `projection`, `compensation`, and `export`.

Local observability profiles may run Prometheus, Grafana, Loki, Tempo, or related
tools for review and debugging. These are optional local/demo services, not
additional product truth stores.

The bare-metal Kubernetes assets under `infra/k8s/baremetal/` deploy the same Go
modular monolith and worker model. They do not turn the application into
microservices.

## Evolution Boundary

Phase 2 is process-first scale hardening on the current monolith: worker kind
isolation, Redis pre-admission, reporting projections, and stronger operations
visibility. Phase 3 may simulate or evaluate higher availability and deeper
observability, but production cross-region HA, Kafka, service mesh, and full
microservices remain deferred decision gates.

Before promoting a deferred topic, provide evidence that the bottleneck cannot
be solved by process count, worker kind isolation, PostgreSQL tuning, Redis
pre-admission tuning, projection rebuild, or clearer operational controls.

Detailed current evolution constraints live in
`docs/specs/evolution-boundaries.md`.

## Documentation And Contracts

- API contract: `docs/openapi.yaml` and `docs/openapi/`.
- Product contract: `docs/PRODUCT.md` and `docs/specs/phase1-product-requirements.md`.
- Production acceptance: `docs/specs/phase1-production-upper-bound.md`.
- Capacity targets: `docs/specs/phase1-nfr-and-capacity.md`.
- Diagrams: `docs/diagrams/`.
