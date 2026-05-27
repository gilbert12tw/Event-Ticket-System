# OCP: Open-Closed Principle

## Intent

Software entities should be open to extension and closed to modification. The practical goal is to
add or change behavior at planned extension points without rewriting stable clients or the main flow.

OCP is strategic, not universal. The design should predict likely complexity and protect those
variation points with appropriate abstractions. It should not prebuild extension points for every
imagined future.

## Project Interpretation

Close the ticketing core against changes in allocation policies, HR/SSO providers, backing stores,
message transports, reporting stores, and deployment targets. Keep the main contracts and domain flow
stable while allowing new adapters or policies to be added.

Organize dependencies by layer so stable domain code is not forced to change when a lower-level
implementation changes. A new object-storage adapter, mail provider, queue publisher, or reporting
store should not require booking, ticket, check-in, privacy, or audit rules to change.

## Apply

- Define extension points around real variation: allocation strategies, HR/SSO adapters, object
  stores, notification delivery, message buses, reporting projections, retention policy, and
  idempotency stores.
- Extend behavior by adding new implementations that emit existing project contracts such as booking,
  ticket, eligibility, check-in, notification, or report records.
- Keep stable flows free of `if provider == ...` branches that must be edited for every new vendor.
- Use data-driven policy where it isolates the real variation, such as configured zones or thresholds.
- Require tests that prove existing clients still work when a new implementation is substituted.

## Avoid

- Adding abstraction layers before a concrete variation point exists or is highly probable.
- Modifying central orchestration for every new allocation policy, HR/SSO provider, mail provider,
  object store, or reporting store.
- Encoding provider-specific payload shapes in domain logic.
- Treating OCP as "never edit code"; bug fixes and changed requirements still require edits.
- Allowing extension points to bypass validation, privacy, idempotency, or review-required behavior.

## Event Ticketing Examples

- A lottery or first-come-first-served allocation strategy should satisfy the same allocation
  contract; controllers and repositories should not branch on the strategy name.
- An enterprise SSO or mock SSO adapter should output the same project identity contract.
- A real object-storage adapter should use existing report export contracts instead of changing
  reporting rules.
- New eligibility dimensions should come from contract-compatible policy data, not hard-coded
  branches inside booking.

## Relationships

- OCP depends on SRP because unrelated reasons to change must be separated before one area can be
  extended independently.
- DIP helps implement OCP by making stable policy depend on abstractions.
- LSP reflects OCP by requiring implementations behind an extension point to be substitutable without
  client changes.

## Sources

- [Robert C. Martin, Open-Closed Principle](https://www.cs.utexas.edu/~downing/papers/OCP-1996.pdf)
- [SOLID overview](../solid.md)
