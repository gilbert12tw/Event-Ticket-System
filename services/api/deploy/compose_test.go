package deploy

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeAppPortBindingInvariant(t *testing.T) {
	compose, err := os.ReadFile(composeFile)
	require.NoError(t, err)
	envExample, err := os.ReadFile(envExampleFile)
	require.NoError(t, err)

	composeText := string(compose)
	assert.Contains(t, composeText, "APP_ADDR: :8080", "compose app must listen on the published container target port")
	assert.Contains(t, composeText, "http://127.0.0.1:8080/readyz", "compose healthcheck must verify the HTTP readiness endpoint")
	assert.Contains(t, composeText, "image: ${API_IMAGE_NAME:-cets-api}:${API_IMAGE_TAG:-dev}", "compose services must share a tagged API image contract")
	assert.Contains(t, composeText, "context: ../../..", "compose build must use the repository root context")
	assert.Contains(t, composeText, "dockerfile: services/api/Dockerfile", "compose build must use the services/api Dockerfile")
	assert.NotContains(t, string(envExample), "APP_ADDR=", ".env.example must not expose APP_ADDR because compose publishes container port 8080")
}

func TestComposeDeclaresPhase1BackingServiceContracts(t *testing.T) {
	compose, err := os.ReadFile(composeFile)
	require.NoError(t, err)
	envExample, err := os.ReadFile(envExampleFile)
	require.NoError(t, err)
	combined := string(compose) + "\n" + string(envExample)

	required := []string{
		"DATABASE_URL: postgresql://",
		"${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}",
		"TOKEN_SIGNING_SECRET:",
		"PROVIDER_TOKEN_SECRET:",
		"REQUEST_TIMEOUT_MS:",
		"DATABASE_TIMEOUT_MS:",
		"SHUTDOWN_TIMEOUT_MS:",
		"WORKER_POLL_INTERVAL_MS:",
		"WORKER_SHUTDOWN_GRACE_SECONDS:",
		"WORKER_MAX_ATTEMPTS:",
		"OUTBOX_BATCH_SIZE:",
		"OUTBOX_BATCH_SIZE=100",
		"OUTBOX_BATCH_SIZE: ${OUTBOX_BATCH_SIZE:-100}",
		"WORKER_BATCH_SIZE:",
		"OUTBOX_LEASE_TTL_SECONDS:",
		"OUTBOX_RETRY_MAX:",
		"OUTBOX_BACKOFF_BASE_MS:",
		"OUTBOX_BACKOFF_MAX_MS:",
		"DEMO_DEBUG_ENABLED=false",
		"DEMO_DEBUG_ENABLED: ${DEMO_DEBUG_ENABLED:-false}",
		"worker:",
		`command: ["worker"]`,
		"migrate:",
		`command: ["migrate"]`,
		"db-reset:",
		`profiles: ["admin"]`,
		`command: ["reset-demo-db"]`,
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
		assert.Contains(t, combined, fragment, "compose/env contract is missing %q", fragment)
	}
}

func TestComposeDBResetIsManualAdminOneOff(t *testing.T) {
	compose, err := os.ReadFile(composeFile)
	require.NoError(t, err)
	composeText := string(compose)

	assert.Contains(t, composeText, "db-reset:")
	assert.Contains(t, composeText, `profiles: ["admin"]`)
	assert.Contains(t, composeText, `command: ["reset-demo-db"]`)
	assert.Contains(t, composeText, "REDIS_URL: redis://redis:6379/0")
	assert.Contains(t, composeText, "postgres:\n        condition: service_healthy")
	assert.Contains(t, composeText, "redis:\n        condition: service_healthy")
	assert.NotContains(t, composeText, "condition: service_completed_successfully\n      db-reset:")
}

