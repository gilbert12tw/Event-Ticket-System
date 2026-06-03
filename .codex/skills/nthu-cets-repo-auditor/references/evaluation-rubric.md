# NTHU CETS Evaluation Rubric

This reference preserves the grading basis extracted from the final report announcement for the Corporate Event Ticketing System topic. Do not require the original PDF during future audits.

## Topic

Corporate Event Ticketing System.

Target users:

- Employees browse activities, check details, and apply for or reserve tickets.
- Welfare committee or activity admins create, publish, govern, and review activities; configure rules such as location limits and capacity limits; monitor ticketing status.
- HR observes which activities exist and how many employees participate.

Required system capabilities:

- Activity management module: create, edit, and configure event rules.
- Employee ticketing module: employee-facing activity list, booking or application flow, and eligibility/capacity checks.
- Ticket management and verification: admin review, e-ticket generation, and on-site fast verification.
- Authentication/role handling: at least employee, activity admin, and HR/check-in/admin-style roles are distinguishable.

Advanced review concerns:

- Performance: hot event booking pressure.
- Service reliability: inability to browse, book, or query tickets must be handled.
- Service correctness: no oversell and no invalid or ineligible bookings.

## Official Weighting

| Weight | Criterion | Strict interpretation for repo audit |
|---:|---|---|
| 30% | 需求轉換與實作 | User stories, acceptance criteria, demo flow, and implemented use cases must align. A feature only described in slides/docs but not executable in UI/API is weak evidence. |
| 25% | 架構設計與可擴展性 | Must include concrete architecture diagrams, system architecture, sequence diagrams, ER model, scaling rationale, and bottleneck handling. High-level boxes alone are insufficient. |
| 25% | 系統測試與驗證 | Must show unit, integration, E2E, load/stress/performance evidence, plus results. Happy-path-only tests are insufficient for correctness-heavy ticketing. |
| 10% | 程式碼品質 | Readability, consistency, modularity, maintainability, security, version control, and objective evidence such as SonarScanner or SonarQube reports. |
| 10% | 運維與可靠性 | Monitoring dashboard, metrics selected, reliability mechanisms, failure handling, health/readiness, logs, and operations story. |

## Strict Pass Bar

A strong repo should make these claims provable without manual database edits:

- Admin can create/publish/manage activities and eligibility/capacity rules.
- Employee can browse eligible events, book, cancel where allowed, receive ticket state, and see clear errors.
- Ticket/check-in enforces one successful redemption per ticket.
- PostgreSQL transactions or unique constraints are final truth for capacity, bookings, tickets, check-ins, and audit.
- Booking and check-in retry behavior is idempotent.
- Ineligible, duplicate, oversell, expired/revoked ticket, unauthorized role, and outage scenarios are tested.
- k6 or equivalent evidence covers hot-event pressure and performance thresholds.
- Playwright or equivalent evidence covers role workflows and responsive UI.
- Monitoring evidence includes request rate, errors, latency, DB lock/pool pressure, queue/outbox lag, check-in success/conflict, and resource use.
- Logs and reports avoid full PII, provider tokens, signed ticket tokens, QR payloads, and secrets.

## Presentation Evidence To Prefer

The announcement emphasized visuals and concise storytelling. Prefer proof objects that can go directly into the final report:

- Architecture diagram, sequence diagram, and ER model.
- Demo path that maps to the core user stories.
- Test result matrix grouped by unit, integration, E2E, k6/load, and reliability/failure tests.
- Sonar or code-quality snapshot with interpretation, not just a screenshot.
- Monitoring dashboard screenshot or metrics table explaining why each metric matters.
- Short risk table for oversell, ineligible booking, duplicate check-in, and service outage.
