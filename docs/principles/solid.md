# SOLID Principle Rules

Use these rules when changing module boundaries, adapters, domain services, contracts, or extension
points. SOLID is not a request to add abstractions everywhere. In this project, it is a change-control
tool: isolate likely reasons to change while keeping ticketing contracts stable.

## Principle Map

- **SRP -> OCP**: OCP depends on SRP. A module can only be closed against a change when unrelated
  reasons to change are already separated.
- **ISP -> SRP and DIP**: ISP applies SRP to interfaces and guides DIP by making abstractions
  client-specific.
- **DIP -> OCP**: DIP helps achieve OCP by making stable policy depend on ports or protocols instead
  of volatile adapters.
- **LSP -> OCP and DIP**: LSP reflects OCP because substitutable implementations let clients remain
  unchanged. It guides DIP by requiring abstractions to describe behavior, not just method names.

## Reference Files

- [SRP: Single Responsibility Principle](references/srp.md)
- [OCP: Open-Closed Principle](references/ocp.md)
- [LSP: Liskov Substitution Principle](references/lsp.md)
- [ISP: Interface Segregation Principle](references/isp.md)
- [DIP: Dependency Inversion Principle](references/dip.md)

## Project-Wide Guidance

- Prefer small cohesive modules around one actor or stakeholder group: Event, Eligibility,
  Registration, Ticket, Check-in, Notification, Reporting, and Audit.
- Add extension points only at proven or highly probable variation points, such as allocation
  strategies, HR/SSO providers, notification delivery, report export storage, outbox workers,
  idempotency stores, and reporting projections.
- Keep domain logic pure and adapter-agnostic. If a module needs PostgreSQL, Redis, MinIO, SMTP,
  HR, SSO, filesystem, or time, pass those capabilities through a narrow boundary.
- Treat Go domain types, API contracts, and runtime schemas as client-facing promises. Changes must
  preserve behavior or include explicit migration and compatibility handling.
- Avoid broad service objects that mix event state, eligibility, booking, tickets, check-in,
  notifications, reporting, audit, privacy, persistence, and deployment concerns.

## Sources

- [Robert C. Martin, Single Responsibility Principle](https://blog.cleancoder.com/uncle-bob/2014/05/08/SingleReponsibilityPrinciple.html)
- [Robert C. Martin, Open-Closed Principle](https://www.cs.utexas.edu/~downing/papers/OCP-1996.pdf)
- [Robert C. Martin, Liskov Substitution Principle](https://objectmentor.com/resources/articles/lsp.pdf)
- [Robert C. Martin, Interface Segregation Principle](https://objectmentor.com/resources/articles/isp.pdf)
- [Robert C. Martin, Dependency Inversion Principle](https://objectmentor.com/resources/articles/dip.pdf)
- [Barbara Liskov and Jeannette Wing, Behavioral Subtyping](https://www.cs.cmu.edu/afs/cs/project/calder/www/fmdp.html)
