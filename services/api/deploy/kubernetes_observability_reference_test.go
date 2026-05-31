package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
