package deploy

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const kubernetesObservabilityReferenceDir = "reference/kubernetes-observability"

func TestKubernetesObservabilityReferenceDocumentsKubeStateMetrics(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "kube-state-metrics.yaml"))
	scrapeExample := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "prometheus-scrape-example.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + scrapeExample + "\n" + readme

	for _, fragment := range []string{
		"kind: Namespace",
		"name: cets-observability",
		"kind: ServiceAccount",
		"kind: ClusterRole",
		"kind: ClusterRoleBinding",
		"kind: Deployment",
		"kind: Service",
		"app.kubernetes.io/name: kube-state-metrics",
		"registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.17.0",
		"resources:",
		"deployments",
		"replicasets",
		"statefulsets",
		"daemonsets",
		"services",
		"endpoints",
		"nodes",
		"namespaces",
		"persistentvolumeclaims",
		"persistentvolumes",
		"horizontalpodautoscalers",
		"verbs: [\"list\", \"watch\"]",
		"readinessProbe:",
		"livenessProbe:",
		"readOnlyRootFilesystem: true",
		"job_name: cets-kube-state-metrics",
		"kube-state-metrics.cets-observability.svc:8080",
		"signal_scope: orchestration-state",
		"reference-only examples",
	} {
		assert.Contains(t, combined, fragment, "Kubernetes observability reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDocumentsContainerMetricsStack(t *testing.T) {
	nodeExporter := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "node-exporter-daemonset.yaml"))
	sidecar := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "app-metrics-sidecar-example.yaml"))
	stack := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "metrics-stack.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := nodeExporter + "\n" + sidecar + "\n" + stack + "\n" + readme

	for _, fragment := range []string{
		"kind: DaemonSet",
		"name: node-exporter",
		"prom/node-exporter:v1.9.1",
		"hostNetwork: true",
		"hostPID: true",
		"hostPath:",
		"path: /proc",
		"path: /sys",
		"kind: StatefulSet",
		"name: prometheus",
		"prom/prometheus:v3.6.0",
		"--storage.tsdb.path=/prometheus",
		"volumeClaimTemplates:",
		"name: tsdb",
		"storage: 10Gi",
		"kind: Deployment",
		"name: grafana",
		"grafana/grafana:12.2.0",
		"grafana-datasource-reference",
		"url: http://prometheus.cets-observability.svc:9090",
		"name: cets-api-metrics-sidecar-example",
		"name: app-specific-exporter",
		"prometheuscommunity/json-exporter:v0.7.0",
		"module: [cets_ready]",
		"target: [http://localhost:8080/readyz]",
		"cets-api-metrics-sidecar-example.cets-observability.svc:7979",
		"job_name: cets-api-app-metrics",
		"job_name: cets-app-specific-exporter",
		"job_name: cets-node-exporter",
		"job_name: cets-kube-state-metrics",
		"signal_scope: app-sidecar",
		"signal_scope: use",
		"one-exporter-per-node DaemonSet",
		"StatefulSet with a persistent volume claim",
		"Grafana stays stateless",
		"logging and metrics stack as code",
	} {
		assert.Contains(t, combined, fragment, "container metrics reference is missing %q", fragment)
	}

	assert.NotContains(t, grafanaDeploymentBlock(t, stack), "volumeClaimTemplates:",
		"Grafana reference must remain stateless")
}

