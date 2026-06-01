package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateForServeRejectsDemoDebugOutsideDemoEnvironments(t *testing.T) {
	cfg := productionServeConfig()
	cfg.DemoDebugEnabled = true

	err := cfg.ValidateForServe()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DEMO_DEBUG_ENABLED")
}

func TestValidateForServeAcceptsDemoDebugInLocalDemoAndTest(t *testing.T) {
	for _, appEnv := range []string{"local", "demo", "test"} {
		t.Run(appEnv, func(t *testing.T) {
			cfg := Config{
				AppAddr:          ":8080",
				AppEnv:           appEnv,
				DatabaseURL:      "postgres://user:pass@localhost:5432/cets",
				RequestTimeout:   time.Second,
				DatabaseTimeout:  time.Second,
				ShutdownTimeout:  time.Second,
				DemoDebugEnabled: true,
			}

			require.NoError(t, cfg.ValidateForServe())
		})
	}
}

func TestValidateWorkerRejectsDemoDebugOutsideDemoEnvironments(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.AppEnv = "staging"
	cfg.DemoDebugEnabled = true

	err := cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DEMO_DEBUG_ENABLED")
}
