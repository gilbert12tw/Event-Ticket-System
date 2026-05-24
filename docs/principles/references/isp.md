# ISP: Interface Segregation Principle

## Intent

Clients should not depend on interfaces they do not use. Interfaces should be cohesive and shaped
around the client that consumes them, not around the full capability set of an implementation.

The broader rule is that software at any level should not depend on things it does not need, because
unused dependency surface creates unexpected maintenance pressure.

## Project Interpretation

Keep ticketing ports and protocols narrow. A component that only needs to write report artifacts
should not depend on a full storage, listing, deletion, retention, and audit interface. A domain
policy that only needs an employee eligibility snapshot should not depend on a complete HR SDK
client.

Narrow interfaces protect privacy and reliability. They also make tests deterministic because fakes
only need to implement the behavior the client actually uses.

## Apply

- Define client-specific protocols for booking persistence, ticket signing, object storage, message
  publishing, notification delivery, HR lookup, SSO identity, and reporting projections.
- Split read, write, delete, list, and audit capabilities when clients do not need all of them.
- Keep domain modules dependent on contract-shaped inputs rather than SDK clients.
- Prefer small fakes that mirror a narrow port over broad mocks of cloud SDKs.
- Review shared helper classes for unused methods before adding new responsibilities.

## Avoid

- Passing a PostgreSQL, Redis, SMTP, S3-compatible object-store, SSO, or HR client into domain code
  when only one operation is required.
- Creating one "ticketing service" interface that includes events, eligibility, booking, tickets,
  check-in, notification, reporting, audit, privacy, and metrics.
- Forcing all adapters to implement methods that only one runtime path needs.
- Adding no-op methods to satisfy an oversized interface.
- Letting one client's new method force unrelated clients to update tests or implementations.

## Event Ticketing Examples

- A booking service should receive only the persistence and idempotency capabilities it needs, not a
  general database handle.
- A report export process may need object-storage write capabilities that booking and check-in paths
  should not see.
- A message publisher should not expose object storage, mail delivery, or idempotency operations.
- Eligibility decisions should consume project eligibility records, not an HR SDK client with
  unrelated employee-management methods.

## Relationships

- ISP reflects SRP because each interface should serve one client responsibility.
- ISP guides DIP by making abstractions smaller, stable, and client-owned.
- ISP supports LSP because narrow interfaces reduce the chance that implementations need invalid
  no-op behavior.

## Sources

- [Robert C. Martin, Interface Segregation Principle](https://objectmentor.com/resources/articles/isp.pdf)
- [SOLID overview](../solid.md)
