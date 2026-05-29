package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvExampleDocumentsWS4AsyncWorkerSettings(t *testing.T) {
	env := loadEnvExample(t)
	expected := map[string]string{
		"OPS_API_ENABLED":                 "false",
		"WORKER_SHUTDOWN_GRACE_SECONDS":   "30",
		"OUTBOX_BATCH_SIZE":               "100",
		"OUTBOX_LEASE_TTL_SECONDS":        "60",
		"OUTBOX_RETRY_MAX":                "10",
		"OUTBOX_BACKOFF_BASE_MS":          "500",
		"OUTBOX_BACKOFF_MAX_MS":           "60000",
		"WORKER_KINDS":                    "notification,projection,compensation,export",
		"WORKER_CONCURRENCY_NOTIFICATION": "4",
		"WORKER_CONCURRENCY_PROJECTION":   "2",
		"WORKER_CONCURRENCY_COMPENSATION": "1",
		"WORKER_CONCURRENCY_EXPORT":       "1",
	}

	for key, value := range expected {
		assert.Equal(t, value, env[key], "%s must stay documented in deploy/.env.example", key)
	}
}

func loadEnvExample(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", ".env.example"))
	require.NoError(t, err)

	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		require.True(t, ok, "invalid .env.example line %q", line)
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}
