# Design Topic Coverage

This package is self-contained for the System Design Primer material used by the skill. The original cloned repository is not required after packaging.

## Core Workflow

| Area | Packaged location |
| --- | --- |
| Study guide and topic order | `primer.md#study-guide` |
| Interview approach | `primer.md#how-to-approach-a-system-design-interview-question` |
| Use cases, constraints, assumptions | `primer.md#step-1-outline-use-cases-constraints-and-assumptions` |
| High-level design | `primer.md#step-2-create-a-high-level-design` |
| Core component design | `primer.md#step-3-design-core-components` |
| Scaling the design | `primer.md#step-4-scale-the-design` |
| Back-of-the-envelope calculations | `primer.md#back-of-the-envelope-calculations` |

## Architecture Topics

| Area | Packaged location |
| --- | --- |
| Performance vs scalability | `primer.md#performance-vs-scalability` |
| Latency vs throughput | `primer.md#latency-vs-throughput` |
| Availability vs consistency | `primer.md#availability-vs-consistency` |
| CAP theorem, CP, AP | `primer.md#cap-theorem` |
| Weak, eventual, strong consistency | `primer.md#consistency-patterns` |
| Fail-over, replication, availability numbers | `primer.md#availability-patterns` |
| DNS | `primer.md#domain-name-system` |
| CDN, push CDN, pull CDN | `primer.md#content-delivery-network` |
| Load balancers, layer 4, layer 7, horizontal scaling | `primer.md#load-balancer` |
| Reverse proxy and load balancer comparison | `primer.md#reverse-proxy-web-server` |
| Application layer, microservices, service discovery | `primer.md#application-layer` |
| RDBMS, replication, federation, sharding, denormalization, tuning | `primer.md#relational-database-management-system-rdbms` |
| NoSQL: key-value, document, wide column, graph | `primer.md#nosql` |
| SQL vs NoSQL decision guidance | `primer.md#sql-or-nosql` |
| Cache locations, query/object cache, update patterns | `primer.md#cache` |
| Cache-aside, write-through, write-behind, refresh-ahead | `primer.md#when-to-update-the-cache` |
| Asynchronism, message queues, task queues, back pressure | `primer.md#asynchronism` |
| HTTP, TCP, UDP, RPC, REST | `primer.md#communication` |
| RPC vs REST comparison | `primer.md#rpc-and-rest-calls-comparison` |
| Security | `primer.md#security` |
| Powers of two and latency numbers | `primer.md#appendix` |
| Additional prompts, real-world architectures, company blogs | `primer.md#additional-system-design-interview-questions` |

## Worked Examples

| Area | Packaged location |
| --- | --- |
| Pastebin / Bit.ly | `solutions/system_design/pastebin/README.md` |
| Twitter timeline and search | `solutions/system_design/twitter/README.md` |
| Web crawler | `solutions/system_design/web_crawler/README.md` |
| Mint.com | `solutions/system_design/mint/README.md` |
| Social graph | `solutions/system_design/social_graph/README.md` |
| Query cache / key-value cache | `solutions/system_design/query_cache/README.md` |
| Amazon sales ranking | `solutions/system_design/sales_rank/README.md` |
| AWS scaling to millions of users | `solutions/system_design/scaling_aws/README.md` |
| Worked-example index | `system-design-solutions.md` |

## Object-Oriented Design

| Area | Packaged location |
| --- | --- |
| Hash map | `solutions/object_oriented_design/hash_table/hash_map.py` |
| LRU cache | `solutions/object_oriented_design/lru_cache/lru_cache.py` |
| Call center | `solutions/object_oriented_design/call_center/call_center.py` |
| Deck of cards | `solutions/object_oriented_design/deck_of_cards/deck_of_cards.py` |
| Parking lot | `solutions/object_oriented_design/parking_lot/parking_lot.py` |
| Chat server | `solutions/object_oriented_design/online_chat/online_chat.py` |
| OOD index | `object-oriented-design.md` |

## Deliberate Exclusions

- Translations are excluded; the skill is English-first.
- `.graffle`, `.ipynb`, `.apkg`, EPUB scripts, and contribution docs are excluded.
- External links in `primer.md` are preserved as optional further reading, not required packaged content.
