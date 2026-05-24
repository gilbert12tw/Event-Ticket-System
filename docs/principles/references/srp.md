# SRP: Single Responsibility Principle

## Intent

A software component should have one responsibility, understood as one coherent reason to change.
That reason should come from one actor, customer group, or business stakeholder group.

In practical terms: a component should not touch work that belongs to another role. If two change
requests would come from different stakeholder groups, separate the code before the requests collide.

## Project Interpretation

In this repository, SRP separates the forces that change the system:

- contract and schema compatibility;
- event state, eligibility, registration, allocation, and waitlist policy;
- ticket signing, QR redemption, check-in conflicts, privacy, RBAC, and retention;
- notification delivery, report export, outbox idempotency, and retry behavior;
- runtime API, same-binary worker orchestration, and deployment configuration.

A module can coordinate multiple components, but it should not own the rules for unrelated
stakeholders. For example, orchestration may call eligibility, booking, ticketing, and notification
logic, but it should not redefine their policies inline.

## Apply

- Keep request validation in controller/API code and domain decisions in domain modules.
- Keep PostgreSQL, Redis, object stores, mail, HR, and SSO behind adapters.
- Keep booking and check-in decisions separate from notification delivery or report export.
- Keep metrics and reports from becoming hidden sources of domain truth.
- Split a module when separate stakeholders would request independent changes.

## Avoid

- Mixing schema evolution, booking policy, persistence, notification delivery, and privacy decisions
  in one class.
- Letting convenience helpers grow into shared objects with unrelated reasons to change.
- Encoding outbox or provider assumptions inside booking, ticket, or check-in logic.
- Adding provider-specific fields to ticketing, reporting, or privacy workflows.
- Splitting so aggressively that a single business rule becomes scattered across many files.

## Event Ticketing Examples

- API contract code should own request and response shape, not booking transaction policy.
- Registration code should own booking, waitlist, capacity, and allocation rules, not email
  rendering.
- Ticket code should own signing and verification, not event creation or report export.
- Check-in code should own duplicate redemption and conflict handling, not notification retry.
- Worker code should own outbox consumption and idempotent side effects, not domain authorization
  rules.

## Relationships

- SRP enables OCP: once reasons to change are separated, stable clients can remain closed while one
  extension point changes.
- ISP reflects SRP at interface boundaries by separating client-specific needs.
- DIP relies on SRP because abstractions should represent cohesive policy, not a bundle of unrelated
  implementation details.

## Sources

- [Robert C. Martin, Single Responsibility Principle](https://blog.cleancoder.com/uncle-bob/2014/05/08/SingleReponsibilityPrinciple.html)
- [SOLID overview](../solid.md)
