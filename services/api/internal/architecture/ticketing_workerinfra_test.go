package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTicketingWorkerInfraDoesNotDependOnDomainOrDatabaseAdapters(t *testing.T) {
	root := repoRoot(t)
	workerInfraRoot := filepath.Join(root, "services", "api", "internal", "ticketing", "workerinfra")
	forbiddenPrefixes := []string{
		"event-ticket-system/internal/ticketing",
		"database/sql",
		"github.com/jackc/pgx",
	}

	var offenders []string
	err := filepath.WalkDir(workerInfraRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if hasAnyPrefix(importPath, forbiddenPrefixes) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel+": "+importPath)
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offenders, "workerinfra must stay adapter-facing and must not import domain orchestration or DB adapters")
}
