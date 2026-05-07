---
name: system-design-primer
description: Use for system design interview practice, large-scale architecture design, scalability tradeoff analysis, back-of-the-envelope estimates, CAP, consistency, availability, databases, caches, queues, load balancers, CDNs, communication protocols, security, worked examples such as Pastebin, Twitter timelines, web crawlers, Mint, social graphs, query caches, sales ranking, AWS scaling, and object-oriented design prompts.
---

# System Design Primer

Use this skill to structure architecture answers with the System Design Primer's interview workflow, topic guide, and worked examples. Keep answers practical: state assumptions, estimate scale, make tradeoffs explicit, and adapt the reference material to the user's actual constraints.

## Workflow

1. Clarify use cases, users, constraints, non-goals, and success metrics before designing.
2. Estimate scale with back-of-the-envelope numbers: traffic, storage, bandwidth, fanout, cache size, and latency targets.
3. Sketch the high-level design: clients, APIs, services, data stores, caches, queues, load balancers, CDNs, and external systems.
4. Detail core components and data flow for the most important use cases.
5. Scale the design by discussing bottlenecks, partitioning, replication, consistency, availability, failure handling, observability, security, and operational tradeoffs.
6. Close with the chosen architecture, rejected alternatives, remaining risks, and what to validate next.

## References

- Read `references/primer.md` for the full topic guide, interview approach, estimation tables, and system design fundamentals.
- Read `references/system-design-solutions.md` to choose a worked system design example and then load only that example folder.
- Read `references/object-oriented-design.md` for object-oriented design prompts and copied Python reference implementations.
- Read `references/design-topic-coverage.md` when checking whether the packaged references cover the primer's main design topics.
- Read `references/source-and-license.md` when attribution, licensing, or provenance matters.

For targeted lookup, search references with focused terms such as `CAP theorem`, `latency numbers`, `load balancer`, `cache-aside`, `SQL or NoSQL`, `message queues`, `REST`, or the product being designed.

## Answering Guidance

- Prefer synthesis over quoting; do not paste long sections of the primer into the answer.
- Ask only for missing product intent that materially changes the architecture. If this is interview practice, proceed with explicit assumptions.
- Keep diagrams text-native. The bundled references use Mermaid diagrams instead of image assets so agents can read and adapt them directly.
- Tie each technology choice to a constraint or tradeoff instead of listing components mechanically.
- For interview feedback, evaluate requirement coverage, estimates, component boundaries, bottleneck handling, and tradeoff clarity.
