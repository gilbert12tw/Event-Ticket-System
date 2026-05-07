---
name: twelve-factor-spec-first
description: >
  Twelve-Factor App compliance and Spec-First development discipline for cloud-native
  services. Enforces writing a feature specification (acceptance criteria, edge cases,
  non-functional requirements, minimal API contract) before any implementation, then
  validates every code change against all 12 factors: codebase, dependencies, config,
  backing services, build-release-run, processes, port binding, concurrency,
  disposability, dev-prod parity, logs, and admin processes. Covers env-based config,
  structured logging to stdout, graceful shutdown, stateless processes, multi-stage
  Docker builds, and pre-commit verification checklists.
---

# Twelve-Factor App + Spec-First Development

Enforce the 12-Factor methodology and "spec before code" discipline on every feature.

## When to Activate

- Adding a new API endpoint, service, or microservice
- Implementing any feature that touches config, backing services, or deployment
- Reviewing code for cloud-native compliance
- Writing a new feature spec, PRD, or design doc
- Refactoring an existing service toward cloud-native best practices
- Creating or modifying Dockerfiles, CI pipelines, or deployment manifests
- Debugging production issues related to config, state, or logging
- Starting any task where requirements are ambiguous or underspecified

## Spec-First Workflow

**Rule: Spec before code.** No implementation begins until a specification exists.
A specification reduces ambiguity, catches design flaws early, and creates a
shared contract between author, reviewer, and future maintainer.

### Step 1 — Write the Spec

Before writing any code, fill out the Spec Template below (or an equivalent doc).
The spec is committed alongside the code in a `docs/` or `specs/` directory.

### Step 2 — Review the Spec

The spec is reviewed by at least one peer. Review focuses on:
- Are acceptance criteria testable and unambiguous?
- Are edge cases identified and handled?
- Are NFRs realistic and measurable?
- Is the API contract minimal and backward-compatible?

### Step 3 — Implement Against the Spec

Code is written to satisfy the spec. Tests map 1:1 to acceptance criteria.
Any deviation from the spec during implementation triggers a spec update first.

### Step 4 — Verify with Checklist

Before merge, run the 12-Factor + Spec-First Checklist at the bottom of this file.

## Spec Template

Copy this template into `docs/specs/<feature-name>.md` before coding.

**Section 1 — Header and Summary:**

    # Feature: <Name>
    ## Summary
    One paragraph describing what this feature does and why.

**Section 2 — Acceptance Criteria (testable, Given/When/Then):**

    - [ ] AC-1: Given <precondition>, When <action>, Then <expected result>
    - [ ] AC-2: Given <precondition>, When <action>, Then <expected result>
    - [ ] AC-3: Given <precondition>, When <action>, Then <expected result>

**Section 3 — Edge Cases table:**

    | # | Scenario | Expected Behavior |
    |---|----------|-------------------|
    | E-1 | empty input / nil / zero value | return error / skip / default |
    | E-2 | concurrent access / race | idempotent / mutex / retry |
    | E-3 | backing service unavailable | circuit break / degrade / backoff |

**Section 4 — Non-Functional Requirements table:**

    | Category | Requirement | Metric |
    |----------|-------------|--------|
    | Timeout | API responds within N ms at p99 | < 500ms p99 |
    | Observability | Structured log for every state change | log event per op |
    | Failure Handling | Graceful degradation when <svc> down | fallback + alert |
    | Rate Limit | Max N req/s per client | 429 after threshold |
    | Idempotency | Retry-safe for POST/PUT | idempotency key / upsert |

**Section 5 — Minimal API Contract:**

```text
POST /api/v1/<resource>
Content-Type: application/json
Authorization: Bearer <token>

{ "field_a": "string (required)", "field_b": 0 }
```

Success response (201):

```json
{ "success": true, "data": { "id": "uuid", "field_a": "string" }, "error": null }
```

Error response (4xx/5xx):

```json
{ "success": false, "data": null, "error": "human-readable message" }
```

