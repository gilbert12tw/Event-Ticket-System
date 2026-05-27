# DIP: Dependency Inversion Principle

## Intent

Stable high-level policy should not depend on volatile low-level details. Both should depend on
abstractions, and those abstractions should be shaped by the stable policy or client need.

DIP is about dependency direction, not merely dependency injection. Passing a concrete PostgreSQL,
Redis, SMTP, S3-compatible object-store, HR, or SSO client into domain logic still couples policy to
detail even if the client is injected.

## Project Interpretation

The ticketing domain should depend on contracts, protocols, and pure data models. Adapters for
PostgreSQL, Redis, MinIO, mail, HR, SSO, filesystems, and queues should depend inward on those
contracts.

Use DIP where volatility is real: HR and SSO providers, mail delivery, object storage, message
publishing, deployment targets, reporting projections, and idempotency persistence. Keep simple pure
functions simple; do not add ports around stable local calculations without a reason.

## Apply

- Put database, Redis, object-storage, mail, HR, SSO, filesystem, and queue details behind adapter
  boundaries.
- Let repositories convert storage rows into domain types before domain decisions run.
- Let HR and SSO adapters convert provider output into eligibility and identity contracts.
- Let workers depend on idempotency and store abstractions before invoking side effects.
- Keep abstractions narrow and behaviorally specified enough for LSP tests.

## Avoid

- Importing database drivers, Redis clients, SMTP clients, object-storage SDKs, or provider-specific
  SSO/HR clients from domain modules.
- Letting high-level booking, ticket, check-in, reporting, or audit rules know concrete adapter
  classes.
- Treating dependency injection as sufficient while still depending on provider-specific types.
- Creating abstractions owned by low-level details that leak provider language into the domain.
- Wrapping every helper in an interface when there is no meaningful variation or test need.

## Event Ticketing Examples

- Registration services can orchestrate repository ports, but storage rows must enter as project
  contracts before booking decisions run.
- Notification workers should depend on delivery and idempotency ports; ticketing rules should not
  import SMTP or provider clients.
- A future queue implementation should depend on outbox event contracts; outbox contracts should not
  depend on queue client types.
- A future reporting store should depend on reporting projection contracts, not force booking or
  audit contracts to contain store-specific attributes.

## Relationships

- DIP helps achieve OCP by allowing new low-level implementations to be added without changing stable
  policy clients.
- ISP guides DIP by keeping abstractions client-specific instead of broad provider mirrors.
- LSP guides DIP by requiring every implementation behind an abstraction to honor the same behavior.
- SRP guides where the abstraction belongs: near the actor or policy whose reason to change should be
  protected.

## Sources

- [Robert C. Martin, Dependency Inversion Principle](https://objectmentor.com/resources/articles/dip.pdf)
- [SOLID overview](../solid.md)
