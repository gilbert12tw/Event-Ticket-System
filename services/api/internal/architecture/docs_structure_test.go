package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentationIndexTracksCurrentSpecsAndArchive(t *testing.T) {
	root := repoRoot(t)
	indexPath := filepath.Join(root, "docs", "INDEX.md")
	index, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	indexContent := string(index)

	activeSpecs := []string{
		"docs/specs/backend-directory-architecture.md",
		"docs/specs/phase1-production-upper-bound.md",
		"docs/specs/phase1-product-requirements.md",
		"docs/specs/phase1-nfr-and-capacity.md",
		"docs/specs/phase1-e2e-test-paths.md",
		"docs/specs/evolution-boundaries.md",
	}
	for _, rel := range activeSpecs {
		require.FileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
		assert.Contains(t, indexContent, rel)
	}
	assert.NotContains(t, indexContent, "docs/archive/")
	assert.NoDirExists(t, filepath.Join(root, "docs", "archive"))
}

func TestSonarScannerConfigExcludesArchiveAndGeneratedOutputs(t *testing.T) {
	root := repoRoot(t)
	config, err := os.ReadFile(filepath.Join(root, "sonar-project.properties"))
	require.NoError(t, err)
	content := strings.ReplaceAll(string(config), "\n", ",")

	for _, fragment := range []string{
		"**/.cache/**",
		"**/.turbo/**",
		"**/coverage/**",
		"services/api/internal/httpapi/static/assets/**",
		"sonar.host.url=${env.SONAR_HOST_URL}",
		"sonar.token=${env.SONAR_TOKEN}",
		"sonar.go.coverage.reportPaths=services/api/coverage.sonar.out",
		"sonar.javascript.lcov.reportPaths=apps/web/coverage/lcov.info",
	} {
		assert.Contains(t, content, fragment)
	}
	assert.NotContains(t, content, "docs/archive/**")
}

func TestCIWorkflowKeepsSonarScannerLocalOnly(t *testing.T) {
	root := repoRoot(t)
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	require.NoError(t, err)
	packageJSON, err := os.ReadFile(filepath.Join(root, "package.json"))
	require.NoError(t, err)

	content := string(workflow)
	for _, fragment := range []string{
		"run_sonar:",
		"sonar-analysis:",
		"INPUT_RUN_SONAR",
		"SONAR_HOST_URL: ${{ vars.SONAR_HOST_URL }}",
		"SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}",
		"sonarsource/sonar-scanner-cli",
	} {
		assert.NotContains(t, content, fragment)
	}
	assert.Contains(t, string(packageJSON), `"test:coverage": "scripts/coverage/sonar-go-coverage.sh && pnpm --filter cets-web test:coverage"`)
	assert.Contains(t, string(packageJSON), `"sonar:scan": "pnpm test:coverage && sonar-scanner"`)
}
