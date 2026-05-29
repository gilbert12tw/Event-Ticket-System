package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestComposeAppPortBindingInvariant(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	envExample, err := os.ReadFile(".env.example")
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
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	envExample, err := os.ReadFile(".env.example")
	require.NoError(t, err)
	combined := string(compose) + "\n" + string(envExample)

	required := []string{
		"DATABASE_URL: postgresql://",
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
		assert.Contains(t, combined, fragment, "compose/env contract is missing %q", fragment)
	}
}

func TestComposeDeclaresOptionalObservabilityStackContracts(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	envExample, err := os.ReadFile(".env.example")
	require.NoError(t, err)
	combined := string(compose) + "\n" + string(envExample)

	required := []string{
		"prometheus:",
		"image: prom/prometheus:v3.6.0",
		`profiles: ["observability"]`,
		"--config.file=/etc/prometheus/prometheus.yml",
		"${PROMETHEUS_PORT:-9090}:9090",
		"./observability/prometheus.yml:/etc/prometheus/prometheus.yml:ro",
		"blackbox-exporter:",
		"image: prom/blackbox-exporter:v0.27.0",
		"--config.file=/etc/blackbox_exporter/config.yml",
		"${BLACKBOX_EXPORTER_PORT:-9115}:9115",
		"./observability/blackbox.yml:/etc/blackbox_exporter/config.yml:ro",
		"./observability/rules:/etc/prometheus/rules:ro",
		"grafana:",
		"image: grafana/grafana:12.2.0",
		"GF_SECURITY_ADMIN_USER: ${GRAFANA_ADMIN_USER:-admin}",
		"GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_ADMIN_PASSWORD:-admin}",
		"${GRAFANA_PORT:-3000}:3000",
		"./observability/grafana/provisioning/datasources:/etc/grafana/provisioning/datasources:ro",
		"./observability/grafana/dashboards:/var/lib/grafana/dashboards:ro",
		"PROMETHEUS_PORT=9090",
		"BLACKBOX_EXPORTER_PORT=9115",
		"GRAFANA_PORT=3000",
	}
	for _, fragment := range required {
		assert.Contains(t, combined, fragment, "optional observability contract is missing %q", fragment)
	}
}

func TestObservabilityProvisioningDeclaresDashboardSignals(t *testing.T) {
	prometheus, err := os.ReadFile("observability/prometheus.yml")
	require.NoError(t, err)
	blackbox, err := os.ReadFile("observability/blackbox.yml")
	require.NoError(t, err)
	prometheusDatasource, err := os.ReadFile("observability/grafana/provisioning/datasources/prometheus.yml")
	require.NoError(t, err)
	dashboardFile, err := os.ReadFile("observability/grafana/dashboards/cets-observability.json")
	require.NoError(t, err)

	combined := string(prometheus) + "\n" +
		string(blackbox) + "\n" +
		string(prometheusDatasource) + "\n" +
		string(dashboardFile)
	required := []string{
		"job_name: cets-app",
		"metrics_path: /metrics",
		"rule_files:",
		"/etc/prometheus/rules/*.yml",
		"app:8080",
		"job_name: cets-blackbox",
		"metrics_path: /probe",
		"http://app:8080/",
		"http://app:8080/healthz",
		"http://app:8080/readyz",
		"probe_scope: blackbox",
		"blackbox-exporter:9115",
		"prober: http",
		"probe_success{probe_scope=\\\"blackbox\\\"}",
		"probe_duration_seconds{probe_scope=\\\"blackbox\\\"}",
		"url: http://prometheus:9090",
		"cets_http_requests_total",
		"cets_http_request_seconds_bucket",
		"cets_db_pool_conns",
		"cets_db_lock_waiting_sessions",
		"cets_outbox_oldest_lag_seconds",
	}
	for _, fragment := range required {
		assert.Contains(t, combined, fragment, "observability provisioning is missing %q", fragment)
	}

	var dashboard map[string]interface{}
	require.NoError(t, json.Unmarshal(dashboardFile, &dashboard))
	assert.Equal(t, "CETS Observability", dashboard["title"])
}

