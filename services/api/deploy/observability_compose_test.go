package deploy

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

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
		"alertmanager:",
		"image: prom/alertmanager:v0.28.1",
		"--config.file=/etc/alertmanager/alertmanager.yml",
		"--storage.path=/alertmanager",
		"${ALERTMANAGER_PORT:-9093}:9093",
		"./observability/alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro",
		"alertmanager_data:",
		"victoria-metrics:",
		"image: victoriametrics/victoria-metrics:v1.129.1",
		"-storageDataPath=/storage",
		"-retentionPeriod=30d",
		"${VICTORIA_METRICS_PORT:-8428}:8428",
		"victoria_metrics_data:",
		"loki:",
		"image: grafana/loki:3.5.0",
		"-config.file=/etc/loki/config.yml",
		"${LOKI_PORT:-3100}:3100",
		"./observability/loki.yml:/etc/loki/config.yml:ro",
		"promtail:",
		"image: grafana/promtail:3.5.0",
		"-config.file=/etc/promtail/config.yml",
		"./observability/promtail.yml:/etc/promtail/config.yml:ro",
		"/var/run/docker.sock:/var/run/docker.sock:ro",
		"tempo:",
		"image: grafana/tempo:2.8.2",
		"-config.file=/etc/tempo/config.yml",
		"${TEMPO_PORT:-3200}:3200",
		"${TEMPO_OTLP_GRPC_PORT:-4317}:4317",
		"${TEMPO_OTLP_HTTP_PORT:-4318}:4318",
		"./observability/tempo.yml:/etc/tempo/config.yml:ro",
		"blackbox-exporter:",
		"image: prom/blackbox-exporter:v0.27.0",
		"--config.file=/etc/blackbox_exporter/config.yml",
		"${BLACKBOX_EXPORTER_PORT:-9115}:9115",
		"./observability/blackbox.yml:/etc/blackbox_exporter/config.yml:ro",
		"redis-exporter:",
		"image: oliver006/redis_exporter:v1.77.0",
		"--redis.addr=redis://redis:6379",
		"${REDIS_EXPORTER_PORT:-9121}:9121",
		"node-exporter:",
		"image: prom/node-exporter:v1.9.1",
		"--path.rootfs=/host",
		"${NODE_EXPORTER_PORT:-9100}:9100",
		"/:/host:ro,rslave",
		"cadvisor:",
		"image: gcr.io/cadvisor/cadvisor:v0.52.1",
		"privileged: true",
		"${CADVISOR_PORT:-8081}:8080",
		"/:/rootfs:ro",
		"/var/run:/var/run:ro",
		"/sys:/sys:ro",
		"/var/lib/docker/:/var/lib/docker:ro",
		"/dev/disk/:/dev/disk:ro",
		"pyroscope:",
		"image: grafana/pyroscope:1.18.1",
		"${PYROSCOPE_PORT:-4040}:4040",
		"pyroscope_data:",
		"./observability/rules:/etc/prometheus/rules:ro",
		"grafana:",
		"image: grafana/grafana:12.2.0",
		"GF_SECURITY_ADMIN_USER: ${GRAFANA_ADMIN_USER:-admin}",
		"GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_ADMIN_PASSWORD:-admin}",
		"${GRAFANA_PORT:-3000}:3000",
		"./observability/grafana/provisioning/datasources:/etc/grafana/provisioning/datasources:ro",
		"./observability/grafana/dashboards:/var/lib/grafana/dashboards:ro",
		"PROMETHEUS_PORT=9090",
		"ALERTMANAGER_PORT=9093",
		"VICTORIA_METRICS_PORT=8428",
		"BLACKBOX_EXPORTER_PORT=9115",
		"REDIS_EXPORTER_PORT=9121",
		"NODE_EXPORTER_PORT=9100",
		"CADVISOR_PORT=8081",
		"LOKI_PORT=3100",
		"TEMPO_PORT=3200",
		"TEMPO_OTLP_GRPC_PORT=4317",
		"TEMPO_OTLP_HTTP_PORT=4318",
		"OTEL_TRACES_ENABLED=false",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://tempo:4318",
		"OTEL_SERVICE_NAME=cets-api",
		"OTEL_SERVICE_VERSION=local-compose",
		"PYROSCOPE_PORT=4040",
		"PYROSCOPE_ENABLED=false",
		"PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040",
		"PYROSCOPE_APPLICATION_NAME=cets-api",
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
	alertmanager, err := os.ReadFile("observability/alertmanager.yml")
	require.NoError(t, err)
	prometheusDatasource, err := os.ReadFile("observability/grafana/provisioning/datasources/prometheus.yml")
	require.NoError(t, err)
	rollups, err := os.ReadFile("observability/rules/cets-rollups.yml")
	require.NoError(t, err)
	loki, err := os.ReadFile("observability/loki.yml")
	require.NoError(t, err)
	promtail, err := os.ReadFile("observability/promtail.yml")
	require.NoError(t, err)
	tempo, err := os.ReadFile("observability/tempo.yml")
	require.NoError(t, err)
	dashboardFile, err := os.ReadFile("observability/grafana/dashboards/cets-observability.json")
	require.NoError(t, err)
	goldenSignalsDashboardFile, err := os.ReadFile("observability/grafana/dashboards/cets-golden-signals.json")
	require.NoError(t, err)
	useDashboardFile, err := os.ReadFile("observability/grafana/dashboards/cets-use-exporters.json")
	require.NoError(t, err)

	combined := strings.Join([]string{
		string(prometheus),
		string(blackbox),
		string(alertmanager),
		string(prometheusDatasource),
		string(rollups),
		string(loki),
		string(promtail),
		string(tempo),
		string(dashboardFile),
		string(goldenSignalsDashboardFile),
		string(useDashboardFile),
	}, "\n")
	required := []string{
		"job_name: cets-app",
		"metrics_path: /metrics",
		"rule_files:",
		"/etc/prometheus/rules/*.yml",
		"alerting:",
		"alertmanagers:",
		"alertmanager:9093",
		"remote_write:",
		"http://victoria-metrics:8428/api/v1/write",
		"receiver: local-review",
		"group_by:",
		"severity",
		"repeat_interval: 4h",
		"app:8080",
		"job_name: cets-blackbox",
		"metrics_path: /probe",
		"http://app:8080/",
		"http://app:8080/healthz",
		"http://app:8080/readyz",
		"probe_scope: blackbox",
		"blackbox-exporter:9115",
		"job_name: cets-node-exporter",
		"node-exporter:9100",
		"job_name: cets-cadvisor",
		"cadvisor:8080",
		"job_name: cets-redis-exporter",
		"redis-exporter:9121",
		"service: cets-redis",
		"signal_scope: backing-service",
		"signal_scope: use",
		"prober: http",
		"probe_success{probe_scope=\\\"blackbox\\\"}",
		"probe_duration_seconds{probe_scope=\\\"blackbox\\\"}",
		"url: http://prometheus:9090",
		"name: VictoriaMetrics",
		"uid: VictoriaMetrics",
		"url: http://victoria-metrics:8428",
		"type: loki",
		"url: http://loki:3100",
		"derivedFields:",
		"name: Tempo trace",
		`matcherRegex: '"otel_trace_id":"([a-f0-9]{32})"'`,
		"datasourceUid: Tempo",
		`url: "$${__value.raw}"`,
		`urlDisplayLabel: "Open trace"`,
		"type: tempo",
		"url: http://tempo:3200",
		"type: grafana-pyroscope-datasource",
		"url: http://pyroscope:4040",
		"url: http://loki:3100/loki/api/v1/push",
		"job_name: cets-compose",
		"regex: \"(app|worker)\"",
		"otlp:",
		"endpoint: 0.0.0.0:4317",
		"endpoint: 0.0.0.0:4318",
		"cets_http_requests_total",
		"cets_http_request_seconds_bucket",
		"cets_db_pool_conns",
		"cets_db_lock_waiting_sessions",
		"cets_outbox_oldest_lag_seconds",
		"node_cpu_seconds_total{mode=\\\"idle\\\", signal_scope=\\\"use\\\"}",
		"node_load1{signal_scope=\\\"use\\\"}",
		"node_memory_MemAvailable_bytes{signal_scope=\\\"use\\\"}",
		"node_vmstat_pgpgin{signal_scope=\\\"use\\\"}",
		"container_cpu_usage_seconds_total{name!=\\\"\\\", signal_scope=\\\"use\\\"}",
		"container_memory_working_set_bytes{name!=\\\"\\\", signal_scope=\\\"use\\\"}",
		"redis_memory_used_bytes{signal_scope=\\\"backing-service\\\"}",
		"redis_connected_clients{signal_scope=\\\"backing-service\\\"}",
		"cets-red-rollups",
		"cets:http_requests:rate5m",
		"cets:http_request_duration_seconds:p95_5m",
		"cets:http_request_duration_seconds:p99_5m",
		"cets:outbox_oldest_lag_seconds:max5m",
		"cets:db_lock_waiting_sessions:max5m",
		"CETS Golden Signals",
		"Golden Signal: Traffic",
		"Golden Signal: Errors",
		"Golden Signal: Success Latency",
		"Golden Signal: Error Latency",
		"Golden Signal: Saturation - DB Pool",
		"Golden Signal: Saturation - Queue Lag",
		"Golden Signal: Saturation - DB Locks",
		"sum(rate(cets_http_requests_total[1m]))",
		"cets_http_requests_total{status_class=~\\\"4xx|5xx\\\"}",
		"cets_http_request_seconds_bucket{status_class=~\\\"2xx|3xx\\\"}",
		"cets_http_request_seconds_bucket{status_class=~\\\"4xx|5xx\\\"}",
		"rate(cets_db_pool_acquire_wait_seconds_total[5m]) / clamp_min(rate(cets_db_pool_acquire_count_total[5m]), 1)",
		"max(cets_outbox_oldest_lag_seconds)",
		"USE CPU Utilization",
		"USE CPU Saturation",
		"USE Memory Utilization",
		"USE Memory Saturation",
		"Container CPU Usage",
		"Container Memory Working Set",
		"Redis Memory Used",
		"Redis Connected Clients",
	}
	for _, fragment := range required {
		assert.Contains(t, combined, fragment, "observability provisioning is missing %q", fragment)
	}

	var dashboard map[string]interface{}
	require.NoError(t, json.Unmarshal(dashboardFile, &dashboard))
	assert.Equal(t, "CETS Observability", dashboard["title"])

	var goldenSignalsDashboard map[string]interface{}
	require.NoError(t, json.Unmarshal(goldenSignalsDashboardFile, &goldenSignalsDashboard))
	assert.Equal(t, "CETS Golden Signals", goldenSignalsDashboard["title"])

	var useDashboard map[string]interface{}
	require.NoError(t, json.Unmarshal(useDashboardFile, &useDashboard))
	assert.Equal(t, "CETS USE Exporters", useDashboard["title"])

	var datasourceProvisioning grafanaDatasourceProvisioning
	require.NoError(t, yaml.Unmarshal(prometheusDatasource, &datasourceProvisioning))
	lokiDatasource := datasourceProvisioning.findDatasource("Loki")
	require.NotNil(t, lokiDatasource)
	require.Len(t, lokiDatasource.JSONData.DerivedFields, 1)
	assert.Equal(t, "Tempo trace", lokiDatasource.JSONData.DerivedFields[0].Name)
	assert.Equal(t, `"otel_trace_id":"([a-f0-9]{32})"`, lokiDatasource.JSONData.DerivedFields[0].MatcherRegex)
	assert.Equal(t, "Tempo", lokiDatasource.JSONData.DerivedFields[0].DatasourceUID)
	assert.Equal(t, "$${__value.raw}", lokiDatasource.JSONData.DerivedFields[0].URL)

	pyroscopeDatasource := datasourceProvisioning.findDatasource("Pyroscope")
	require.NotNil(t, pyroscopeDatasource)
	assert.Equal(t, "grafana-pyroscope-datasource", pyroscopeDatasource.Type)
	assert.Equal(t, "http://pyroscope:4040", pyroscopeDatasource.URL)

	victoriaMetricsDatasource := datasourceProvisioning.findDatasource("VictoriaMetrics")
	require.NotNil(t, victoriaMetricsDatasource)
	assert.Equal(t, "prometheus", victoriaMetricsDatasource.Type)
	assert.Equal(t, "http://victoria-metrics:8428", victoriaMetricsDatasource.URL)
}

