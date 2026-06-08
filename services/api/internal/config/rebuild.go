package config

import (
	"fmt"
	"strings"
)

// RebuildSettings holds PH2-43 read-model rebuild flags sourced from env. They
// live in their own loader (not the main Config) to keep config.go within the
// 500-line architecture budget.
type RebuildSettings struct {
	// DryRun aggregates from OLTP and reports counts without writing.
	DryRun bool
	// SampleValidate spot-checks up to 5 events against OLTP before commit.
	SampleValidate bool
}

// LoadRebuildSettings reads REBUILD_DRY_RUN and REBUILD_SAMPLE_VALIDATE from the
// environment. Defaults: DryRun=false, SampleValidate=true. Never hardcoded.
// Unparseable values surface as an error (mirroring the main Config's
// validateLoadedConfig pattern) rather than silently falling back to defaults.
func LoadRebuildSettings() (RebuildSettings, error) {
	var loadErrors []string
	settings := RebuildSettings{
		DryRun:         parseBoolEnv("REBUILD_DRY_RUN", "false", &loadErrors),
		SampleValidate: parseBoolEnv("REBUILD_SAMPLE_VALIDATE", "true", &loadErrors),
	}
	if len(loadErrors) > 0 {
		return RebuildSettings{}, fmt.Errorf("invalid rebuild configuration: %s", strings.Join(loadErrors, "; "))
	}
	return settings, nil
}
