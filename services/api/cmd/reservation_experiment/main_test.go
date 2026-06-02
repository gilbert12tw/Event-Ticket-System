package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoadExperimentConfig(t *testing.T) {
	t.Setenv("EXPERIMENT_ALLOW_DESTRUCTIVE", "1")
	t.Setenv("DATABASE_URL", "postgresql://example")
	t.Setenv("EXPERIMENT_MODE", "on")
	t.Setenv("EXPERIMENT_VUS", "20")
	t.Setenv("EXPERIMENT_CAPACITY", "10")
	t.Setenv("EXPERIMENT_TIMEOUT_SECONDS", "30")

	cfg, err := loadExperimentConfig()

	require.NoError(t, err)
	assert.Equal(t, "on", cfg.Mode)
	assert.Equal(t, 20, cfg.VUs)
	assert.Equal(t, 10, cfg.Capacity)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, "postgresql://example", cfg.DBURL)
}

func TestLoadExperimentConfigRejectsInvalidInput(t *testing.T) {
	t.Setenv("EXPERIMENT_ALLOW_DESTRUCTIVE", "1")
	t.Setenv("DATABASE_URL", "postgresql://example")
	t.Setenv("EXPERIMENT_MODE", "maybe")

	_, err := loadExperimentConfig()
	require.ErrorContains(t, err, "EXPERIMENT_MODE")

	t.Setenv("EXPERIMENT_MODE", "off")
	t.Setenv("DATABASE_URL", "")
	_, err = loadExperimentConfig()
	require.ErrorContains(t, err, "DATABASE_URL")
}

func TestConfigureExperimentGateValidation(t *testing.T) {
	logger := slogDiscard()
	cfg := experimentConfig{Mode: "off"}
	client, err := configureExperimentGate(t.Context(), cfg, logger, nil)
	require.NoError(t, err)
	require.Nil(t, client)

	cfg.Mode = "on"
	t.Setenv("REDIS_URL", "")
	_, err = configureExperimentGate(t.Context(), cfg, logger, nil)
	require.ErrorContains(t, err, "REDIS_URL")

	t.Setenv("REDIS_URL", "://bad")
	_, err = configureExperimentGate(t.Context(), cfg, logger, nil)
	require.ErrorContains(t, err, "invalid")
}

func TestWriteExperimentResult(t *testing.T) {
	originalStdout := os.Stdout
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writeEnd
	t.Cleanup(func() { os.Stdout = originalStdout })

	err = writeExperimentResult(experimentConfig{Mode: "off", VUs: 2, Capacity: 1}, bookingBurstResult{
		latenciesMs: []float64{2, 1},
		wall:        time.Second,
		confirmed:   1,
		waitlisted:  1,
	}, 1)
	require.NoError(t, err)
	require.NoError(t, writeEnd.Close())
	body, err := io.ReadAll(readEnd)
	require.NoError(t, err)

	var got result
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, "off", got.Mode)
	assert.Equal(t, 2, got.VUs)
	assert.Equal(t, 1, got.Outcomes.Confirmed)
	assert.Equal(t, 1.0, got.LatencyMS.Min)
	assert.Equal(t, 2.0, got.LatencyMS.Max)
}

func TestExperimentSmallHelpers(t *testing.T) {
	t.Setenv("EXPERIMENT_DEFAULT_TEST", "")
	assert.Equal(t, "fallback", getenv("EXPERIMENT_DEFAULT_TEST", "fallback"))
	t.Setenv("EXPERIMENT_DEFAULT_TEST", "value")
	assert.Equal(t, "value", getenv("EXPERIMENT_DEFAULT_TEST", "fallback"))

	t.Setenv("EXPERIMENT_INT_TEST", "bad")
	assert.Equal(t, 7, atoiOr("EXPERIMENT_INT_TEST", 7))
	t.Setenv("EXPERIMENT_INT_TEST", "11")
	assert.Equal(t, 11, atoiOr("EXPERIMENT_INT_TEST", 7))
	assert.Equal(t, "EXP00003", employeeIDFor(3))

	summary := summarize([]float64{3, 1, 2})
	assert.Equal(t, []float64{1, 2, 3}, summary.All)
	assert.Equal(t, 2.0, summary.Mean)
	assert.Zero(t, percentile(nil, 0.9))
}
