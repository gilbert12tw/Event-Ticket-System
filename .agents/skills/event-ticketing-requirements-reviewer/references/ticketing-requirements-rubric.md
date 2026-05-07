# Corporate Event Ticketing System Rubric

Use this reference to review requirements, architecture drafts, project proposals, or homework submissions for the Corporate Event Ticketing System case.

## Background

Large companies often run welfare events such as performances, talks, family days, and exhibitions. Ticket management, allocation, and fairness are common operational pain points for welfare committees.

The project goal is to build a modern, automated, cloud-native ticketing system that simplifies welfare committee operations, improves employee participation, and keeps ticket allocation fair and transparent.

## Target Audience

- Employees: internal employees who browse events, inspect event details, apply for or book tickets, receive tickets, and use them at the venue.
- Welfare committee members or activity admins: users who plan, publish, manage, and govern events, including location restrictions, capacity limits, application review, and ticket status monitoring.
- HR or reporting users: users who need visibility into available activities and employee participation without unnecessary access to personal data.

## System Requirements

Review whether the draft covers these baseline modules:

- Event management module: allows admins to create, edit, publish, and configure event rules, including location restrictions, ticket limits, booking periods, event information, event state, and visibility.
- Employee booking module: provides a clear employee experience to browse activities, inspect details, apply/book tickets, and check eligibility and remaining capacity at the right moments.
- Ticket management and check-in module: supports application review or allocation, electronic ticket generation, and fast on-site ticket redemption.
- Authentication and supporting services: classifies at least three roles: employee, welfare committee/activity admin, and HR or system admin.

## Advanced Requirements

Review whether the draft explicitly handles these risks:

- Performance: hot events may create burst traffic, queues, inventory contention, retries, or latency spikes.
- Reliability: the system may fail to book, verify, query, notify, sync HR data, access the database, or access the cache.
- Correctness: the system must prevent overselling, ineligible booking, duplicate check-in, forged tickets, review mistakes, and fairness disputes.

## Weighted Evaluation Criteria

Use 100 points by default.

### 30 points: Requirements Conversion and Implementation Readiness

Award points for:

- Clear user stories for each target audience.
- Concrete demo flows, including create event, configure rules, browse event, book/apply ticket, approve or allocate, generate ticket, check in, and view reports.
- Requirements that define input, output, states, permissions, and failure behavior.
- Explicit eligibility rules, region restrictions, ticket limits, family/dependent rules, first-come-first-served or lottery policy, waitlist behavior, and cancellation behavior.
- Clear mapping from requirements to implementation modules.

Deduct points for:

- Only describing abstract goals without user stories or workflows.
- Missing one or more core audiences.
- Missing ticket allocation fairness rules.
- Missing state transitions for events, applications, tickets, or check-in records.
- Demo scenarios that cannot prove the critical flows work.

### 10 points: Code Quality and Security Readiness

Award points for:

- Clear modular boundaries and readable responsibility separation.
- Authentication and authorization requirements, preferably enterprise SSO plus RBAC.
- Security controls for PII, audit logs, QR code integrity, rate limiting, and sensitive admin actions.
- Version control and implementation hygiene expectations.
- Mention of avoiding obvious security holes such as forged tickets, leaked employee data, unauthorized role actions, or insecure logs.

Deduct points for:

- Role checks only described at UI level.
- No auditability for admin or ticket actions.
- No QR code anti-forgery or token expiration design.
- No mention of PII protection.
- Technology lists without maintainability or security rationale.

### 25 points: Architecture Design and Scalability

Award points for:

- Architecture that supports hot-event booking spikes and normal-day browsing separately.
- Clear plan to reduce response time per request under peak load.
- Scalable handling of booking inventory, eligibility checks, queues, caches, databases, read replicas, async jobs, and notification workloads.
- Global or multi-site deployment considerations when relevant, including latency, region/site constraints, and failover.
- Tradeoffs appropriate to project phase; avoid premature complexity for low scale, but address peak correctness risks.

Deduct points for:

- No usage estimates or peak traffic assumptions.
- No strategy for oversell prevention under concurrent booking.
- No distinction between synchronous booking confirmation and asynchronous notification or ticket generation.
- Claiming queues solve correctness without idempotency, transactions, or compensation.
- No scaling or failover story for check-in, eligibility, or reporting.

### 25 points: System Testing and Verification

Award points for:

- Unit tests for domain rules such as eligibility, ticket limits, allocation, lottery, cancellation, and QR token validation.
- Integration tests for SSO/HR sync, booking, ticket generation, notification, and reporting.
- End-to-end tests covering employee booking, admin event setup, approval, check-in, and HR reporting.
- Load, stress, and performance tests for popular-event booking and on-site check-in.
- Failure tests for duplicate booking, duplicate scan, service timeout, database failure, cache failure, queue backlog, and notification failure.
- Clear acceptance criteria and traceability from requirements to tests.

Deduct points for:

- Only saying "unit test and e2e test" without scenarios.
- No performance or stability test for hot-event booking.
- No negative tests for ineligible user, oversell, duplicate booking, or duplicate check-in.
- No test data strategy for HR attributes, locations, departments, or eligibility changes.

### 10 points: Operations and Reliability

Award points for:

- Stability monitoring for API latency, error rate, booking success/failure, check-in failure, queue lag, remaining ticket anomalies, and notification failures.
- Fault handling for unavailable booking, verification, query, HR sync, notification, database, cache, or queue services.
- Alerting, logs, tracing, health checks, rollback, backup, and disaster recovery where appropriate.
- Graceful degradation, such as read-only activity browsing or offline ticket display/check-in with conflict resolution.

Deduct points for:

- No monitoring or alerting plan.
- No incident behavior for failed check-in or failed booking.
- No rollback or recovery story.
- No audit trail or operational visibility for fairness disputes.

## Required Reviewer Checks

Always check:

- Can an employee complete the full path from discovering an activity to receiving and using a ticket?
- Can an admin create and govern an event without manual database changes?
- Can HR see participation results without accessing unnecessary personal data?
- Is eligibility evaluated at the correct moments, especially before final ticket allocation?
- Can the system prevent overselling under concurrent requests?
- Can the system explain why a user is not eligible or why booking failed?
- Can duplicate booking, duplicate ticket generation, and duplicate check-in be detected?
- Can a popular event be handled without the entire service becoming unavailable?
- Can failures be observed, retried, or compensated safely?
- Are testing scenarios concrete enough to verify the promises?
