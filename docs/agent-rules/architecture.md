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

Phase 2 may split Registration, Notification, Reporting, or other hot paths only after load tests or real usage show a focused bottleneck. Phase 2 may evaluate managed queues or Kafka.

Phase 3 handles high availability, multi-AZ deployment, DB failover, partitioning, independent Check-in scaling, Kubernetes, or an equivalent container platform.

Before introducing larger architecture, answer:

- Which module is the bottleneck, and what metrics or load tests prove it?
- What are the consistency, idempotency, tracing, retry, and compensation strategies after the split?
- Is the added operational cost lower than the risk it resolves?
