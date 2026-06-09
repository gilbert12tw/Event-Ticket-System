package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRebuildSettingsDefaults(t *testing.T) {
	settings, err := LoadRebuildSettings()

	require.NoError(t, err)
	assert.False(t, settings.DryRun)
	assert.True(t, settings.SampleValidate)
}

func TestLoadRebuildSettingsParsesFlags(t *testing.T) {
	t.Setenv("REBUILD_DRY_RUN", "true")
	t.Setenv("REBUILD_SAMPLE_VALIDATE", "false")

	settings, err := LoadRebuildSettings()

	require.NoError(t, err)
	assert.True(t, settings.DryRun)
	assert.False(t, settings.SampleValidate)
}

func TestLoadRebuildSettingsRejectsUnparseableValues(t *testing.T) {
	t.Setenv("REBUILD_DRY_RUN", "yesplease")

	_, err := LoadRebuildSettings()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "REBUILD_DRY_RUN")
}