func TestOptionalLogTraceBackendsDoNotChangeAppRuntimeContracts(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	composeText := string(compose)

	for _, serviceName := range []string{"app", "worker"} {
		serviceBlock := composeServiceBlock(t, composeText, serviceName)
		assert.NotContains(t, serviceBlock, "loki:", "%s must not depend on Loki for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "promtail:", "%s must not depend on Promtail for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "\n      tempo:", "%s must not depend on Tempo for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "alertmanager:", "%s must not depend on Alertmanager for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "LOKI_", "%s must continue to write logs to stdout/stderr", serviceName)
		assert.NotContains(t, serviceBlock, "TEMPO_", "%s must not require Tempo to serve product traffic", serviceName)
		assert.NotContains(t, serviceBlock, "ALERTMANAGER_", "%s must not require Alertmanager to serve product traffic", serviceName)
		assert.NotContains(t, serviceBlock, "\n      pyroscope:", "%s must not depend on Pyroscope for runtime behavior", serviceName)
	}
	appBlock := composeServiceBlock(t, composeText, "app")
	assert.Contains(t, appBlock, "OTEL_TRACES_ENABLED: ${OTEL_TRACES_ENABLED:-false}")
	assert.Contains(t, appBlock, "OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT:-http://tempo:4318}")
	assert.Contains(t, appBlock, "PYROSCOPE_ENABLED: ${PYROSCOPE_ENABLED:-false}")
	assert.Contains(t, appBlock, "PYROSCOPE_SERVER_ADDRESS: ${PYROSCOPE_SERVER_ADDRESS:-http://pyroscope:4040}")
	assert.Contains(t, appBlock, "PYROSCOPE_APPLICATION_NAME: ${PYROSCOPE_APPLICATION_NAME:-cets-api}")
	workerBlock := composeServiceBlock(t, composeText, "worker")
	assert.NotContains(t, workerBlock, "OTEL_", "worker must not enable trace export without worker span coverage")
	assert.NotContains(t, workerBlock, "PYROSCOPE_", "worker must not enable profiling without worker profile coverage")
}

