# CETS — Corporate Event Ticketing System

A corporate-grade internal event ticketing system built as a **Go modular monolith** with a **React SPA**. It covers event publishing, eligibility, booking, waitlist, signed QR tickets, one-time check-in, reporting, and audit — all runnable locally with a single Docker Compose command.

> **Current Phase:** Phase 1 production-complete · Phase 2 in active development (process-first scale hardening)

---

## Feature Highlights

| Area | What's Included |
|---|---|
| **Event Management** | Create, publish, close, cancel, archive events; capacity types (limited / unlimited); eligibility rules per department / site / grade |
| **Booking & Allocation** | First-come-first-served and lottery modes; waitlist; idempotency keys; PostgreSQL row-lock oversell prevention |
| **Tickets** | Signed QR tokens; one-time redemption enforced by DB unique constraint; offline manifest download |
| **Check-in** | Online scan (real-time DB verification) and offline scan (signed manifest + sync on reconnect); conflict detection |
| **Notifications** | Email and in-app via PostgreSQL outbox worker; retry, dead-letter, delivery suppression |
| **Reporting** | Event summary read model (projection worker); CSV export via MinIO; freshness contract metadata |
| **Admin Rebuild** | `cets admin rebuild-projection` — truncate-and-rebuild the reporting projection from OLTP source; dry-run and spot-check validation modes |
| **Audit** | Immutable audit log for all sensitive actions; role-based query |
| **Observability** | JSON structured logs; Prometheus `/metrics`; optional LGTM stack (Grafana, Loki, Tempo, Prometheus, Pyroscope, Alloy) |

---

## Architecture

```
┌────────────────────────── Docker Compose ──────────────────────────┐
│                                                                     │
│  ┌──────────────────┐     ┌──────────────────────────────────────┐ │
│  │   CETS App        │     │              CETS Worker              │ │
│  │  (Go HTTP API +  │     │  same binary, WORKER_KINDS=...        │ │
│  │   React SPA)     │     │  • notification • projection          │ │
│  │  :8080           │     │  • compensation • export              │ │
│  └────────┬─────────┘     └──────────────┬───────────────────────┘ │
│           │                              │                          │
│      ┌────▼──────────────────────────────▼────┐                    │
│      │           PostgreSQL  (source of truth)  │                    │
│      │   bookings · tickets · outbox_events     │                    │
│      │   reporting_event_summary · audit_logs   │                    │
│      └──────────────────────────────────────────┘                   │
│                                                                     │
│  Redis (pre-admission gate / idempotency)                           │
│  MinIO (report exports / event posters)                             │
│  Mailhog (local SMTP mock)                                          │
└─────────────────────────────────────────────────────────────────────┘
```

**Key design decisions:**
- PostgreSQL is the **only** committed booking truth — Redis is a pre-admission gate, never final state.
- All async side-effects (notifications, projection updates, report exports) go through `outbox_events`.
- Worker kinds are the same binary, separated by `WORKER_KINDS` env var.
- One ticket may be successfully redeemed exactly once — enforced by a DB unique constraint on `checkin_records.ticket_id`.

---

## Quick Start (Docker)

### Prerequisites

- Docker + Docker Compose
- GNU Make or Bash

### One-Time Shell Aliases

```bash
export COMPOSE_PROJECT_NAME=cets-dev

dc() {
  docker compose --env-file services/api/deploy/.env.example \
    -f services/api/deploy/compose.yaml "$@"
}

dcdev() {
  docker compose --env-file services/api/deploy/.env.example \
    -f services/api/deploy/compose.yaml \
    -f services/api/deploy/compose.dev.yaml "$@"
}
```

### Start

```bash
dc build app
dc up -d app worker
curl -fsS http://localhost:8080/readyz   # → 200 OK when ready
```

`dc up` automatically runs **migrate → seed → app/worker** via Compose `depends_on`. Seed data includes demo employees E1001 Ariel / E1002 Ben / E2001 Carla (idempotent reruns).

Open:
- **User workspace:** `http://localhost:8080/user/events`
- **Admin panel:** `http://localhost:8080/admin/events`
- **Full demo flow:** `http://localhost:8080/admin/demo`

### Rebuild After Code Changes

```bash
dc build app
dc up -d app worker
```

### Logs & Teardown

```bash
dc logs -f app worker   # tail logs
dc down                 # stop containers
dc down -v              # stop + wipe all local data
```

---

## Local Services

| Service | Host | Purpose |
|---|---|---|
| CETS App | `http://localhost:8080` | React SPA + HTTP API |
| Vite Dev (HMR) | `http://localhost:5173` | Frontend hot-reload (dev overlay only) |
| PostgreSQL | `localhost:15432` | Source of truth |
| Redis | `localhost:16379` | Reservation gate / idempotency |
| MinIO API | `http://localhost:19000` | S3-compatible object storage |
| MinIO Console | `http://localhost:19001` | Object storage admin UI |
| Mailhog SMTP | `localhost:11025` | Mock SMTP server |
| Mailhog UI | `http://localhost:18025` | Captured email viewer |

---

## Frontend Development (Hot Reload)

The React SPA lives in `apps/web/` (Vite + TypeScript). Production assets are embedded into the Go binary.

**Containerized hot reload (recommended):**

```bash
dc build app
dcdev up -d app worker web-dev
# Open http://localhost:5173
```

**Host Vite (alternative):**

```bash
corepack enable && pnpm install
dc up -d app worker
CETS_DEV_API_TARGET=http://localhost:8080 pnpm --filter cets-web dev
```

