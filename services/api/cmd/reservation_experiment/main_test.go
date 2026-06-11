package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"event-ticket-system/internal/postgres"

	"github.com/jackc/pgx/v5"
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

func TestRunExperimentOffModeAgainstIsolatedSchema(t *testing.T) {
	got := runExperimentIntegration(t, "off")

	assert.Equal(t, "off", got.Mode)
	assert.Equal(t, 3, got.VUs)
	assert.Equal(t, 1, got.Capacity)
	assert.Equal(t, 1, got.Outcomes.Confirmed)
	assert.Equal(t, 2, got.Outcomes.Waitlisted)
	assert.Zero(t, got.Outcomes.Error)
	assert.Equal(t, 1, got.DBConfirmedCount)
}

func TestRunExperimentOnModeUsesRedisGateAgainstIsolatedSchema(t *testing.T) {
	if os.Getenv("REDIS_URL") == "" {
		t.Skip("REDIS_URL is not set")
	}

	got := runExperimentIntegration(t, "on")

	assert.Equal(t, "on", got.Mode)
	assert.Equal(t, 3, got.VUs)
	assert.Equal(t, 1, got.Capacity)
	assert.Equal(t, 1, got.Outcomes.Confirmed)
	assert.Equal(t, 2, got.Outcomes.Waitlisted)
	assert.Zero(t, got.Outcomes.Error)
	assert.Equal(t, 1, got.DBConfirmedCount)
}

func runExperimentIntegration(t *testing.T, mode string) result {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	t.Setenv("EXPERIMENT_ALLOW_DESTRUCTIVE", "1")
	t.Setenv("EXPERIMENT_MODE", mode)
	t.Setenv("EXPERIMENT_VUS", "3")
	t.Setenv("EXPERIMENT_CAPACITY", "1")
	t.Setenv("EXPERIMENT_TIMEOUT_SECONDS", "30")
	t.Setenv("DATABASE_URL", experimentDatabaseURL(t, databaseURL))

	body := captureExperimentStdout(t, func() {
		require.NoError(t, run())
	})

	var got result
	require.NoError(t, json.Unmarshal(body, &got))
	return got
}

func experimentDatabaseURL(t *testing.T, databaseURL string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	adminPool, err := postgres.Connect(ctx, databaseURL)
	require.NoError(t, err)

	schema := fmt.Sprintf("reservation_experiment_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchema))
	require.NoError(t, err)
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dropCancel()
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchema))
		adminPool.Close()
	})

	parsed, err := url.Parse(databaseURL)
	require.NoError(t, err)
	values := parsed.Query()
	values.Set("search_path", schema)
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func captureExperimentStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	originalStdout := os.Stdout
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writeEnd
	t.Cleanup(func() {
		os.Stdout = originalStdout
		_ = readEnd.Close()
	})

	fn()

	require.NoError(t, writeEnd.Close())
	body, err := io.ReadAll(readEnd)
	require.NoError(t, err)
	return body
}
