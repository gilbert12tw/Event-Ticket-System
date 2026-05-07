package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestComposeAppPortBindingInvariant(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	envExample, err := os.ReadFile(".env.example")
	if err != nil {
		t.Fatal(err)
	}

	composeText := string(compose)
	if !strings.Contains(composeText, "APP_ADDR: :8080") {
		t.Fatal("compose app must listen on the published container target port")
	}
	if !strings.Contains(composeText, "http://127.0.0.1:8080/readyz") {
		t.Fatal("compose healthcheck must verify the HTTP readiness endpoint")
	}
	if !strings.Contains(composeText, "image: ${API_IMAGE_NAME:-cets-api}:${API_IMAGE_TAG:-dev}") {
		t.Fatal("compose services must share a tagged API image contract")
	}
	if !strings.Contains(composeText, "context: ../../..") || !strings.Contains(composeText, "dockerfile: services/api/Dockerfile") {
		t.Fatal("compose build must use the repository root context and services/api Dockerfile")
	}
	if strings.Contains(string(envExample), "APP_ADDR=") {
		t.Fatal(".env.example must not expose APP_ADDR because compose publishes container port 8080")
	}
}

func TestComposeDeclaresPhase1BackingServiceContracts(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	envExample, err := os.ReadFile(".env.example")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(compose) + "\n" + string(envExample)

	required := []string{
		"DATABASE_URL: postgresql://",
		"TOKEN_SIGNING_SECRET:",
		"AUTH_SESSION_SECRET:",
		"AUTH_SESSION_TTL_MINUTES:",
		"AUTH_COOKIE_SECURE:",
		"REQUEST_TIMEOUT_MS:",
		"DATABASE_TIMEOUT_MS:",
		"SHUTDOWN_TIMEOUT_MS:",
		"WORKER_POLL_INTERVAL_MS:",
		"WORKER_MAX_ATTEMPTS:",
		"WORKER_BATCH_SIZE:",
		"worker:",
		`command: ["worker"]`,
		"migrate:",
		`command: ["migrate"]`,
		"postgres:",
		"image: postgres:16.13-alpine",
		"redis:",
		"image: redis:7.4.8-alpine",
		"minio:",
		"image: minio/minio:RELEASE.2025-09-07T16-13-09Z",
		"minio-init:",
		"image: minio/mc:RELEASE.2025-08-13T08-35-41Z",
		"mc mb --ignore-existing",
		"mailhog:",
		"image: mailhog/mailhog:v1.0.1",
		"AUTO_MIGRATE=false",
		"REDIS_URL=redis://localhost:6379/0",
		"OBJECT_STORAGE_ENDPOINT=http://localhost:9000",
		"MAILER_HOST=localhost",
		"MAILER_REDIRECT_TO=notifications@cets.local",
	}

	for _, fragment := range required {
		if !strings.Contains(combined, fragment) {
			t.Fatalf("compose/env contract is missing %q", fragment)
		}
	}
}

func TestComposeDevOverlayDeclaresFrontendHotReloadContract(t *testing.T) {
	compose, err := os.ReadFile("compose.dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	envExample, err := os.ReadFile(".env.example")
	if err != nil {
		t.Fatal(err)
	}
	viteConfig, err := os.ReadFile("../../../apps/web/vite.config.ts")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(compose) + "\n" + string(envExample) + "\n" + string(viteConfig)

	required := []string{
		"web-dev:",
		"image: node:24.14.1-alpine3.22",
		"working_dir: /src",
		"WEB_DEV_PORT: ${WEB_DEV_PORT:-5173}",
		"CETS_DEV_API_TARGET: ${CETS_DEV_API_TARGET:-http://app:8080}",
		"CHOKIDAR_USEPOLLING: ${CHOKIDAR_USEPOLLING:-true}",
		"corepack enable",
		"pnpm config set store-dir /pnpm/store",
		"pnpm install --frozen-lockfile --filter cets-web",
		"pnpm --filter cets-web dev --host 0.0.0.0 --port",
		`"${WEB_DEV_PORT:-5173}:${WEB_DEV_PORT:-5173}"`,
		"../../..:/src",
		"web_dev_root_node_modules:/src/node_modules",
		"web_dev_app_node_modules:/src/apps/web/node_modules",
		"web_dev_pnpm_store:/pnpm/store",
		"condition: service_healthy",
		"WEB_DEV_PORT=5173",
		"CETS_DEV_API_TARGET=http://app:8080",
		"CHOKIDAR_USEPOLLING=true",
		"const configuredWebDevPort = process.env.WEB_DEV_PORT",
		`configuredWebDevPort ?? "5173"`,
		`process.env.CETS_DEV_API_TARGET ?? "http://localhost:8080"`,
		"strictPort: true",
		"hmr: configuredWebDevPort ? { clientPort: webDevPort } : undefined",
		"usePolling: true",
	}

	for _, fragment := range required {
		if !strings.Contains(combined, fragment) {
			t.Fatalf("frontend hot reload contract is missing %q", fragment)
		}
	}
}

func TestDockerfileBuildsSingleRuntimeBinary(t *testing.T) {
	dockerfile, err := os.ReadFile("../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	content := string(dockerfile)

	required := []string{
		"FROM node:24.14.1-alpine3.22 AS web-builder",
		"RUN corepack enable",
		"COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./",
		"COPY apps/web/package.json ./apps/web/",
		"RUN pnpm install --frozen-lockfile --filter cets-web",
		"RUN pnpm --filter cets-web build",
		"FROM golang:1.25.9-alpine3.22 AS builder",
		"COPY services/api/go.mod services/api/go.sum ./",
		"COPY --from=web-builder /src/services/api/internal/httpapi/static ./internal/httpapi/static",
		"RUN CGO_ENABLED=0 GOOS=linux go build -o /out/cets ./cmd/cets",
		"FROM alpine:3.22.4",
		"COPY --from=builder /out/cets /app/cets",
		"USER cets",
		"EXPOSE 8080",
		`ENTRYPOINT ["/app/cets"]`,
	}
	for _, fragment := range required {
		if !strings.Contains(content, fragment) {
			t.Fatalf("Dockerfile is missing %q", fragment)
		}
	}
}
