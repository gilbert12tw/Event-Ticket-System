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
		"full metrics stack as code",
	} {
		assert.Contains(t, combined, fragment, "container metrics reference is missing %q", fragment)
	}

	assert.NotContains(t, grafanaDeploymentBlock(t, stack), "volumeClaimTemplates:",
		"Grafana reference must remain stateless")
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
