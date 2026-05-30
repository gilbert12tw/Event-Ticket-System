package deploy

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase2AsyncDeploymentDoesNotDeclareExternalBrokers(t *testing.T) {
	files := deploymentContractFiles(t)
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`\bkafka\b`),
		regexp.MustCompile(`\brabbitmq\b`),
		regexp.MustCompile(`\brabbit\b`),
		regexp.MustCompile(`\bsqs\b`),
		regexp.MustCompile(`\bnats\b`),
		regexp.MustCompile(`\bamqp\b`),
		regexp.MustCompile(`\bpubsub\b`),
		regexp.MustCompile(`\bservicebus\b`),
	}

	var offenders []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		content := strings.ToLower(string(data))
		for _, pattern := range forbidden {
			if pattern.MatchString(content) {
				offenders = append(offenders, filepath.ToSlash(file)+": "+pattern.String())
			}
		}
	}
	assert.Empty(t, offenders,
		"Phase 2 WS4 deployment must remain PostgreSQL-outbox based, not add external broker resources: %s",
		strings.Join(offenders, "; "))
}

func TestWorkerIsolationComposeOverlayDeclaresKindScopedProcesses(t *testing.T) {
	overlay, err := os.ReadFile("compose.worker-isolation.yaml")
	require.NoError(t, err)
	content := string(overlay)

	assert.Contains(t, content, `worker:`)
	assert.Contains(t, content, `profiles: ["combined-worker"]`,
		"overlay must keep the default all-kinds worker out of the worker-isolation profile")
	for _, tc := range []struct {
		service     string
		kind        string
		concurrency string
	}{
		{service: "worker-notification", kind: "notification", concurrency: "WORKER_CONCURRENCY_NOTIFICATION"},
		{service: "worker-projection", kind: "projection", concurrency: "WORKER_CONCURRENCY_PROJECTION"},
		{service: "worker-compensation", kind: "compensation", concurrency: "WORKER_CONCURRENCY_COMPENSATION"},
		{service: "worker-export", kind: "export", concurrency: "WORKER_CONCURRENCY_EXPORT"},
	} {
		assert.Contains(t, content, tc.service+":")
		assert.Contains(t, content, "service: worker")
		assert.Contains(t, content, `profiles: ["worker-isolation"]`)
		assert.Contains(t, content, "WORKER_KINDS: "+tc.kind)
		assert.Contains(t, content, tc.concurrency+": ${"+tc.concurrency+":-")
	}
	assert.Contains(t, content, "WORKER_NOTIFICATION_MAILER_HOST",
		"live gates need an env-controlled notification failure injection point")
}

func TestComposeWorkerHonorsConfiguredShutdownGraceBudget(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	env, err := os.ReadFile(".env.example")
	require.NoError(t, err)

	assert.Contains(t, string(compose), "stop_grace_period: ${WORKER_STOP_GRACE_PERIOD:-45s}")
	assert.Contains(t, string(env), "WORKER_SHUTDOWN_GRACE_SECONDS=30")
	assert.Contains(t, string(env), "WORKER_STOP_GRACE_PERIOD=45s")
}

func TestK8sConfigMapDeclaresWS4WorkerControls(t *testing.T) {
	configmap, err := os.ReadFile("k8s/configmap.yaml")
	require.NoError(t, err)
	content := string(configmap)
	for _, fragment := range []string{
		`OPS_API_ENABLED: "false"`,
		`WORKER_SHUTDOWN_GRACE_SECONDS: "30"`,
		`OUTBOX_BATCH_SIZE: "100"`,
		`OUTBOX_LEASE_TTL_SECONDS: "60"`,
		`OUTBOX_RETRY_MAX: "10"`,
		`OUTBOX_BACKOFF_BASE_MS: "500"`,
		`OUTBOX_BACKOFF_MAX_MS: "60000"`,
		`WORKER_KINDS: notification,projection,compensation,export`,
		`WORKER_CONCURRENCY_NOTIFICATION: "4"`,
		`WORKER_CONCURRENCY_PROJECTION: "2"`,
		`WORKER_CONCURRENCY_COMPENSATION: "1"`,
		`WORKER_CONCURRENCY_EXPORT: "1"`,
	} {
		assert.Contains(t, content, fragment)
	}
}