func TestBlackboxProbingStaysOutsideProductBehavior(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	prometheus, err := os.ReadFile("observability/prometheus.yml")
	require.NoError(t, err)
	combined := string(compose) + "\n" + string(prometheus)

	assert.NotContains(t, combined, "/api/v1/", "black-box probes must not exercise product APIs")
	assert.NotContains(t, combined, "Authorization:", "black-box probes must not depend on product credentials")
	assert.NotContains(t, combined, "app:\n    depends_on:\n      blackbox-exporter:", "app must not depend on probe health")
	assert.NotContains(t, combined, "worker:\n    depends_on:\n      blackbox-exporter:", "worker must not depend on probe health")
}

func TestPrometheusAlertRulesCoverStarterSLOSignals(t *testing.T) {
	rulesFile, err := os.ReadFile("observability/rules/cets-alerts.yml")
	require.NoError(t, err)

	var rules prometheusRulesFile
	require.NoError(t, yaml.Unmarshal(rulesFile, &rules))
	require.Len(t, rules.Groups, 1)
	assert.Equal(t, "cets-slo-alerts", rules.Groups[0].Name)

	alerts := map[string]prometheusAlertRule{}
	for _, rule := range rules.Groups[0].Rules {
		alerts[rule.Alert] = rule
	}
	requiredAlerts := []string{
		"CETSHighHTTPErrorRate",
		"CETSHighP99Latency",
		"CETSDBPoolAcquireWaitHigh",
		"CETSDBLockWaitingSessions",
		"CETSOutboxOldestLagHigh",
		"CETSMetricsScrapeErrors",
	}
	for _, alert := range requiredAlerts {
		rule, ok := alerts[alert]
		require.True(t, ok, "missing alert rule %s", alert)
		assert.NotEmpty(t, rule.Expr, "alert %s must have a PromQL expression", alert)
		assert.NotEmpty(t, rule.For, "alert %s must require sustained breach time", alert)
		assert.NotEmpty(t, rule.Labels["severity"], "alert %s must declare routing severity", alert)
		assert.NotEmpty(t, rule.Labels["slo"], "alert %s must map back to a starter SLO", alert)
		assert.NotEmpty(t, rule.Annotations["summary"], "alert %s must explain the symptom", alert)
	}

	combinedExpr := string(rulesFile)
	for _, metric := range []string{
		"cets_http_requests_total",
		"cets_http_request_seconds_bucket",
		"cets_http_request_seconds_count",
		"cets_db_pool_acquire_wait_seconds_total",
		"cets_db_pool_acquire_count_total",
		"cets_db_lock_waiting_sessions",
		"cets_outbox_oldest_lag_seconds",
		"cets_metrics_scrape_errors_total",
	} {
		assert.Contains(t, combinedExpr, metric, "alert rules must use the shipped PR #43 metric %s", metric)
	}
}

type prometheusRulesFile struct {
	Groups []prometheusRuleGroup `yaml:"groups"`
}

type prometheusRuleGroup struct {
	Name  string                `yaml:"name"`
	Rules []prometheusAlertRule `yaml:"rules"`
}

type prometheusAlertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

func TestComposePassesNoShowPolicyConfig(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	envExample, err := os.ReadFile(".env.example")
	require.NoError(t, err)

	composeText := string(compose)
	envText := string(envExample)
	for _, name := range []string{"NO_SHOW_THRESHOLD", "NO_SHOW_COOLDOWN_DAYS", "NO_SHOW_GRACE_HOURS"} {
		assert.Contains(t, envText, name+"=", ".env.example must expose %s for local operators", name)
		assert.GreaterOrEqual(t, strings.Count(composeText, name+":"), 3, "app, worker, and seed must receive %s", name)
	}
}

