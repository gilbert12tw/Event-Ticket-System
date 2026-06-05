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
		// Deployment infrastructure — from observability.sh / Helm values
		"cets-grafana-dashboards.yaml",
		"kind: ConfigMap",
		`grafana_dashboard: "1"`,
		"grafana_folder: Event-Ticket-System",
		"folderAnnotation: grafana_folder",
		"foldersFromFilesStructure: true",
		// New dashboard titles — one per dashboard file
		"ETS 01 — Golden Signals",
		"ETS 02 — RED Traffic Drilldown",
		"ETS 03 — Booking & Redis Pressure",
		"ETS 04 — USE Infrastructure",
		"ETS 05 — Outbox & Worker Health",
		"ETS 06 — Service Anomaly Investigation",
		// Template variable queries — present in k8s-01/02
		"CETS_REPLICA_ID",
		"fieldPath: metadata.name",
		"cets_build_info",
		"label_values(cets_build_info, service)",
		`label_values(cets_build_info{service=~\"$service\"}, replica)`,
		// RED metrics queries — present across k8s-01/02/06
		`sum by (service, route) (rate(cets_http_requests_total[1m]))`,
		`sum by (service) (rate(cets_http_requests_total{status_class=\"5xx\"}[5m]))`,
		`sum by (service, replica, route, status) (increase(cets_http_requests_total{service=~\"$service\",replica=~\"$replica\",status_class=\"5xx\"}[5m]))`,
		// Infrastructure queries — present in k8s-04
		"cets_db_pool_acquire_wait_seconds_total",
		"cets_db_lock_waiting_sessions",
		// Datasource UIDs — in every dashboard
		`"uid": "Prometheus"`,
		`"uid": "Loki"`,
		`"uid": "Tempo"`,
		`"uid": "Pyroscope"`,
		`"type": "grafana-pyroscope-datasource"`,
		// Tempo-derived service graph metrics — in k8s-06 service anomaly
		"traces_service_graph_request_total",
		"traces_spanmetrics_calls_total",
		// K8s-specific: OpenTelemetry ingress configuration — from networking.sh
		`enable-opentelemetry: "true"`,
		`otlp-collector-host: "alloy.observability.svc.cluster.local"`,
		`otlp-collector-port: "4317"`,
		`otel-service-name: "ingress-nginx"`,
		// K8s operational references — from README.md
		"kubectl -n observability port-forward svc/kube-prometheus-stack-grafana 3000:80",
		"66-verify-observability.sh",
		// Topology notes about ingress spans — from README.md
		"ingress-nginx emits spans",
		"backend proxy client spans",
	} {
		assert.Contains(t, combined, fragment, "K8s LGTM dashboard contract is missing %q", fragment)
	}

	// K8s dashboards must use Kubernetes log labels, not Docker Compose service_name labels.
	for filename, dashboardText := range dashboardTexts {
		assert.NotContains(t, dashboardText, `{service_name=~"backend-.*|worker-.*"}`,
			"K8s dashboard %s must use Kubernetes labels instead of Compose service labels", filename)
	}

	dashboards := parseK8sDashboards(t, dashboardTexts)
	// Verify all six new dashboard titles are present.
	assertDashboardTitle(t, dashboards, "ETS 01 — Golden Signals")
	assertDashboardTitle(t, dashboards, "ETS 02 — RED Traffic Drilldown")
	assertDashboardTitle(t, dashboards, "ETS 03 — Booking & Redis Pressure")
	assertDashboardTitle(t, dashboards, "ETS 04 — USE Infrastructure")
	assertDashboardTitle(t, dashboards, "ETS 05 — Outbox & Worker Health")
	assertDashboardTitle(t, dashboards, "ETS 06 — Service Anomaly Investigation")

	// k8s-06 must use Kubernetes log label selectors, not Docker Compose service_name.
	assert.True(t, containsJSONValue(dashboards["k8s-06-service-anomaly.json"], `{namespace="cets", app=~"backend|worker"}`),
		"k8s-06 error log panel must use namespace+app labels")
	// k8s-06 must have a Pyroscope CPU profile panel.
	assert.True(t, containsJSONValue(dashboards["k8s-06-service-anomaly.json"], `process_cpu:cpu:nanoseconds:cpu:nanoseconds`),
		"k8s-06 must include a Pyroscope CPU profiling panel")
	// k8s-06 must have trace-derived service graph metrics from Tempo.
	assert.True(t, containsJSONValue(dashboards["k8s-06-service-anomaly.json"], "traces_service_graph_request_total"),
		"k8s-06 must include Tempo-derived service graph metrics")
	// k8s-06 must not imply API routes currently pass through the frontend tier.
	assert.False(t, containsJSONValue(dashboards["k8s-06-service-anomaly.json"], "ingress-nginx -> frontend -> cets-backend"),
		"k8s-06 must not imply API routes pass through frontend")

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
	require.Len(t, texts, 6)
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