**Section 6 — 12-Factor Compliance Notes:**

    - Config: All new config via env vars; no hardcoded values.
    - Backing Services: New dependency attached via URL/credential in env.
    - Logs: Structured JSON to stdout; no file writes.
    - Disposability: Graceful shutdown handles in-flight work.
    - Processes: No local state; session data in Redis/DB.

## The Twelve Factors — Actionable Rules

### I. Codebase — One repo, many deploys

- One Git repo per app; shared code extracted into libraries via dependency manager.
- The same codebase deploys to dev, staging, production — only config changes.

**Do:** Monorepo with clear module boundaries (`backend/`, `dashboard/`).
**Don't:** Copy-paste code between repos; fork instead of library.

### II. Dependencies — Explicitly declare and isolate

- All dependencies declared in manifest (`go.mod`, `package.json`).
- Never rely on system-wide packages or implicit tools.

**Do:** `go mod tidy`, lockfiles committed, multi-stage Docker with vendored deps.
**Don't:** `go install` in production without pinned version; `curl | bash` in CI.

### III. Config — Store in the environment

- Config that varies between deploys lives in env vars, never in code.
- Litmus test: could you open-source the repo right now without leaking secrets?

```go
// CORRECT — typed env getter with safe fallback
port := getEnv("SERVER_PORT", ":8080")
dbHost := getEnv("DB_HOST", "localhost")
maxConns := parseEnvInt("DB_MAX_OPEN_CONNS", 25)
```

```go
// WRONG — hardcoded connection string
dsn := "postgres://admin:secret@prod-db:5432/mydb"
```

**Do:** `getEnv(key, fallback)` helpers; validate required vars at startup.
**Don't:** Commit `.env` with real secrets; group config by "environment name".

### IV. Backing Services — Attached resources via config

- Database, Redis, MinIO, LakeFS, SMTP are all "attached resources."
- Swappable by changing env vars alone — no code change.

```go
// CORRECT — backing service resolved from config
func NewProjectRepo(db *gorm.DB) project.Repository {
    return &projectRepo{db: db}
}
```

**Do:** Inject connection via constructor; one resource handle per service.
**Don't:** Hard-wire `localhost:5432` in repository code.

### V. Build, Release, Run — Strict separation

- **Build**: compile binary, install deps (`go build`, `npm run build`).
- **Release**: build artifact + config = deployable unit (Docker image + env).
- **Run**: start the release (container orchestrator launches pod).

```dockerfile
# CORRECT — multi-stage build, config injected at run time
FROM golang:1.24 AS builder
RUN CGO_ENABLED=0 go build -o main ./cmd/api
FROM ubuntu:latest
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
```

**Do:** Immutable images; unique release tags; rollback = deploy previous tag.
**Don't:** `go build` inside running container; modify code at runtime.

### VI. Processes — Stateless, share-nothing

- Every process is stateless. Persistent data goes to a backing service.
- No sticky sessions; JWT carries auth state.

```go
// CORRECT — auth state in token, session in Redis
claims := jwt.ParseToken(c.GetHeader("Authorization"))
c.Set("userID", claims.UserID)
```

**Do:** Store session data in Redis with TTL; file uploads to object storage.
**Don't:** Write temp files that must survive restarts; use in-memory session maps.

### VII. Port Binding — Self-contained HTTP server

- App exports HTTP by binding to a port. No runtime webserver injection.

```go
srv := &http.Server{
    Addr:    getEnv("SERVER_PORT", ":8080"),
    Handler: router,
}
```

**Do:** Port from env var; app starts its own listener.
**Don't:** Rely on external Apache/Nginx to inject the app as a module.

### VIII. Concurrency — Scale out via process model

- Scale by running more process instances, not by threading inside one giant VM.
- Assign different work to different process types (web, worker, cron).

**Do:** HPA scales pod replicas; worker pods for background jobs.
**Don't:** Spawn unbounded goroutines to handle all work in one process.

### IX. Disposability — Fast startup, graceful shutdown

- Processes start in seconds and handle SIGTERM gracefully.
- Web: stop listening, drain in-flight requests, exit.
- Worker: return current job to queue, then exit.

