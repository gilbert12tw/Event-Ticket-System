# Architecture Rules

Phase 1 uses Docker Compose backing services and a modular monolith to deliver a demonstrable, testable ticketing flow. Module boundaries live inside one application; they are not independently deployed services.

## Modules

| Module | Responsibility |
| --- | --- |
| Auth & RBAC | Enterprise SSO token validation, role authorization, and sensitive action controls. |
| Event | Event creation, editing, state machine, booking windows, and attachments. |
| Eligibility | HR attributes, eligibility rules, region, department, grade restrictions, and rejection reasons. |
| Registration | Booking, cancellation, waitlist, first-come-first-served allocation, lottery, and allocation policies. |
| Ticket | Ticket state, QR codes, signed ticket tokens, and token verification. |
| Check-in | Online redemption, duplicate scan protection, offline sync boundary, and conflict handling. |
| Notification | Email or in-app notifications, templates, retry, and failure records. |
| Reporting | Participation metrics, dashboards, exports, and read models. |
| Admin / Audit | System parameters, sensitive changes, and immutable audit log queries. |

## Layering

- Controllers handle input/output and authorization only.
- Application services coordinate use cases.
- Domain code owns business rules.
- Repositories and adapters wrap PostgreSQL, Redis, object storage, queue, mail, HR, and SSO dependencies.
- Keep module-owned rules inside the owning module; do not reach across module boundaries for internal data or helper functions.

## Evolution Policy

Phase 2 defaults to **process-first scale hardening on the Phase 1 modular monolith**:

- Same Go binary; scale by splitting processes, not services. `app` + same-binary worker processes selected by env (`WORKER_KINDS=notification,projection,compensation,reservation_compensation`).
- Redis is a booking pre-admission gate only; PostgreSQL remains the final truth for booking, ticket, check-in, and audit state.
- Reporting projections are derived, disposable, and rebuildable from the outbox; they must never be used as booking, eligibility, ticket redemption, check-in, authorization, or audit truth.
- Extracting Registration, Notification, or Reporting into a separately deployed service is a **deferred decision-gate** and requires baseline evidence per `docs/specs/phase2-scale-hardening.md` §2 / §4. It is not a Phase 2 default deliverable.
- Kafka, Kubernetes, service mesh, cross-region HA, and full microservices are likewise deferred — **not Phase 2 deliverables**. The docs guard (`TestPhase2DocsDoNotClaimDeferredInfraIsRequired`) will fail if Phase 2 docs claim any of these are required, used, has, implemented, shipped, or complete.

Phase 3 handles high availability, multi-AZ deployment, DB failover, partitioning, independent Check-in scaling, container platform (Kubernetes or equivalent), and other operational maturity beyond Phase 2 scope.

Before promoting a deferred decision-gate topic to an implementation issue, answer:

- Which module is the bottleneck, and what specific Phase 2 baseline metrics or load tests prove it (not generic latency spikes)?
- Why is the bottleneck not addressable by process-first scaling (more `app` / worker processes, kind split, pre-admission gate tuning, projection rebuild)?
- What are the consistency, idempotency, tracing, retry, and compensation strategies after the change?
- Is the added operational cost lower than the risk it resolves?