func TestComposeExternalImagesAreDigestPinned(t *testing.T) {
	for _, file := range []string{
		composeFile,
		composePhase3HAFile,
		"compose.dev.yaml",
	} {
		content := readComposeTestFile(t, file)
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "image: ") {
				continue
			}
			image := strings.TrimSpace(strings.TrimPrefix(line, "image: "))
			if strings.Contains(image, "${") {
				continue
			}
			assert.Contains(t, image, "@sha256:", "%s must pin external image %q by digest", file, image)
		}
	}
}

func TestComposePassesNoShowPolicyConfig(t *testing.T) {
	compose, err := os.ReadFile(composeFile)
	require.NoError(t, err)
	envExample, err := os.ReadFile(envExampleFile)
	require.NoError(t, err)

	composeText := string(compose)
	envText := string(envExample)
	for _, name := range []string{"NO_SHOW_THRESHOLD", "NO_SHOW_COOLDOWN_DAYS", "NO_SHOW_GRACE_HOURS"} {
		assert.Contains(t, envText, name+"=", ".env.example must expose %s for local operators", name)
		assert.GreaterOrEqual(t, strings.Count(composeText, name+":"), 3, "app, worker, and seed must receive %s", name)
	}
}

func TestPhase3BackendCanEnableBookingPreadmission(t *testing.T) {
	compose, err := os.ReadFile(composePhase3HAFile)
	require.NoError(t, err)
	composeText := string(compose)

	for _, name := range []string{
		"BOOKING_PREADMISSION",
		"REDIS_OUTAGE_MODE",
		"RESERVATION_TTL_SECONDS",
		"RESERVATION_TTL_GRACE_SECONDS",
		"RESERVATION_COMPENSATION_INTERVAL_SECONDS",
		"REDIS_OPERATION_TIMEOUT_MS",
		"BOOKING_RESERVATION_HASH_SECRET",
	} {
		assert.Contains(t, composeText, name+":", "Phase3 backend must receive %s for capacity tuning", name)
	}
}

func readComposeTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func TestComposeDevOverlayDeclaresFrontendHotReloadContract(t *testing.T) {
	compose, err := os.ReadFile("compose.dev.yaml")
	require.NoError(t, err)
	envExample, err := os.ReadFile(envExampleFile)
	require.NoError(t, err)
	viteConfig, err := os.ReadFile("../../../apps/web/vite.config.ts")
	require.NoError(t, err)
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
		assert.Contains(t, combined, fragment, "frontend hot reload contract is missing %q", fragment)
	}
}

func TestDockerfileBuildsSingleRuntimeBinary(t *testing.T) {
	dockerfile, err := os.ReadFile("../Dockerfile")
	require.NoError(t, err)
	content := string(dockerfile)

	required := []string{
		"FROM public.ecr.aws/docker/library/node:24.14.1-alpine3.22 AS web-builder",
		"RUN corepack enable",
		"COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./",
		"COPY apps/web/package.json ./apps/web/",
		"--mount=type=cache,id=pnpm-store,target=/pnpm/store",
		"pnpm install --frozen-lockfile --filter cets-web --store-dir /pnpm/store",
		"RUN pnpm --filter cets-web build",
		"FROM public.ecr.aws/docker/library/golang:1.25.9-alpine3.22 AS builder",
		"COPY services/api/go.mod services/api/go.sum ./",
		"--mount=type=cache,target=/go/pkg/mod",
		"COPY --from=web-builder /src/services/api/internal/httpapi/static ./internal/httpapi/static",
		"--mount=type=cache,target=/root/.cache/go-build",
		"CGO_ENABLED=0 GOOS=linux go build -o /out/cets ./cmd/cets",
		"FROM public.ecr.aws/docker/library/alpine:3.22.4",
		"COPY --from=builder /out/cets /app/cets",
		"USER cets",
		"EXPOSE 8080",
		`ENTRYPOINT ["/app/cets"]`,
	}
	for _, fragment := range required {
		assert.Contains(t, content, fragment, "Dockerfile is missing %q", fragment)
	}
}
