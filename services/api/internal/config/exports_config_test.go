package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultsExportsSourceProjection(t *testing.T) {
	t.Setenv("EXPORTS_SOURCE", "")
	t.Setenv("EXPORTS_STALE_POLICY", "")

	cfg := Load()

	assert.Equal(t, "projection", cfg.ExportsSource)
	assert.Equal(t, "fail", cfg.ExportsStalePolicy)
}

func TestLoadAcceptsOperationalExportsSource(t *testing.T) {
	t.Setenv("EXPORTS_SOURCE", "operational")

	cfg := Load()

	assert.Equal(t, "operational", cfg.ExportsSource)
}

func TestLoadedConfigRejectsMalformedExportsSource(t *testing.T) {
	t.Setenv("EXPORTS_SOURCE", "warehouse")

	err := Load().ValidateForServe()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "EXPORTS_SOURCE")
}

func TestLoadedConfigRejectsMalformedExportsStalePolicy(t *testing.T) {
	t.Setenv("EXPORTS_STALE_POLICY", "fallback")

	err := Load().ValidateForServe()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "EXPORTS_STALE_POLICY")
}
