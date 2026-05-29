package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIWorkerKindSchemasStayRuntimeOnly(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "docs", "openapi", "components", "schemas", "ops.yaml"))
	require.NoError(t, err)
	content := string(data)

	assert.Equal(t, []string{"notification", "projection", "compensation", "export"},
		extractOpenAPISchemaEnum(content, "WorkerKind"),
		"WORKER_KINDS runtime processes must stay limited to the WS4 same-binary worker kinds")

	paths, err := os.ReadFile(filepath.Join(root, "docs", "openapi", "paths", "admin-ops.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(paths), `"/admin/ops/queues/{kind}/replay":`,
		"queue replay is a same-binary CLI admin process, not a long-lived HTTP API")
}

func extractOpenAPISchemaEnum(content string, schemaName string) []string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != schemaName+":" {
			continue
		}
		for offset, candidate := range lines[i+1:] {
			if strings.TrimSpace(candidate) == "enum:" {
				return collectOpenAPIEnumValues(lines[i+1+offset+1:])
			}
			if strings.TrimSpace(candidate) != "" && !strings.HasPrefix(candidate, " ") {
				return nil
			}
		}
	}
	return nil
}

func collectOpenAPIEnumValues(lines []string) []string {
	var values []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			values = append(values, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		if trimmed != "" {
			break
		}
	}
	return values
}
