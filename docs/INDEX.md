# Documentation Index

Use this page as the first stop when deciding which document is current.

## Current Source Of Truth

- `AGENTS.md` - agent workflow and non-negotiable project rules.
- `docs/ARCHITECTURE.md` - architecture decisions, phase boundaries, and deployment shape.
- `docs/PRODUCT.md` - product scope and business flows.
- `docs/DESIGN.md` - UI design system and product interaction direction.
- `docs/diagrams/` - editable draw.io deployment architecture and ERD diagrams with SVG exports.
- `docs/openapi.yaml` and `docs/openapi/` - public API contract.
- `docs/ci-self-hosted-runner.md` - opt-in self-hosted GitHub Actions runner setup for billing or quota blocks.

## Active Specifications

- `docs/specs/backend-directory-architecture.md` - backend package boundaries and refactor target.
- `docs/specs/phase1-production-upper-bound.md` - Phase 1 production acceptance contract.
- `docs/specs/phase1-product-requirements.md` - Phase 1 product requirements.
- `docs/specs/phase1-nfr-and-capacity.md` - Phase 1 non-functional requirements and capacity baseline.
- `docs/specs/phase1-e2e-test-paths.md` - Phase 1 test layer map.
- `docs/specs/phase2-scale-hardening.md` - Phase 2 acceptance matrix and deferred decision gates.
- `docs/specs/phase2-event-contract-v2.md` - event envelope and registry contract.
- `docs/specs/phase2-redis-reservation-gate.md` - Redis reservation pre-admission contract.
- `docs/specs/phase2-ws1-contracts-release.md` - contracts, release gates, and OpenAPI hardening.
- `docs/specs/phase2-ws2-load-observability.md` - load and observability workstream.
- `docs/specs/phase2-ws3-registration-hot-path.md` - registration hot path and reservation pressure.
- `docs/specs/phase2-ws4-async-notification.md` - async notification, outbox, and worker isolation.
- `docs/specs/phase2-ws5-reporting-ops.md` - reporting read model and ops controls.
- `docs/specs/phase3-local-ha-compose-lgtm.md` - current Phase 3 local HA Compose and LGTM simulation contract.

## Rules And Principles

- `docs/agent-rules/` - task-specific rules for architecture, clean code, correctness, workflow, and local runtime.
- `docs/principles/solid.md` - SOLID project guidance.
- `docs/principles/twelve-factor-rule.md` - Twelve-Factor local and cloud-native guidance.

## Archive

- `docs/archive/specs/` contains completed, superseded, or one-time implementation specs.
- Archived docs are historical evidence. If an archived document conflicts with an active source-of-truth document, follow the active document.