```go
// CORRECT — graceful shutdown with timeout
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
srv.Shutdown(ctx)
```

**Do:** `defer cleanup()` for connections; idempotent job processing.
**Don't:** Ignore SIGTERM; leave database connections dangling.

### X. Dev/Prod Parity — Keep environments similar

- Same backing services in dev and production (PostgreSQL, not SQLite).
- Same Docker image runs everywhere; only env vars differ.

```yaml
# docker-compose.dev.yml — same services as production
services:
  postgres:
    image: postgres:15
  redis:
    image: redis:7
  minio:
    image: minio/minio
```

**Do:** `docker-compose` with production-grade images for local dev.
**Don't:** SQLite in dev, PostgreSQL in prod; skip Redis locally.

### XI. Logs — Event streams to stdout

- App writes structured JSON logs to stdout. Never manage log files.
- Log routing, storage, and analysis handled by the execution environment.

```go
// CORRECT — structured JSON to stdout via slog
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
logger := slog.New(handler)
logger.Info("request handled",
    "method", r.Method,
    "path", r.URL.Path,
    "status", status,
    "duration_ms", elapsed.Milliseconds(),
)
```

**Do:** `slog.NewJSONHandler(os.Stdout, ...)` with structured key-value pairs.
**Don't:** `log.SetOutput(file)`; `fmt.Println` for operational logs.

### XII. Admin Processes — One-off tasks as processes

- Migrations, data fixes, and console tasks run as one-off processes.
- Same codebase, same config, same dependencies as the running app.

```bash
# CORRECT — migration as one-off container with same image
kubectl run migrate --image=platform-api:v1.2.3 \
  --restart=Never --env-from=configmap/api-config \
  -- ./main migrate
```

**Do:** Migrations in the same binary; one-off K8s Jobs for admin tasks.
**Don't:** SSH into production and run ad-hoc scripts with different deps.

## Non-Functional Requirements Patterns

When writing NFRs in spec, use these concrete patterns:

### Timeouts

```go
// HTTP client with explicit timeout
client := &http.Client{Timeout: 10 * time.Second}

// Database query with context deadline
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
db.WithContext(ctx).Find(&results)
```

### Observability

```go
// Prometheus counter for business metrics
requestsTotal := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "api_requests_total",
        Help: "Total API requests by method and status",
    },
    []string{"method", "path", "status"},
)
```

### Failure Handling

```go
// Circuit breaker for external service calls
cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "external-api",
    MaxRequests: 3,
    Interval:    10 * time.Second,
    Timeout:     30 * time.Second,
})
result, err := cb.Execute(func() (interface{}, error) {
    return client.Get(url)
})
```

## 12-Factor + Spec-First Checklist

### Spec Completeness

- [ ] Spec exists in `docs/specs/` before implementation started
- [ ] Acceptance criteria are testable (Given/When/Then)
- [ ] Edge cases table covers: empty input, concurrent access, service failure
- [ ] NFRs include: timeout, observability, failure handling
- [ ] API contract defines request, success response, and error response
- [ ] Tests map 1:1 to acceptance criteria

### Factor Compliance

- [ ] **I. Codebase**: Changes in one repo; shared code is a library
- [ ] **II. Dependencies**: `go.mod` / `package.json` updated; no implicit deps
- [ ] **III. Config**: New config via env vars with `getEnv()` helper; no secrets in code
- [ ] **IV. Backing Services**: New services attached via env URL/credentials
- [ ] **V. Build/Release/Run**: Dockerfile builds immutable image; config at runtime
- [ ] **VI. Processes**: No local state stored; data in DB/Redis/object storage
- [ ] **VII. Port Binding**: Server binds to env-configured port
- [ ] **VIII. Concurrency**: Scales horizontally; no singleton process assumptions
- [ ] **IX. Disposability**: Handles SIGTERM; drains in-flight work; starts in <5s
- [ ] **X. Dev/Prod Parity**: docker-compose uses same DB/cache as production
- [ ] **XI. Logs**: Structured JSON to stdout; no file log management
- [ ] **XII. Admin Processes**: Migrations/scripts as one-off processes with same image