**Primary routes:**
`/user/events` · `/user/tickets` · `/admin/events` · `/admin/checkin` · `/admin/reports` · `/admin/audit` · `/admin/demo`

---

## Backend Structure

```
services/api/
├── cmd/
│   ├── cets/                  # Main binary: serve / worker / migrate / seed / admin
│   └── reservation_experiment/ # Redis reservation load experiment
└── internal/
    ├── architecture/          # ArchUnit-style import boundary tests
    ├── config/                # Env-var config structs
    ├── eventcontract/         # Outbox envelope v2 typed event definitions
    ├── httpapi/               # HTTP handlers, routes, contracts, middleware
    ├── notification/          # Email + in-app delivery workers
    ├── objectstore/           # MinIO / S3-compatible adapter
    ├── observability/         # Metrics, structured logging, OTEL tracing
    ├── postgres/              # Migrations, schema, query files
    ├── ratelimit/             # Rate limiting middleware
    ├── reservation/           # Redis pre-admission gate + compensation
    ├── ticketing/             # Core domain: events, booking, tickets, check-in,
    │                          # waitlist, projection, rebuild, bans, offline sync
    └── traceid/               # Request trace ID propagation
```

---

## CLI Reference

```bash
# Run all tests (no DB required for unit tests)
go test ./services/api/...

# Run migrations
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets \
  go run ./services/api/cmd/cets migrate

# Seed demo data
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets \
  go run ./services/api/cmd/cets seed

# Start the HTTP server locally
APP_ADDR=:8080 AUTO_MIGRATE=true \
  DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets \
  go run ./services/api/cmd/cets serve

# Rebuild the reporting projection from OLTP (admin one-off)
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets \
  go run ./services/api/cmd/cets admin rebuild-projection

# Dry-run rebuild (aggregate only, no writes)
REBUILD_DRY_RUN=true \
  DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets \
  go run ./services/api/cmd/cets admin rebuild-projection
```

---

## Configuration

All config is injected via environment variables. See [`services/api/deploy/.env.example`](services/api/deploy/.env.example) for the full list.

Key variables:

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | *(required)* | PostgreSQL connection string |
| `REDIS_URL` | `redis://localhost:16379` | Redis for reservation gate |
| `APP_ADDR` | `:8080` | HTTP listen address |
| `WORKER_KINDS` | `all` | Comma-separated worker kinds: `notification,projection,compensation,export` |
| `REBUILD_DRY_RUN` | `false` | Dry-run mode for projection rebuild |
| `REBUILD_SAMPLE_VALIDATE` | `true` | Spot-check validation after rebuild |
| `OBJECT_STORAGE_ENDPOINT` | MinIO local | S3-compatible endpoint |
| `MAILER_HOST` | Mailhog local | SMTP host for notifications |

MinIO local login: `minioadmin` / `minioadmin_dev_password`

---

## Testing

```bash
# All Go unit + integration tests
go test ./services/api/...

# Frontend unit tests
pnpm --filter cets-web test

# Playwright E2E tests (requires running Compose stack)
dc up -d app worker
pnpm --filter cets-web test:e2e

# k6 production gate (requires running Compose stack)
docker run --rm -i --network cets-dev_default grafana/k6 run - < k6/phase1-production-gate.js
```

**Test philosophy:** TDD-first. Integration tests use isolated DB schemas via `newMigrationTestPool`. Every core flow (booking, cancellation, check-in, projection rebuild) has acceptance tests that must pass before merge.

---

## Observability

- **Metrics:** `GET http://localhost:8080/metrics` — Prometheus format; HTTP RED, DB pool, outbox lag, reservation counters.
- **Health:** `GET /healthz` (liveness) · `GET /readyz` (readiness — checks PostgreSQL connectivity).
- **Logs:** JSON structured to stdout. Fields include `trace_id`, `event_id`, `action`, `status` — no raw PII.

**Optional LGTM stack** (Grafana + Loki + Tempo + Prometheus + Pyroscope):

```bash
dc --profile observability up -d
# Grafana: http://localhost:3000
```

---

## Phase Roadmap

| Phase | Status | Scale Target | Key Focus |
|---|---|---|---|
| **Phase 1** | ✅ Production-complete | 36 RPS · 5 TPS · 320 concurrent | Booking correctness, offline check-in, notifications, reporting, audit |
| **Phase 2** | 🚧 In progress | 270 RPS · 35 TPS · 2,400 concurrent | Process-first worker isolation, Redis pre-admission gate, reporting read model, projection rebuild admin |
| **Phase 3** | 📋 Planned | 1,000 RPS · 120 TPS · 10,000 concurrent | Local HA simulation, LGTM full observability, offline check-in hardening |

> Phase 2 is **process-first**: same Go binary, multiple worker processes by kind. No Kafka, no Kubernetes, no microservices — those are deferred decision-gates gated by measured evidence.

---

## Contributing

See [`AGENTS.md`](AGENTS.md) for agent and developer workflow rules, [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for architecture decisions, and [`docs/agent-rules/`](docs/agent-rules/) for domain-specific implementation rules.

**Non-negotiable rules:**
- Source files: target 200–300 lines, never exceed 500.
- PostgreSQL is final truth for booking, tickets, check-in, and audit.
- Every booking, cancellation, ticket generation, notification, and check-in uses idempotency keys.
- One ticket redeemed successfully only once — DB constraint is the guarantee.