func TestK8sWorkerManifestsDeclareKindScopedProcesses(t *testing.T) {
	worker, err := os.ReadFile("k8s/worker.yaml")
	require.NoError(t, err)
	content := string(worker)

	for _, kind := range []string{"notification", "projection", "compensation", "export"} {
		assert.Contains(t, content, "name: cets-worker-"+kind)
		assert.Contains(t, content, "app.kubernetes.io/worker-kind: "+kind)
		assert.Contains(t, content, "--kinds="+kind)
		assert.Contains(t, content, "value: "+kind)
	}
	assert.Equal(t, 4, strings.Count(content, "kind: Deployment"))
	assert.Equal(t, 4, strings.Count(content, "terminationGracePeriodSeconds: 45"),
		"k8s workers must exceed the 30 second worker drain window")
	assert.NotContains(t, content, "value: notification,projection,compensation,export",
		"k8s workers must not run the combined all-kinds process")
}

func TestLiveGatesUseWorkerIsolationComposeOverlay(t *testing.T) {
	workflow, err := os.ReadFile("../../../.github/workflows/ci.yml")
	require.NoError(t, err)
	content := string(workflow)

	assert.Contains(t, content, "compose.worker-isolation.yaml")
	assert.Contains(t, content, "--profile worker-isolation")
	for _, service := range []string{
		"worker-notification",
		"worker-projection",
		"worker-compensation",
		"worker-export",
	} {
		assert.Contains(t, content, service)
	}
	assert.NotContains(t, content, "up -d app worker\n",
		"live-gates must not run the combined all-kinds worker as the process-isolation evidence")
}

func TestLiveGatesRunWorkerIsolationLagK6Gate(t *testing.T) {
	workflow, err := os.ReadFile("../../../.github/workflows/ci.yml")
	require.NoError(t, err)
	phase1Script, err := os.ReadFile("../../../k6/phase1-production-gate.js")
	require.NoError(t, err)
	script, err := os.ReadFile("../../../k6/phase2-worker-isolation-lag.js")
	require.NoError(t, err)

	workflowContent := string(workflow)
	phase1ScriptContent := string(phase1Script)
	scriptContent := string(script)
	assert.Contains(t, workflowContent, "docker run --rm -i --network host")
	assert.Contains(t, workflowContent, "run - < k6/phase2-worker-isolation-lag.js")
	assert.Contains(t, workflowContent, "--network host")
	assert.Contains(t, workflowContent, `services/api/cmd/cets/worker.*\.go`)
	assert.Contains(t, workflowContent, "services/api/internal/config/")
	assert.Contains(t, workflowContent, `services/api/internal/postgres/schema\.sql`)
	assert.Contains(t, workflowContent, `services/api/internal/ticketing/(worker|outbox|notification).*\.go`)
	assert.Contains(t, workflowContent, "services/api/internal/observability/")
	assert.Contains(t, workflowContent, "APP_PORT: 18080")
	assert.Contains(t, workflowContent, "MINIO_API_PORT: 19000")
	assert.Contains(t, workflowContent, "MAILHOG_SMTP_PORT: 11025")
	assert.Contains(t, workflowContent, "LIVE_BASE_URL: http://127.0.0.1:18080")
	assert.Contains(t, workflowContent, "WORKER_NOTIFICATION_MAILER_HOST=127.0.0.1")
	assert.Contains(t, workflowContent, "K6_REQUIRED_WORKER_KINDS=export,projection,compensation")
	assert.Contains(t, workflowContent, "playwright install --with-deps chromium")
	assert.Contains(t, workflowContent, "Set up Docker CLI for local act runs")
	assert.Contains(t, workflowContent, "docker-compose-v2")
	assert.Contains(t, workflowContent, "docker compose version")
	assert.NotContains(t, workflowContent, "--force chromium",
		"local act should reuse runner Playwright browsers instead of forcing a fresh Chromium download")
	assert.Contains(t, phase1ScriptContent, "event_id: event.event_id")
	assert.Contains(t, scriptContent, "cets_outbox_lag_seconds_bucket")
	assert.Contains(t, scriptContent, "cets_outbox_oldest_lag_seconds")
	assert.Contains(t, scriptContent, "K6_OUTBOX_P95_MAX_SECONDS")
	assert.Contains(t, scriptContent, "K6_OUTBOX_MAX_SECONDS")
	assert.Contains(t, scriptContent, "K6_REQUIRED_WORKER_KINDS")
	assert.Contains(t, scriptContent, "workerKind === \"notification\"")
	assert.Contains(t, scriptContent, "0.95")
	assert.Contains(t, scriptContent, "exec.test.abort")
}

func deploymentContractFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isDeploymentContractFile(path) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, files)
	return files
}

func isDeploymentContractFile(path string) bool {
	for _, ext := range []string{".yaml", ".yml", ".md"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
