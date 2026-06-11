# Event Ticket System

Corporate event ticketing and check-in system for employees, activity admins,
check-in staff, and HR/system admins. Go modular monolith with a React SPA;
PostgreSQL is the source of truth (bookings, tickets, check-ins, reports,
audit), Redis is advisory only, MinIO stores report exports, and Mailhog is the
local email sink. App and worker share the same Go binary.

## Quick Start

```bash
export COMPOSE_PROJECT_NAME=cets-dev
docker compose --env-file services/api/deploy/.env.example \
  -f services/api/deploy/compose.yaml up -d app worker
curl -fsS http://localhost:8080/readyz
```

- App `http://localhost:8080` · demo runbook `/admin/demo` · Mailhog `:18025` · MinIO `:19001`
- Other ports: PostgreSQL `15432`, Redis `16379`, MinIO API `19000`, Mailhog SMTP `11025`.

## Frontend Development

Add the dev overlay, then open `http://localhost:5173` (Vite proxies the API):

```bash
docker compose --env-file services/api/deploy/.env.example \
  -f services/api/deploy/compose.yaml -f services/api/deploy/compose.dev.yaml \
  up -d app worker web-dev
```

## Common Checks

```bash
go test ./services/api/...
pnpm --filter cets-web test
pnpm --filter cets-web build
docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml config
```

Host DB CLI: `DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets go run ./services/api/cmd/cets migrate`

## Auth

Production APIs use external provider bearer tokens and claims; local/demo/test
helpers may issue mock provider-format tokens.

## Documentation

Start with `docs/INDEX.md` — `ARCHITECTURE.md`, `PRODUCT.md`, `DESIGN.md`,
`openapi.yaml`/`openapi/`, and `specs/`.
