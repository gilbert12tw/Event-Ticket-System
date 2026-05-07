package architecture_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const maxHandwrittenGoLines = 500
const maxHandwrittenWebLines = 500

func TestHandwrittenGoFilesStayUnderLimit(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "services", "api"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || isGeneratedGo(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := bytes.Count(data, []byte{'\n'})
		if len(data) > 0 && data[len(data)-1] != '\n' {
			lines++
		}
		if lines > maxHandwrittenGoLines {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("hand-written Go files exceed %d lines: %s", maxHandwrittenGoLines, strings.Join(offenders, ", "))
	}
}

func TestHandwrittenWebFilesStayUnderLimit(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "apps", "web", "src"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isWebSource(path) || isVendoredWebSource(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := bytes.Count(data, []byte{'\n'})
		if len(data) > 0 && data[len(data)-1] != '\n' {
			lines++
		}
		if lines > maxHandwrittenWebLines {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("hand-written web files exceed %d lines: %s", maxHandwrittenWebLines, strings.Join(offenders, ", "))
	}
}

func TestTicketingDoesNotImportHTTPAPI(t *testing.T) {
	root := repoRoot(t)
	ticketingRoot := filepath.Join(root, "services", "api", "internal", "ticketing")
	var offenders []string
	err := filepath.WalkDir(ticketingRoot, func(path string, entry os.DirEntry, err error) error {
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
			if strings.Trim(imported.Path.Value, `"`) == "event-ticket-system/internal/httpapi" {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("ticketing must not import httpapi: %s", strings.Join(offenders, ", "))
	}
}

func TestPhase1DocsDoNotClaimDeferredInfrastructureIsComplete(t *testing.T) {
	root := repoRoot(t)
	docsRoot := filepath.Join(root, "docs")
	forbidden := []string{
		"phase 1 uses microservices",
		"phase 1 implements microservices",
		"phase 1 requires kafka",
		"phase 1 uses kafka",
		"phase 1 requires kubernetes",
		"phase 1 uses kubernetes",
		"phase 1 has cross-region ha",
		"phase 1 has cross-region high availability",
	}

	var offenders []string
	for _, path := range []string{filepath.Join(root, "AGENTS.md"), docsRoot} {
		err := filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := strings.ToLower(string(data))
			for _, phrase := range forbidden {
				if strings.Contains(content, phrase) {
					rel, _ := filepath.Rel(root, path)
					offenders = append(offenders, rel+": "+phrase)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("Phase 1 docs claim deferred infrastructure is complete: %s", strings.Join(offenders, "; "))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func isGeneratedGo(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".pb.go") || strings.HasSuffix(base, "_generated.go")
}

func isWebSource(path string) bool {
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func isVendoredWebSource(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.HasSuffix(normalized, "apps/web/src/components/ui/sidebar.tsx")
}
