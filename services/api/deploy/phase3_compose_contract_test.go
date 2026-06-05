package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase3ComposeOverlayDeclaresLocalHATopology(t *testing.T) {
	compose := readText(t, composePhase3HAFile)
	nginx := readFilesUnder(t, "nginx/phase3")
	webNginx := readText(t, filepath.Join("..", "..", "..", "apps", "web", "nginx.phase3.conf"))
	envExample := readText(t, envExampleFile)
	combined := compose + "\n" + nginx + "\n" + webNginx + "\n" + envExample

	for _, fragment := range []string{
		"edge-lb:",
		"${PHASE3_EDGE_PORT:-18080}:8080",
		"gateway-1:",
		"gateway-2:",
		"gateway-3:",
		"frontend-lb:",
		"frontend-1:",
		"frontend-2:",
		"frontend-3:",
		"backend-lb:",
		"backend-1:",
		"CETS_REPLICA_ID: backend-1",
		"backend-2:",
		"CETS_REPLICA_ID: backend-2",
		"backend-3:",
		"CETS_REPLICA_ID: backend-3",
		"${PHASE3_POSTGRES_PORT:-15432}:5432",
		"${PHASE3_REDIS_PORT:-16379}:6379",
		"${PHASE3_MINIO_API_PORT:-19000}:9000",
		"${PHASE3_MAILHOG_UI_PORT:-18025}:8025",
		`profiles: ["phase3-ha"]`,
		`profiles: ["phase1-default"]`,
		`profiles: ["combined-worker"]`,
		"resolver 127.0.0.11 valid=5s ipv6=off;",
		"server gateway-1:8080 max_fails=1 fail_timeout=5s;",
		"server gateway-2:8080 max_fails=1 fail_timeout=5s;",
		"server gateway-3:8080 max_fails=1 fail_timeout=5s;",
		"server frontend-1:8080 max_fails=1 fail_timeout=5s;",
		"server frontend-2:8080 max_fails=1 fail_timeout=5s;",
		"server frontend-3:8080 max_fails=1 fail_timeout=5s;",
		"server backend-1:8080 max_fails=1 fail_timeout=5s;",
		"server backend-2:8080 max_fails=1 fail_timeout=5s;",
		"server backend-3:8080 max_fails=1 fail_timeout=5s;",
		"server backend-lb:8080 resolve max_fails=1 fail_timeout=5s;",
		"proxy_pass http://cets_backend_lb",
		"proxy_pass http://cets_frontend_lb",
		"proxy_set_header traceparent $http_traceparent;",
		"proxy_set_header tracestate $http_tracestate;",
		"proxy_set_header baggage $http_baggage;",
		"proxy_set_header X-Request-ID $request_id;",
		"X-CETS-Edge-Replica",
		"X-CETS-Gateway-Replica",
		"X-CETS-Frontend-Replica",
		"X-CETS-Backend-Replica",
		"PHASE3_EDGE_PORT=18080",
		"PHASE3_POSTGRES_PORT=15432",
		"PHASE3_MINIO_API_PORT=19000",
		"PHASE3_MAILHOG_UI_PORT=18025",
	} {
		assert.Contains(t, combined, fragment, "Phase 3 Compose HA topology is missing %q", fragment)
	}
}

func TestPhase3ComposeOverlayDeclaresWorkerKindIsolation(t *testing.T) {
	compose := readText(t, composePhase3HAFile)

	for _, kind := range []string{"notification", "projection", "compensation", "export"} {
		assert.Contains(t, compose, "worker-"+kind+":")
		assert.Contains(t, compose, "OTEL_SERVICE_NAME: cets-worker-"+kind)
		assert.Contains(t, compose, "PYROSCOPE_APPLICATION_NAME: cets-worker-"+kind)
	}
	assert.NotContains(t, compose, "WORKER_KINDS: notification,projection,compensation,export",
		"Phase 3 workers must stay kind-scoped when the isolation overlay is enabled")
}

