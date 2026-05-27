# LSP: Liskov Substitution Principle

## Intent

Implementations behind the same abstraction must be substitutable without surprising their clients.
Changing the implementation must preserve the behavior promised by the interface, not merely the
method names or type signatures.

LSP is client-centered: the question is whether users of the abstraction can keep their assumptions
and tests unchanged when a different subtype, adapter, or implementation is supplied.

## Project Interpretation

Every adapter or domain implementation must honor the contract consumed by its client. Repositories,
identity providers, HR adapters, stores, message buses, and cleanup jobs may vary internally, but
they must preserve the externally promised behavior, validation, idempotency, and error semantics.

For API models and domain contracts, schema compatibility is not enough. Field meaning, state
transitions, capacity rules, idempotency behavior, privacy decisions, and audit behavior must remain
stable unless the contract is explicitly versioned.

## Apply

- Preserve preconditions: do not require stricter input than the abstraction promises.
- Preserve postconditions: return the expected contract, normalized values, and status semantics.
- Preserve invariants: keep capacity, eligibility, ticket redemption, privacy, idempotency, and
  retention guarantees intact.
- Make adapter fakes and real adapters pass the same behavioral tests where practical.
- Version contracts when substitutability cannot be preserved.

## Avoid

- Returning provider-specific partial objects where project booking, ticket, check-in, or eligibility
  contracts are promised.
- Adding implementations that silently skip duplicate booking, duplicate check-in, privacy, or audit
  behavior.
- Making a fake adapter accept inputs that the real adapter rejects unless the difference is explicit
  in tests.
- Changing booking status, ticket status, eligibility, or check-in conflict semantics behind the same
  public interface.
- Requiring clients to branch on concrete adapter type to recover correct behavior.

## Event Ticketing Examples

- Any registration repository must preserve booking capacity, uniqueness, and idempotency behavior
  expected by application services.
- Any ticket signer must preserve token verification, expiry, and redaction semantics expected by
  ticket and check-in code.
- Any object-store adapter must preserve artifact write/read and missing-object errors expected by
  report export code.
- Any idempotency store must prevent duplicate booking, ticket generation, notification, or check-in
  side effects for the same operation identity.

## Relationships

- LSP reflects OCP because closed clients can remain unchanged only when extensions are behaviorally
  substitutable.
- LSP guides DIP because abstractions must specify stable behavior that implementations can honor.
- ISP supports LSP by keeping abstractions small enough for implementations to satisfy without fake
  or no-op behavior.

## Sources

- [Robert C. Martin, Liskov Substitution Principle](https://objectmentor.com/resources/articles/lsp.pdf)
- [Barbara Liskov and Jeannette Wing, Behavioral Subtyping](https://www.cs.cmu.edu/afs/cs/project/calder/www/fmdp.html)
- [SOLID overview](../solid.md)
