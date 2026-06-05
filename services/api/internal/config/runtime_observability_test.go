package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRuntimeObservabilityRequiresServiceNames(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.OTelTracesEnabled = true
	cfg.OTelEndpoint = "http://alloy:4318"
	cfg.OTelServiceName = ""

	err := cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OTEL_SERVICE_NAME")

	cfg = validWorkerConfig()
	cfg.PyroscopeEnabled = true
	cfg.PyroscopeAddress = "http://pyroscope:4040"
	cfg.PyroscopeAppName = ""

	err = cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "PYROSCOPE_APPLICATION_NAME")
}

func TestParseHelpersUseSafeFallbackWhenFallbackIsMalformed(t *testing.T) {
	var loadErrors []string
	t.Setenv("BAD_DURATION_SECONDS", "soon")
	t.Setenv("BAD_NON_NEGATIVE_INT", "later")

	duration := parsePositiveDurationSecondsEnv(
		"BAD_DURATION_SECONDS",
		"also-bad",
		&loadErrors,
	)
	nonNegative := parseNonNegativeIntEnv(
		"BAD_NON_NEGATIVE_INT",
		"also-bad",
		&loadErrors,
	)

	assert.Equal(t, time.Second, duration)
	assert.Equal(t, 0, nonNegative)
	assert.Len(t, loadErrors, 2)
}

func TestLoadRuntimeObservabilityReplicaIDUsesExplicitValueThenHostname(t *testing.T) {
	t.Setenv("CETS_REPLICA_ID", "backend-2")
	t.Setenv("HOSTNAME", "pod-hostname")

	cfg := Load()

	assert.Equal(t, "backend-2", cfg.CETSReplicaID)

	t.Setenv("CETS_REPLICA_ID", "")

	cfg = Load()

	assert.Equal(t, "pod-hostname", cfg.CETSReplicaID)
}