func TestPhase3ComposeLGTMDeclaresFourSignalsAndNodeGraph(t *testing.T) {
	compose := readText(t, composePhase3HAFile)
	observability := readFilesUnder(t, "observability/phase3")
	combined := compose + "\n" + observability

	for _, fragment := range []string{
		"grafana/grafana:12.4.0",
		"grafana/loki:3.6.0",
		"prom/prometheus:v3.6.0",
		"grafana/tempo:2.10.0",
		"grafana/pyroscope:1.18.1",
		"grafana/alloy:v1.16.0",
		"--web.enable-remote-write-receiver",
		"job_name: cets-backend",
		"backend-1:8080",
		"backend-2:8080",
		"backend-3:8080",
		"service_graphs:",
		"span_metrics: {}",
		"type: loki",
		"type: tempo",
		"type: prometheus",
		"type: grafana-pyroscope-datasource",
		"serviceMap:",
		"nodeGraph:",
		"tracesToLogsV2:",
		"tracesToMetrics:",
		"tracesToProfiles:",
		"discovery.docker",
		"loki.source.docker",
		"otelcol.receiver.otlp",
		"cets_http_requests_total",
		"cets_build_info",
		"label_values(cets_build_info, service)",
		"sum by (service, route) (rate(cets_http_requests_total[1m]))",
		"sum by (service) (rate(cets_http_requests_total{status_class=\\\"5xx\\\"}[5m]))",
		"sum by (service, replica, route, status) (rate(cets_http_requests_total{service=~\\\"$service\\\",replica=~\\\"$replica\\\",status_class=\\\"5xx\\\"}[1m]))",
		"Backend RED by Replica",
		"DB Pool Wait Rate by Backend",
		"Global DB Lock Waiting Sessions",
		`cets_db_pool_acquire_wait_seconds_total{job=\"cets-backend\"}`,
		`cets_db_lock_waiting_sessions{job=\"cets-backend\"}`,
		"traces_service_graph_request_total",
		"traces_spanmetrics_calls_total",
		"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
	} {
		assert.Contains(t, combined, fragment, "Phase 3 LGTM contract is missing %q", fragment)
	}
}

func TestPhase3ComposeLGTMRedactsSensitiveTelemetry(t *testing.T) {
	alloy := strings.ToLower(readText(t, "observability/phase3/alloy.alloy"))
	compose := strings.ToLower(readText(t, composePhase3HAFile))
	combined := alloy + "\n" + compose

	for _, fragment := range []string{
		"signed_token",
		"signed_qr_token",
		"provider_token",
		"provider_secret",
		"email_body",
		"raw_recipient_email",
		"raw_idempotency_key",
		"stage.replace",
		"replace    = `\"[redacted]\"`",
		"replace    = `[redacted]`",
		"phase3-redaction-canary",
	} {
		assert.Contains(t, combined, fragment)
	}
}

func TestPhase3OTelTraceIDCompatibilityContract(t *testing.T) {
	router := readText(t, filepath.Join("..", "internal", "httpapi", "router.go"))
	datasources := readText(t, filepath.Join("observability", "phase3", "grafana", "provisioning", "datasources", "datasources.yml"))
	dashboard := readText(t, filepath.Join("observability", "phase3", "grafana", "dashboards", "cets-phase3-compose.json"))
	verify := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify.sh")) +
		readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-trace-evidence.sh"))

	for _, source := range []string{router, datasources, dashboard, verify} {
		assert.Contains(t, source, "otel_trace_id")
	}
	assert.Contains(t, datasources, `[a-fA-F0-9]{32}`)
	assert.Contains(t, router, "otelTraceIDFromContext")
	assert.Contains(t, router, "observability.TraceHTTP")
	assert.Contains(t, verify, `service_name=~\"backend-.*\"`)
	assert.Contains(t, verify, `|= \"$TEMPO_TRACE_ID\"`)
	assert.NotContains(t, verify, `if printf '%s\n' "$traces" | grep -q '"traceID"'`)
	assert.Contains(t, verify, `have jq || die "jq is required"`)
	assert.NotContains(t, verify, `grep -Eq "http.route|cets.route|rootTraceName"`)
}

func TestPhase3ComposeReplacesLocalK3sAssets(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "..", "scripts", "k3s"),
		"k8s-phase3",
		filepath.Join("observability", "k8s-lgtm"),
		filepath.Join("..", "..", "..", "docs", "specs", "phase3-local-ha-k3s-lgtm.md"),
	} {
		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err), "obsolete Phase 3 k3s path should be removed: %s", path)
	}
	require.FileExists(t, filepath.Join("..", "..", "..", "docs", "specs", "phase3-local-ha-compose-lgtm.md"))
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func requireCommand(t *testing.T, name string) {
	t.Helper()
	_, err := exec.LookPath(name)
	require.NoError(t, err)
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o700))
}

func readFilesUnder(t *testing.T, root string) string {
	t.Helper()
	var builder strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		builder.WriteString("\n--- ")
		builder.WriteString(filepath.ToSlash(path))
		builder.WriteString(" ---\n")
		builder.Write(data)
		return nil
	})
	require.NoError(t, err)
	return builder.String()
}
