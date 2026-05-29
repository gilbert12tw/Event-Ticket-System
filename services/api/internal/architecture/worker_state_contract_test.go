package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase2WorkersDoNotWriteLocalDurableState(t *testing.T) {
	root := repoRoot(t)
	scanRoots := []string{
		filepath.Join(root, "services", "api", "cmd", "cets"),
		filepath.Join(root, "services", "api", "internal", "ticketing"),
	}
	forbidden := map[string]struct{}{
		"os.Create":        {},
		"os.CreateTemp":    {},
		"os.Mkdir":         {},
		"os.MkdirAll":      {},
		"os.OpenFile":      {},
		"os.Remove":        {},
		"os.RemoveAll":     {},
		"os.Rename":        {},
		"os.WriteFile":     {},
		"ioutil.WriteFile": {},
	}

	var offenders []string
	for _, scanRoot := range scanRoots {
		offenders = append(offenders, localStateWriteOffenders(t, root, scanRoot, forbidden)...)
	}
	assert.Empty(t, offenders,
		"Phase 2 WS4 workers must keep durable state in backing services, not local files: %s",
		strings.Join(offenders, "; "))
}

func localStateWriteOffenders(t *testing.T, root string, scanRoot string, forbidden map[string]struct{}) []string {
	t.Helper()
	var offenders []string
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			name := ident.Name + "." + selector.Sel.Name
			if _, forbiddenCall := forbidden[name]; forbiddenCall {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel+": "+name)
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	return offenders
}