func TestBlackboxProbingStaysOutsideProductBehavior(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	prometheus, err := os.ReadFile("observability/prometheus.yml")
	require.NoError(t, err)
	composeText := string(compose)
	blackboxJob := prometheusJobBlock(t, string(prometheus), "cets-blackbox")

	assert.NotContains(t, blackboxJob, "/api/v1/", "black-box probes must not exercise product APIs")
	assert.NotContains(t, blackboxJob, "Authorization:", "black-box probes must not depend on product credentials")
	assert.NotContains(t, composeText, "app:\n    depends_on:\n      blackbox-exporter:", "app must not depend on probe health")
	assert.NotContains(t, composeText, "worker:\n    depends_on:\n      blackbox-exporter:", "worker must not depend on probe health")
}

func TestInfraExportersStayOutsideProductRuntimeContracts(t *testing.T) {
	compose, err := os.ReadFile("compose.yaml")
	require.NoError(t, err)
	composeText := string(compose)

	for _, serviceName := range []string{"app", "worker"} {
		serviceBlock := composeServiceBlock(t, composeText, serviceName)
		assert.NotContains(t, serviceBlock, "victoria-metrics:", "%s must not depend on VictoriaMetrics for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "redis-exporter:", "%s must not depend on Redis exporter for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "node-exporter:", "%s must not depend on node exporter for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "cadvisor:", "%s must not depend on cAdvisor for runtime behavior", serviceName)
		assert.NotContains(t, serviceBlock, "VICTORIA_METRICS", "%s must not receive long-term metrics storage config", serviceName)
		assert.NotContains(t, serviceBlock, "REDIS_EXPORTER", "%s must not receive Redis exporter config", serviceName)
		assert.NotContains(t, serviceBlock, "NODE_EXPORTER", "%s must not receive exporter config", serviceName)
		assert.NotContains(t, serviceBlock, "CADVISOR", "%s must not receive cAdvisor config", serviceName)
	}
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

type grafanaDatasourceProvisioning struct {
	Datasources []grafanaDatasource `yaml:"datasources"`
}

func (p grafanaDatasourceProvisioning) findDatasource(name string) *grafanaDatasource {
	for i := range p.Datasources {
		if p.Datasources[i].Name == name {
			return &p.Datasources[i]
		}
	}
	return nil
}

type grafanaDatasource struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	URL      string `yaml:"url"`
	JSONData struct {
		DerivedFields []grafanaDerivedField `yaml:"derivedFields"`
	} `yaml:"jsonData"`
}

type grafanaDerivedField struct {
	Name            string `yaml:"name"`
	MatcherRegex    string `yaml:"matcherRegex"`
	DatasourceUID   string `yaml:"datasourceUid"`
	URL             string `yaml:"url"`
	URLDisplayLabel string `yaml:"urlDisplayLabel"`
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

func composeServiceBlock(t *testing.T, composeText string, serviceName string) string {
	t.Helper()

	normalized := strings.ReplaceAll(composeText, "\r\n", "\n")
	pattern := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(serviceName) + `:\n(?:    .*(?:\n|$))*`)
	block := pattern.FindString(normalized)
	require.NotEmpty(t, block, "compose service %s must exist", serviceName)
	return block
}

func prometheusJobBlock(t *testing.T, prometheusText string, jobName string) string {
	t.Helper()

	normalized := strings.ReplaceAll(prometheusText, "\r\n", "\n")
	marker := "  - job_name: " + jobName + "\n"
	start := strings.Index(normalized, marker)
	require.NotEqual(t, -1, start, "prometheus job %s must exist", jobName)

	rest := normalized[start:]
	nextStart := strings.Index(rest[len(marker):], "\n  - job_name: ")
	if nextStart == -1 {
		return rest
	}
	return rest[:len(marker)+nextStart]
}
