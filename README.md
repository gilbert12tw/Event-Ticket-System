# Event-Ticket-System

企業員工活動票務與現場驗票系統。Phase 1 以 Go modular monolith 搭配 Docker Compose backing services，提供可本地展示的 React 前端、活動建立、資格檢查、報名、票券、驗票、報表與 audit flow。

## Local Developer Docker

Use this for local development. It keeps the Go app on `8080` for the Vite proxy, and moves backing services off common host ports such as `5432`.

### Services

| Service | Default URL / Port | Purpose |
| --- | --- | --- |
| CETS App | `http://localhost:8080` | React SPA and HTTP API. |
| PostgreSQL | `localhost:15432` | Source of truth for app data. |
| Redis | `localhost:16379` | Cache, idempotency, and queue backing service. |
| MinIO API | `http://localhost:19000` | S3-compatible object storage. |
| MinIO Console | `http://localhost:19001` | Object storage admin UI. |
| Mailhog SMTP | `localhost:11025` | Mock SMTP server. |
| Mailhog UI | `http://localhost:18025` | Captured email UI. |

### Shell Setup

```bash
export COMPOSE_PROJECT_NAME=cets-dev
export APP_PORT=8080
export POSTGRES_PORT=15432
export REDIS_PORT=16379
export MINIO_API_PORT=19000
export MINIO_CONSOLE_PORT=19001
export MAILHOG_SMTP_PORT=11025
export MAILHOG_UI_PORT=18025

dc() {
  docker compose --env-file services/api/deploy/.env.example -f services/api/deploy/compose.yaml "$@"
}
```

### First Start

```bash
dc build app
dc up -d postgres redis minio mailhog minio-init
dc run --rm migrate
dc up -d app worker
curl -fsS http://localhost:8080/readyz
```

Open `http://localhost:8080` for the User Workspace, or `http://localhost:8080/admin/demo` to run the full acceptance flow.

### Update After Code Changes

```bash
dc build app
dc run --rm migrate
dc up -d app worker
dc ps
```

Use this when Go code or production frontend assets change. `dc restart app worker` is enough only for config/runtime restarts with no image changes.

### Logs And Stop

```bash
dc logs -f app worker
dc down
```

To wipe local data:

```bash
dc down -v
```

### Frontend Hot Reload

The browser UI is a Vite + React + TypeScript SPA under `apps/web/`. Production assets are generated into `services/api/internal/httpapi/static` and embedded by the Go app.

```bash
corepack enable
pnpm install
dc up -d app worker
pnpm --filter cets-web dev
```

Run First Start once before this. Open `http://localhost:5173`. During Vite development, `/api`, `/healthz`, and `/readyz` proxy to the Go app on `localhost:8080`.

Primary browser routes are `/user/events`, `/user/tickets`, `/admin/events`, `/admin/checkin`, `/admin/reports`, `/admin/audit`, and `/admin/demo`. Legacy demo routes such as `/employee/events`, `/employee/tickets`, `/checkin`, `/hr/reports`, and `/demo` remain SPA aliases.

### Configuration

Ports and credentials are local defaults only. If `8080` is also busy, change `APP_PORT` and use the Docker-served UI on that port. Vite hot reload currently expects the Go app on `localhost:8080`.

For local CLI/tests against the Compose database, use:

```bash
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets
```

The app container uses the internal Compose hostname `postgres`; the host `DATABASE_URL` is for local CLI and tests.

MinIO local login defaults to `minioadmin` / `minioadmin_dev_password`.

## Local CLI

```bash
go test ./services/api/...
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets go run ./services/api/cmd/cets migrate
DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets go run ./services/api/cmd/cets seed
APP_ADDR=:8080 DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets AUTO_MIGRATE=true go run ./services/api/cmd/cets
```

The browser UI uses `/api/v1/auth/login` to create a local SSO-style `HttpOnly` session cookie. Legacy actor headers remain available only in `APP_ENV=local`, `demo`, or `test` for compatibility; production must use non-demo `TOKEN_SIGNING_SECRET` and `AUTH_SESSION_SECRET` values with secure cookies enabled.