func TestComposeDevOverlayDeclaresFrontendHotReloadContract(t *testing.T) {
	compose, err := os.ReadFile("compose.dev.yaml")
	require.NoError(t, err)
	envExample, err := os.ReadFile(".env.example")
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

func TestKubernetesManifestsDeclareApplicationContracts(t *testing.T) {
	manifests := readKubernetesManifests(t)

	required := []string{
		"kind: Kustomization",
		"namespace: cets",
		"name: cets-api",
		"name: cets-worker-notification",
		"name: cets-worker-projection",
		"name: cets-worker-compensation",
		"name: cets-worker-export",
		"name: cets-api-config",
		"name: cets-api-secret",
		"APP_ADDR: \":8080\"",
		"APP_ENV: staging",
		"AUTH_MODE: external_sso",
		"AUTO_MIGRATE: \"false\"",
		"REDIS_URL: redis://cets-redis:6379/0",
		"QUEUE_URL: redis://cets-redis:6379/1",
		"OBJECT_STORAGE_ENDPOINT: http://cets-minio:9000",
		"OBJECT_STORAGE_BUCKET: cets-staging",
		"MAILER_HOST: cets-mailhog",
		"DATABASE_URL: postgresql://cets:change-me-postgres-password@cets-postgres:5432/cets",
		"TOKEN_SIGNING_SECRET: replace-with-staging-token-signing-secret-32chars",
		"PROVIDER_TOKEN_SECRET: replace-with-staging-provider-token-secret-32chars",
		"imagePullPolicy: IfNotPresent",
		"path: /healthz",
		"path: /readyz",
	}
	for _, fragment := range required {
		assert.Contains(t, manifests, fragment, "k8s manifest contract is missing %q", fragment)
	}

	assert.GreaterOrEqual(t, strings.Count(manifests, "image: cets-api:dev"), 7, "app, per-kind workers, migrate, and seed must share the API image contract")
}

func TestKubernetesManifestsDeclareFullLocalStackContracts(t *testing.T) {
	manifests := readKubernetesManifests(t)

	required := []string{
		"name: cets-postgres",
		"image: postgres:16.13-alpine",
		"name: cets-redis",
		"image: redis:7.4.8-alpine",
		"name: cets-minio",
		"image: minio/minio:RELEASE.2025-09-07T16-13-09Z",
		"name: cets-mailhog",
		"image: mailhog/mailhog:v1.0.1",
		"name: cets-minio-init",
		"image: minio/mc:RELEASE.2025-08-13T08-35-41Z",
		"mc mb --ignore-existing",
		"kind: StatefulSet",
		"volumeClaimTemplates:",
		"kind: Deployment",
		"kind: Service",
	}
	for _, fragment := range required {
		assert.Contains(t, manifests, fragment, "k8s local stack contract is missing %q", fragment)
	}
}

func TestKubernetesManifestsDeclareAdminProcessContracts(t *testing.T) {
	manifests := readKubernetesManifests(t)

	required := []string{
		"name: cets-migrate",
		"- migrate",
		"name: cets-seed",
		"suspend: true",
		"- seed",
		"name: cets-worker-notification",
		"- worker",
		"restartPolicy: OnFailure",
	}
	for _, fragment := range required {
		assert.Contains(t, manifests, fragment, "k8s admin process contract is missing %q", fragment)
	}
}

func TestKubernetesManifestsDoNotCommitComposeLocalSecrets(t *testing.T) {
	manifests := readKubernetesManifests(t)

	forbidden := []string{
		"cets_dev_password",
		"local_dev_ticket_signing_secret_change_me",
		"local_dev_provider_token_secret_change_me",
		"minioadmin_dev_password",
	}
	for _, fragment := range forbidden {
		assert.NotContains(t, manifests, fragment, "k8s manifests must not commit compose local secret %q", fragment)
	}
}

func readKubernetesManifests(t *testing.T) string {
	t.Helper()

	entries, err := os.ReadDir("k8s")
	require.NoError(t, err)

	var builder strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		content, err := os.ReadFile(filepath.Join("k8s", entry.Name()))
		require.NoError(t, err)
		builder.Write(content)
		builder.WriteString("\n")
	}
	return builder.String()
}
