package architecture_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"event-ticket-system/internal/eventcontract"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	assert.Empty(t, offenders, "hand-written Go files exceed %d lines: %s", maxHandwrittenGoLines, strings.Join(offenders, ", "))
}

func TestHandwrittenWebFilesStayUnderLimit(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "apps", "web", "src"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isWebSource(path) {
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
	require.NoError(t, err)
	assert.Empty(t, offenders, "hand-written web files exceed %d lines: %s", maxHandwrittenWebLines, strings.Join(offenders, ", "))
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
	require.NoError(t, err)
	assert.Empty(t, offenders, "ticketing must not import httpapi: %s", strings.Join(offenders, ", "))
}

func TestTicketingNotificationPortDoesNotImportSMTPAdapter(t *testing.T) {
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
			if strings.Trim(imported.Path.Value, `"`) == "net/smtp" {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel)
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offenders, "ticketing must depend on NotificationSender, not the SMTP adapter: %s", strings.Join(offenders, ", "))
}

func TestPostgresMigrationDDLStaysInVersionedSQL(t *testing.T) {
	root := repoRoot(t)
	migratePath := filepath.Join(root, "services", "api", "internal", "postgres", "migrate.go")
	migrateCode, err := os.ReadFile(migratePath)
	require.NoError(t, err)

	for _, fragment := range []string{
		"CREATE TABLE",
		"CREATE INDEX",
		"CREATE UNIQUE INDEX",
		"ALTER TABLE",
		"INSERT INTO",
	} {
		assert.NotContains(t, string(migrateCode), fragment,
			"database DDL belongs in versioned SQL files, not Go constants")
	}

	schemaPath := filepath.Join(root, "services", "api", "internal", "postgres", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	assert.Contains(t, string(schema), "CREATE TABLE IF NOT EXISTS events")
}

func TestPhase1DocsDoNotClaimDeferredInfrastructureIsComplete(t *testing.T) {
	root := repoRoot(t)
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

	offenders, err := forbiddenMarkdownPhrases(root, forbidden)
	require.NoError(t, err)
	assert.Empty(t, offenders, "Phase 1 docs claim deferred infrastructure is complete: %s", strings.Join(offenders, "; "))
}

func TestPhase2DocsDoNotClaimDeferredInfraIsRequired(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		"phase 2 requires kafka",
		"phase 2 uses kafka",
		"phase 2 has kafka",
		"phase 2 implements kafka",
		"phase 2 ships kafka",
		"kafka is complete in phase 2",
		"phase 2 requires kubernetes",
		"phase 2 uses kubernetes",
		"phase 2 has kubernetes",
		"phase 2 implements kubernetes",
		"phase 2 ships kubernetes",
		"kubernetes is complete in phase 2",
		"phase 2 implements microservices",
		"phase 2 has microservices",
		"phase 2 has full microservices",
		"phase 2 implements full microservices",
		"phase 2 ships microservices",
		"phase 2 ships full microservices",
		"microservices are complete in phase 2",
		"full microservices are complete in phase 2",
		"phase 2 requires service mesh",
		"phase 2 uses service mesh",
		"phase 2 has service mesh",
		"phase 2 implements service mesh",
		"phase 2 ships service mesh",
		"service mesh is complete in phase 2",
		"phase 2 requires cross-region ha",
		"phase 2 has cross-region ha",
		"phase 2 has cross-region high availability",
		"phase 2 implements cross-region ha",
		"phase 2 ships cross-region ha",
		"cross-region ha is complete in phase 2",
		"cross-region high availability is complete in phase 2",
		"phase 2 completed kafka",
		"phase 2 completed kubernetes",
		"phase 2 completed service mesh",
		"phase 2 completed cross-region ha",
		"phase 2 completed microservices",
		"kafka completed in phase 2",
		"kubernetes completed in phase 2",
		"service mesh completed in phase 2",
	}

	offenders, err := forbiddenMarkdownPhrases(root, forbidden)
	require.NoError(t, err)
	assert.Empty(t, offenders, "Phase 2 docs claim deferred infrastructure is required: %s", strings.Join(offenders, "; "))
}

func TestPhase2AsyncPlatformDoesNotDependOnExternalBrokerClients(t *testing.T) {
	root := repoRoot(t)
	forbiddenPrefixes := []string{
		"github.com/segmentio/kafka-go",
		"github.com/confluentinc/confluent-kafka-go",
		"github.com/IBM/sarama",
		"github.com/Shopify/sarama",
		"github.com/rabbitmq/amqp091-go",
		"github.com/streadway/amqp",
		"github.com/nats-io/nats.go",
		"github.com/aws/aws-sdk-go/service/sqs",
		"github.com/aws/aws-sdk-go-v2/service/sqs",
	}

	offenders := brokerClientImportOffenders(t, root, forbiddenPrefixes)
	offenders = append(offenders, brokerClientModuleOffenders(t, root, forbiddenPrefixes)...)
	assert.Empty(t, offenders, "Phase 2 WS4 must stay on the PostgreSQL outbox, not external broker clients: %s", strings.Join(offenders, "; "))
}

