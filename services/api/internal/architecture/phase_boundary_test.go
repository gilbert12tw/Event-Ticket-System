package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase3DocsDeclareLocalSimulationBoundary(t *testing.T) {
	root := repoRoot(t)
	spec, err := os.ReadFile(filepath.Join(root, "docs", "specs", "phase3-local-ha-compose-lgtm.md"))
	require.NoError(t, err)
	architecture, err := os.ReadFile(filepath.Join(root, "docs", "ARCHITECTURE.md"))
	require.NoError(t, err)

	combined := strings.ToLower(string(spec) + "\n" + string(architecture))
	for _, phrase := range []string{
		"single-machine local",
		"local simulation",
		"docker compose",
		"explicit replica services",
		"does not claim production",
		"multi-az",
		"postgresql remains the final truth",
		"grafana",
		"loki",
		"tempo",
		"prometheus",
		"pyroscope",
		"microservices remain a deferred decision gate",
	} {
		assert.Contains(t, combined, phrase, "Phase 3 local simulation boundary is missing %q", phrase)
	}
}

func TestPhase2WorkersUseSingleGoCommandEntrypoint(t *testing.T) {
	root := repoRoot(t)

	dockerfile, err := os.ReadFile(filepath.Join(root, "services", "api", "Dockerfile"))
	require.NoError(t, err)
	assert.Contains(t, string(dockerfile), "go build -o /out/cets ./cmd/cets",
		"the deployable API image must build the single cets command binary")

	entries, err := os.ReadDir(filepath.Join(root, "services", "api", "cmd"))
	require.NoError(t, err)

	var commandDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			commandDirs = append(commandDirs, entry.Name())
		}
	}

	assert.Contains(t, commandDirs, "cets",
		"workers must remain same-binary cets command modes, not separately built worker services")

	for _, relPath := range []string{
		filepath.Join("services", "api", "deploy", "compose.yaml"),
		filepath.Join("services", "api", "deploy", "compose.worker-isolation.yaml"),
		filepath.Join("services", "api", "deploy", "compose.phase3-ha.yaml"),
	} {
		data, err := os.ReadFile(filepath.Join(root, relPath))
		require.NoError(t, err)
		content := string(data)
		assert.NotContains(t, content, "reservation_experiment",
			"Phase 2 WS4 worker deployments must not use diagnostic or experiment binaries: "+relPath)
		assert.NotContains(t, content, "cmd/",
			"Phase 2 WS4 worker deployments must use the built cets binary command modes: "+relPath)
	}

	compose, err := os.ReadFile(filepath.Join(root, "services", "api", "deploy", "compose.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(compose), `command: ["worker"]`,
		"Compose worker must remain the cets worker command mode")
}
