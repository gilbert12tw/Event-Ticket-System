# Event Ticket System

Corporate event ticketing and check-in system for employees, activity admins,
check-in staff, and HR/system admins.

The current implementation is a Go modular monolith with a React SPA. PostgreSQL
is the source of truth for bookings, tickets, check-ins, reports, and audit
records. Redis is advisory only, MinIO stores report exports, and Mailhog is the
local email sink. App and worker processes use the same Go binary.

## Quick Start

```bash
export COMPOSE_PROJECT_NAME=cets-dev

docker compose \
  --env-file services/api/deploy/.env.example \
  -f services/api/deploy/compose.yaml \
  up -d app worker

curl -fsS http://localhost:8080/readyz
```

Open:

- App: `http://localhost:8080`
- Demo runbook: `http://localhost:8080/admin/demo`
- Mailhog: `http://localhost:18025`
- MinIO: `http://localhost:19001`

Useful local ports are `8080` for the Go app, `15432` for PostgreSQL, `16379`
for Redis, `19000`/`19001` for MinIO, and `11025`/`18025` for Mailhog.

## Frontend Development

```bash
docker compose \
  --env-file services/api/deploy/.env.example \
  -f services/api/deploy/compose.yaml \
  -f services/api/deploy/compose.dev.yaml \
  up -d app worker web-dev
```

Open `http://localhost:5173`. The Vite dev server proxies API calls to the app
container.

## Common Checks

```bash
go test ./services/api/...
pnpm --filter cets-web test
pnpm --filter cets-web build
docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config
```

For host-side database commands:

```bash
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets \
  go run ./services/api/cmd/cets migrate
```

## Auth Boundary

Production APIs use external provider bearer tokens and claims. Local/demo/test
helpers may issue mock provider-format tokens, but product docs and OpenAPI do
not require local login/logout, passwords, session lifecycle, or refresh tokens.

## Documentation

Start with `docs/INDEX.md`.

- `docs/ARCHITECTURE.md` - current architecture and evolution boundaries.
- `docs/PRODUCT.md` - product scope and role workflows.
- `docs/DESIGN.md` - UI direction and accessibility expectations.
- `docs/openapi.yaml` and `docs/openapi/` - API contract.
- `docs/specs/` - active acceptance criteria and narrowly scoped contracts.