func TestOpsNotificationDeliveriesOpenAPIRolesMatchRuntime(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "docs", "openapi", "paths", "admin-ops.yaml"))
	require.NoError(t, err)

	block := extractOpenAPIPathBlock(string(data), `"/admin/ops/notification-deliveries":`)
	require.NotEmpty(t, block, "admin ops notification deliveries OpenAPI path not found")

	roles := extractOpenAPIRequiredRoles(block)
	assert.Equal(t, []string{"hr_admin", "system_admin"}, roles,
		"ops notification delivery feed is HR/system-admin only; activity_admin can use the Phase 1 delivery listing instead")
}

func TestEventTypeRegistryMatchesNormativeDoc(t *testing.T) {
	root := repoRoot(t)
	docPath := filepath.Join(root, "docs", "specs", "phase2-ws4-async-notification.md")
	data, err := os.ReadFile(docPath)
	require.NoError(t, err)

	docTypes := extractEventRegistryFromDoc(t, string(data))
	require.NotEmpty(t, docTypes, "WS4 §6 event registry block not found in %s", docPath)
	assert.Equalf(t, eventcontract.Registry, docTypes,
		"event type registry drift: docs/specs/phase2-ws4-async-notification.md §6 must match eventcontract.Registry exactly (order-sensitive)")

	allowed := map[string]struct{}{}
	for _, et := range eventcontract.Registry {
		allowed[et] = struct{}{}
	}
	literalRe := regexp.MustCompile(`"[a-z_]+(?:\.[a-z_]+){1,3}\.v[0-9]+"`)
	var offenders []string
	err = filepath.WalkDir(filepath.Join(root, "services", "api", "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasSuffix(filepath.ToSlash(path), "/eventcontract/registry.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range literalRe.FindAll(body, -1) {
			literal := strings.Trim(string(match), `"`)
			if _, ok := allowed[literal]; !ok {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel+": "+literal)
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offenders, "Go code references event types not in eventcontract.Registry: %s", strings.Join(offenders, "; "))
}

func extractEventRegistryFromDoc(t *testing.T, content string) []string {
	t.Helper()
	lines := strings.Split(content, "\n")
	marker := "Event type registry (normative"
	var inBlock bool
	var sawMarker bool
	var types []string
	for _, line := range lines {
		if !sawMarker {
			if strings.Contains(line, marker) {
				sawMarker = true
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if strings.HasPrefix(trimmed, "```") {
				inBlock = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			break
		}
		if trimmed == "" {
			continue
		}
		types = append(types, trimmed)
	}
	return types
}

func extractOpenAPIPathBlock(content string, path string) string {
	start := strings.Index(content, path)
	if start < 0 {
		return ""
	}
	lines := strings.Split(content[start+len(path):], "\n")
	var block []string
	for _, line := range lines {
		if strings.HasPrefix(line, `"/`) {
			break
		}
		block = append(block, line)
	}
	return strings.Join(block, "\n")
}

func extractOpenAPIRequiredRoles(block string) []string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "x-required-roles:" {
			continue
		}
		var roles []string
		for _, roleLine := range lines[i+1:] {
			trimmed := strings.TrimSpace(roleLine)
			if strings.HasPrefix(trimmed, "- ") {
				roles = append(roles, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
				continue
			}
			if trimmed != "" {
				break
			}
		}
		return roles
	}
	return nil
}

func forbiddenMarkdownPhrases(root string, forbidden []string) ([]string, error) {
	var offenders []string
	for _, path := range []string{filepath.Join(root, "AGENTS.md"), filepath.Join(root, "docs")} {
		err := filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			fileOffenders, err := forbiddenMarkdownPhrasesInFile(root, path, forbidden)
			if err != nil {
				return err
			}
			offenders = append(offenders, fileOffenders...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return offenders, nil
}

func brokerClientImportOffenders(t *testing.T, root string, forbiddenPrefixes []string) []string {
	t.Helper()
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "services", "api"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || isGeneratedGo(path) {
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
	return offenders
}

func brokerClientModuleOffenders(t *testing.T, root string, forbiddenPrefixes []string) []string {
	t.Helper()
	var offenders []string
	for _, name := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(root, "services", "api", name)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, line := range strings.Split(string(data), "\n") {
			modulePath := strings.Fields(line)
			if len(modulePath) == 0 {
				continue
			}
			if hasAnyPrefix(modulePath[0], forbiddenPrefixes) {
				offenders = append(offenders, filepath.Join("services", "api", name)+": "+modulePath[0])
			}
		}
	}
	return offenders
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
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
