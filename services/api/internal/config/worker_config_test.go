package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWorkerRedactsUnsafeUnsupportedWorkerKind(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.WorkerKinds = []string{"notification", "e1001@cets.local"}

	err := cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_KINDS")
	assert.Contains(t, err.Error(), "unsupported worker kind")
	assert.NotContains(t, err.Error(), "e1001@cets.local")
	assert.NotContains(t, err.Error(), "E1001")
}

func TestLoadedWorkerKindsRedactsUnsafeEnvValue(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/cets")
	t.Setenv("WORKER_KINDS", "notification,e1001@cets.local-eyJhbGciOiJIUzI1NiJ9.payload.signature")

	err := Load().ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_KINDS")
	assert.Contains(t, err.Error(), "unsupported worker kind")
	assert.NotContains(t, err.Error(), "e1001@cets.local")
	assert.NotContains(t, err.Error(), "E1001")
	assert.NotContains(t, err.Error(), "eyjhbgci")
}

func TestWorkerArgsRedactUnsafeUnknownFlagValue(t *testing.T) {
	cfg := validWorkerConfig()

	_, err := cfg.WithWorkerArgs([]string{"--workers=e1001@cets.local"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown worker argument")
	assert.NotContains(t, err.Error(), "e1001@cets.local")
	assert.NotContains(t, err.Error(), "--workers=")
}