func TestKubernetesObservabilityReferenceDocumentsMetricsStorageAlternatives(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "metrics-storage-alternatives.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + readme

	for _, fragment := range []string{
		"name: metrics-storage-alternatives-reference",
		"prometheus-remote-write.yml: |",
		"remote_write:",
		"http://thanos-receive.cets-observability.svc:19291/api/v1/receive",
		"http://cortex.cets-observability.svc:9009/api/v1/push",
		"grafana-agent.yml: |",
		"app.kubernetes.io/name: thanos-receive",
		"quay.io/thanos/thanos:v0.40.1",
		"--receive.replication-factor=1",
		"app.kubernetes.io/name: cortex",
		"quay.io/cortexproject/cortex:v1.18.0",
		"blocks_storage:",
		"kind: StatefulSet",
		"volumeClaimTemplates:",
		"storage: 20Gi",
		"app.kubernetes.io/name: grafana-agent",
		"grafana/agent:v0.43.4",
		"Thanos Receive, Cortex, and Grafana Agent remote-write",
		"long-term metrics storage alternatives",
	} {
		assert.Contains(t, combined, fragment, "metrics storage alternatives reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDocumentsContainerLogPipeline(t *testing.T) {
	sidecar := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "log-sidecar-example.yaml"))
	plg := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "plg-log-stack.yaml"))
	efk := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "efk-log-stack.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := sidecar + "\n" + plg + "\n" + efk + "\n" + readme

	for _, fragment := range []string{
		"name: cets-api-log-sidecar-example",
		"name: fluent-bit",
		"cr.fluentbit.io/fluent/fluent-bit:4.0.13",
		"name: stdout-spool",
		"emptyDir: {}",
		"tee -a /var/log/cets/stdout.log",
		"Name          tail",
		"Path          /var/log/cets/stdout.log",
		"Parser        json",
		"Name          loki",
		"Host          loki.cets-observability.svc",
		"service.name cets-api",
		"kind: StatefulSet",
		"name: loki",
		"grafana/loki:3.6.0",
		"storage: 10Gi",
		"name: grafana-log-viewer",
		"grafana/grafana:12.4.0",
		"type: loki",
		"derivedFields:",
		"name: Tempo trace",
		"name: elasticsearch",
		"docker.elastic.co/elasticsearch/elasticsearch:9.2.2",
		"storage: 20Gi",
		"name: kibana",
		"docker.elastic.co/kibana/kibana:9.2.2",
		"ELASTICSEARCH_HOSTS",
		"Fluent Bit sidecar",
		"PLG pattern",
		"EFK pattern",
		"logging and metrics stack as code",
	} {
		assert.Contains(t, combined, fragment, "container log pipeline reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDocumentsLogCollectorAlternatives(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "log-collector-alternatives.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + readme

	for _, fragment := range []string{
		"name: fluentd-log-collector-reference",
		"app.kubernetes.io/name: fluentd",
		"fluent.conf: |",
		"@type tail",
		"path /var/log/containers/cets-api-*.log",
		"@type json",
		"collector fluentd",
		"@type loki",
		"url http://loki.cets-observability.svc:3100",
		"name: vector-log-collector-reference",
		"app.kubernetes.io/name: vector",
		"vector.yaml: |",
		"type: kubernetes_logs",
		"include_paths:",
		"type: remap",
		"parse_json!(.message)",
		"collector = \"vector\"",
		"type: loki",
		"endpoint: http://loki.cets-observability.svc:3100",
		"Fluentd and Vector collector config alternatives",
	} {
		assert.Contains(t, combined, fragment, "log collector alternatives reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDocumentsLogAnalysisAlternatives(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "log-analysis-alternatives.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + readme

	for _, fragment := range []string{
		"name: logstash-log-pipeline-reference",
		"app.kubernetes.io/name: logstash",
		"logstash.conf: |",
		"input {",
		"http {",
		"codec => json",
		"output {",
		"opensearch {",
		"hosts => [\"http://opensearch.cets-observability.svc:9200\"]",
		"index => \"cets-api-logs-%{+YYYY.MM.dd}\"",
		"docker.elastic.co/logstash/logstash:9.2.2",
		"name: opensearch",
		"app.kubernetes.io/name: opensearch",
		"opensearchproject/opensearch:3.3.2",
		"kind: StatefulSet",
		"volumeClaimTemplates:",
		"storage: 20Gi",
		"Logstash + OpenSearch analysis pipeline alternative",
		"indexed search",
	} {
		assert.Contains(t, combined, fragment, "log analysis alternatives reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDocumentsOpenTelemetryCollector(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "opentelemetry-collector.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + readme

	for _, fragment := range []string{
		"kind: ConfigMap",
		"name: opentelemetry-collector-reference-config",
		"receivers:",
		"otlp:",
		"grpc:",
		"endpoint: 0.0.0.0:4317",
		"http:",
		"endpoint: 0.0.0.0:4318",
		"processors:",
		"batch:",
		"exporters:",
		"otlphttp/tempo:",
		"endpoint: http://tempo.cets-observability.svc:4318",
		"loki:",
		"endpoint: http://loki.cets-observability.svc:3100/loki/api/v1/push",
		"traces:",
		"logs:",
		"extensions: [health_check]",
		"kind: Deployment",
		"otel/opentelemetry-collector-contrib:0.142.0",
		"name: otlp-grpc",
		"name: otlp-http",
		"kind: Service",
		"vendor-neutral OpenTelemetry Collector",
		"routes traces to Tempo and logs to Loki",
	} {
		assert.Contains(t, combined, fragment, "OpenTelemetry collector reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDocumentsTraceBackendAlternatives(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "trace-backend-alternatives.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + readme

	for _, fragment := range []string{
		"name: jaeger-trace-backend",
		"app.kubernetes.io/name: jaeger",
		"jaegertracing/all-in-one:1.62.0",
		"name: query",
		"containerPort: 16686",
		"name: otlp-grpc",
		"name: otlp-http",
		"name: zipkin",
		"containerPort: 9411",
		"name: zipkin-trace-backend",
		"app.kubernetes.io/name: zipkin",
		"openzipkin/zipkin:3.5.1",
		"path: /health",
		"kind: Service",
		"type: ClusterIP",
		"Jaeger and Zipkin trace backend alternatives",
		"span timelines and dependency graphs",
	} {
		assert.Contains(t, combined, fragment, "trace backend alternatives reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDocumentsAppLifecycleAndServiceDiscovery(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "app-lifecycle-service-discovery.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + readme

	for _, fragment := range []string{
		"kind: Deployment",
		"name: cets-api-lifecycle-example",
		"replicas: 3",
		"type: RollingUpdate",
		"maxUnavailable: 0",
		"maxSurge: 1",
		"restartPolicy: Always",
		"readinessProbe:",
		"path: /readyz",
		"livenessProbe:",
		"path: /healthz",
		"topologySpreadConstraints:",
		"topologyKey: kubernetes.io/hostname",
		"podAntiAffinity:",
		"kind: Service",
		"name: cets-api",
		"type: ClusterIP",
		"targetPort: http",
		"kind: PodDisruptionBudget",
		"minAvailable: 2",
		"kind: HorizontalPodAutoscaler",
		"minReplicas: 3",
		"maxReplicas: 9",
		"name: cpu",
		"averageUtilization: 70",
		"name: cets_http_requests_per_second",
		"averageValue: \"25\"",
		"rolling updates",
		"stable Service discovery",
		"HPA",
		"PodDisruptionBudget",
	} {
		assert.Contains(t, combined, fragment, "app lifecycle reference is missing %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceStaysOutsideProductRuntime(t *testing.T) {
	reference := readFilesUnder(t, kubernetesObservabilityReferenceDir)
	compose := readDeployText(t, "compose.yaml")
	phase3Compose := readDeployText(t, "compose.phase3-ha.yaml")

	assert.NotContains(t, compose, kubernetesObservabilityReferenceDir)
	assert.NotContains(t, phase3Compose, kubernetesObservabilityReferenceDir)
	for _, fragment := range []string{
		"DATABASE_URL",
		"REDIS_URL",
		"TOKEN_SIGNING_SECRET",
		"OBJECT_STORAGE",
		"MAILER_",
		"OTEL_",
		"PYROSCOPE_",
		"secrets",
		"signed_token",
		"signed_qr_token",
		"provider_secret",
		"raw_recipient_email",
		"raw_idempotency_key",
	} {
		assert.NotContains(t, reference, fragment, "reference manifests must not carry product runtime config %q", fragment)
	}
}

func TestKubernetesObservabilityReferenceDoesNotReintroduceActiveK8sManifests(t *testing.T) {
	_, err := os.Stat("k8s")
	assert.True(t, os.IsNotExist(err), "active K8s manifests must stay absent")

	_, err = os.Stat(kubernetesObservabilityReferenceDir)
	require.NoError(t, err)
}

func TestKubernetesObservabilityReferenceManifestsParseAsYAML(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(kubernetesObservabilityReferenceDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data := readDeployText(t, file)
			decoder := yaml.NewDecoder(strings.NewReader(data))
			var docs int
			for {
				var document map[string]interface{}
				err := decoder.Decode(&document)
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				if len(document) == 0 {
					continue
				}
				docs++
				assert.NotEmpty(t, document["apiVersion"])
				assert.NotEmpty(t, document["kind"])
			}
			assert.Greater(t, docs, 0, "manifest must contain Kubernetes documents")
		})
	}
}

func readDeployText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestKubernetesObservabilityReferenceRBACIsReadOnly(t *testing.T) {
	manifest := strings.ToLower(readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "kube-state-metrics.yaml")))

	for _, forbiddenVerb := range []string{"create", "update", "patch", "delete", "deletecollection"} {
		assert.NotContains(t, manifest, forbiddenVerb, "kube-state-metrics RBAC must remain read-only")
	}
}

func grafanaDeploymentBlock(t *testing.T, manifest string) string {
	t.Helper()
	start := strings.Index(manifest, "kind: Deployment\nmetadata:\n  name: grafana")
	require.NotEqual(t, -1, start, "grafana Deployment must exist")
	rest := manifest[start:]
	next := strings.Index(rest[len("kind: Deployment\n"):], "\n---\n")
	if next == -1 {
		return rest
	}
	return rest[:len("kind: Deployment\n")+next]
}
