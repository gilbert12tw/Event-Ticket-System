package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSK8sTrackIsRetired(t *testing.T) {
	root := repoRoot(t)

	for _, removed := range []string{
		filepath.Join(root, ".github", "workflows", "aws-k8s-release.yml"),
		filepath.Join(root, "infra", "aws"),
		filepath.Join(root, "docs", "specs", "aws-self-managed-k8s.md"),
	} {
		_, err := os.Stat(removed)
		require.ErrorIs(t, err, os.ErrNotExist, "%s should be retired", removed)
	}

	architecture := readTextFile(t, filepath.Join(root, "docs", "ARCHITECTURE.md"))
	index := readTextFile(t, filepath.Join(root, "docs", "INDEX.md"))
	assert.NotContains(t, architecture, "AWS Self-Managed Kubernetes Experiment")
	assert.NotContains(t, index, "aws-self-managed-k8s")
}

func TestBaremetalGitOpsArgoCDContract(t *testing.T) {
	root := repoRoot(t)
	bmRoot := filepath.Join(root, "infra", "k8s", "baremetal")
	app := readTextFile(t, filepath.Join(bmRoot, "gitops", "argocd", "application.yaml"))
	kustomization := readTextFile(t, filepath.Join(bmRoot, "gitops", "app", "kustomization.yaml"))
	config := readTextFile(t, filepath.Join(bmRoot, "gitops", "app", "configmap.yaml"))
	bootstrap := readTextFile(t, filepath.Join(bmRoot, "scripts", "80-bootstrap-argocd.sh"))
	verify := readTextFile(t, filepath.Join(bmRoot, "scripts", "82-verify-release-gitops.sh"))
	workflow := readTextFile(t, filepath.Join(root, ".github", "workflows", "baremetal-cd.yml"))
	ci := readTextFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"))
	detector := readTextFile(t, filepath.Join(root, "scripts", "ci", "detect-changes.sh"))

	for _, fragment := range []string{
		"name: cets-baremetal",
		"namespace: argocd",
		"repoURL: https://github.com/gilbert12tw/Event-Ticket-System.git",
		"targetRevision: release/baremetal",
		"path: infra/k8s/baremetal/gitops/app",
		"automated:",
		"prune: true",
		"selfHeal: true",
	} {
		assert.Contains(t, app, fragment)
	}
	for _, fragment := range []string{
		"ghcr.io/gilbert12tw/event-ticket-system/cets-api",
		"ghcr.io/gilbert12tw/event-ticket-system/cets-frontend",
		"newTag:",
	} {
		assert.Contains(t, kustomization, fragment)
	}
	assert.Contains(t, config, `APP_ENV: "demo"`)
	assert.Contains(t, bootstrap, "ARGOCD_VERSION=${ARGOCD_VERSION:-v3.4.2}")
	assert.Contains(t, bootstrap, "create secret docker-registry ghcr-pull")
	assert.Contains(t, bootstrap, "create secret generic cets-runtime-env")
	assert.Contains(t, verify, "GitOps manifests must not contain unsealed Kubernetes Secrets")
	assert.Contains(t, workflow, "release/baremetal")
	assert.Contains(t, workflow, "uses: ./.github/workflows/ci.yml")
	assert.Contains(t, workflow, "run_full_ci: true")
	assert.Contains(t, workflow, "docker/build-push-action@v7")
	assert.Contains(t, workflow, "chore(baremetal-cd): update image tags")
	assert.Contains(t, ci, "workflow_call:")
	assert.Contains(t, detector, `"$event_name" == "workflow_dispatch" || "$event_name" == "workflow_call"`)
}

func TestBaremetalGitOpsDoesNotCommitPlaintextSecrets(t *testing.T) {
	root := repoRoot(t)
	gitopsRoot := filepath.Join(root, "infra", "k8s", "baremetal", "gitops")
	content := readFilesUnder(t, gitopsRoot)

	for _, forbidden := range []string{
		"kind: Secret",
		"stringData:",
		"DATABASE_URL:",
		"POSTGRES_PASSWORD",
		"TOKEN_SIGNING_SECRET:",
		"PROVIDER_TOKEN_SECRET:",
		"BOOKING_RESERVATION_HASH_SECRET:",
		"OBJECT_STORAGE_SECRET_KEY:",
		"CLOUDFLARE_TUNNEL_TOKEN",
		"GHCR_TOKEN",
	} {
		assert.NotContains(t, content, forbidden)
	}
	for _, reference := range []string{
		"name: cets-runtime-env",
		"name: cets-app-secrets",
		"name: cloudflared-token",
		"name: ghcr-pull",
	} {
		assert.Contains(t, content, reference)
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
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
		require.NoError(t, err)
		builder.WriteString(filepath.ToSlash(path))
		builder.WriteByte('\n')
		builder.Write(data)
		builder.WriteByte('\n')
		return nil
	})
	require.NoError(t, err)
	return builder.String()
}
