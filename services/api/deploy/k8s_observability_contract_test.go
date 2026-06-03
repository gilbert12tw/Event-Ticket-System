package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaremetalK8sLGTMProvisionsGrafanaDashboard(t *testing.T) {
	observability := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "deploy-cets", "observability.sh"))
	app := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "deploy-cets", "app.sh"))
	networking := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "30-networking.sh"))
	readme := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "README.md"))
	dashboardTexts := readK8sDashboardTexts(t)
	combined := observability + "\n" + app + "\n" + networking + "\n" + readme
	for _, text := range dashboardTexts {
		combined += "\n" + text
	}

	for _, fragment := range []string{
		"cets-grafana-dashboards.yaml",
		"kind: ConfigMap",
		`grafana_dashboard: "1"`,
		"grafana_folder: Cets",
		"folderAnnotation: grafana_folder",
		"foldersFromFilesStructure: true",
		"CETS Metrics RED",
		"CETS Metrics USE",
		"CETS Logs",
		"CETS Traces",
		"CETS Profiles",
		"CETS_REPLICA_ID",
		"fieldPath: metadata.name",
		"cets_build_info",
		"label_values(cets_build_info, service)",
		"label_values(cets_build_info{service=~\\\"$service\\\"}, replica)",
		"sum by (service, route) (rate(cets_http_requests_total[1m]))",
		"sum by (service) (rate(cets_http_requests_total{status_class=\\\"5xx\\\"}[5m]))",
		"sum by (service, replica, route, status) (rate(cets_http_requests_total{service=~\\\"$service\\\",replica=~\\\"$replica\\\",status_class=\\\"5xx\\\"}[1m]))",
		"cets_db_pool_acquire_wait_seconds_total",
		"cets_db_lock_waiting_sessions",
		`"uid": "Prometheus"`,
		`"uid": "Loki"`,
		`"uid": "Tempo"`,
		`"uid": "Pyroscope"`,
		`"type": "grafana-pyroscope-datasource"`,
		"traces_service_graph_request_total",
		"traces_spanmetrics_calls_total",
		"Tempo Service Graph (Trace-Derived Only)",
		"K8s Deployment Topology (Not Tempo-Derived)",
		"UI/static routes: /",
		"API/health/ready routes: /api /healthz /readyz",
		"This is K8s deployment topology, not Tempo-derived node graph data.",
		"ingress-nginx emits spans",
		"backend proxy client spans",
		"enable-opentelemetry: \"true\"",
		"otlp-collector-host: \"alloy.observability.svc.cluster.local\"",
		"otlp-collector-port: \"4317\"",
		"otel-service-name: \"ingress-nginx\"",
		"kubectl -n observability port-forward svc/kube-prometheus-stack-grafana 3000:80",
		"66-verify-observability.sh",
	} {
		assert.Contains(t, combined, fragment, "K8s LGTM dashboard contract is missing %q", fragment)
	}
	for filename, dashboardText := range dashboardTexts {
		assert.NotContains(t, dashboardText, `{service_name=~"backend-.*|worker-.*"}`,
			"K8s dashboard %s must use Kubernetes labels instead of Compose service labels", filename)
	}

	dashboards := parseK8sDashboards(t, dashboardTexts)
	assertDashboardTitle(t, dashboards, "CETS Metrics RED")
	assertDashboardTitle(t, dashboards, "CETS Metrics USE")
	assertDashboardTitle(t, dashboards, "CETS Logs")
	assertDashboardTitle(t, dashboards, "CETS Traces")
	assertDashboardTitle(t, dashboards, "CETS Profiles")
	assert.True(t, containsJSONValue(dashboards["cets-logs.json"], `{namespace="cets", app="backend"} |= "otel_trace_id"`))
	assert.True(t, containsJSONValue(dashboards["cets-profiles.json"], `process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="cets-backend"}`))
	for _, fragment := range []string{"user", "ingress-nginx", "frontend", "cets-backend", "postgres", "redis", "minio"} {
		assert.True(t, containsJSONValue(dashboards["cets-traces.json"], fragment), "trace dashboard missing topology label %q", fragment)
	}
	assert.False(t, containsJSONValue(dashboards["cets-traces.json"], "ingress-nginx -> frontend -> cets-backend"),
		"trace dashboard must not imply API routes currently pass through frontend")
	verifyScript := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "66-verify-observability.sh"))
	assert.Contains(t, verifyScript, `client="user",server="ingress-nginx"`)
	assert.Contains(t, verifyScript, `client="user",server="cets-backend"`)
	assert.NotContains(t, verifyScript, `client="ingress-nginx",server="cets-backend"`)
}

func readK8sDashboardTexts(t *testing.T) map[string]string {
	t.Helper()
	dashboardDir := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "dashboards")
	entries, err := os.ReadDir(dashboardDir)
	require.NoError(t, err)
	texts := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		texts[entry.Name()] = readText(t, filepath.Join(dashboardDir, entry.Name()))
	}
	require.Len(t, texts, 5)
	return texts
}

func parseK8sDashboards(t *testing.T, texts map[string]string) map[string]map[string]interface{} {
	t.Helper()
	dashboards := make(map[string]map[string]interface{}, len(texts))
	for filename, text := range texts {
		var dashboard map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(text), &dashboard), filename)
		panels, ok := dashboard["panels"].([]interface{})
		require.True(t, ok, filename)
		assert.NotEmpty(t, panels, filename)
		dashboards[filename] = dashboard
	}
	return dashboards
}

func assertDashboardTitle(t *testing.T, dashboards map[string]map[string]interface{}, expected string) {
	t.Helper()
	for _, dashboard := range dashboards {
		if dashboard["title"] == expected {
			return
		}
	}
	assert.Failf(t, "missing dashboard title", "title %q was not found", expected)
}

func containsJSONValue(value interface{}, expected string) bool {
	switch typed := value.(type) {
	case string:
		return typed == expected || strings.Contains(typed, expected)
	case []interface{}:
		for _, item := range typed {
			if containsJSONValue(item, expected) {
				return true
			}
		}
	case map[string]interface{}:
		for _, item := range typed {
			if containsJSONValue(item, expected) {
				return true
			}
		}
	}
	return false
}
