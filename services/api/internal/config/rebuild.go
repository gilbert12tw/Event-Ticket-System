package config

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
func LoadRebuildSettings() RebuildSettings {
	var loadErrors []string
	return RebuildSettings{
		DryRun:         parseBoolEnv("REBUILD_DRY_RUN", "false", &loadErrors),
		SampleValidate: parseBoolEnv("REBUILD_SAMPLE_VALIDATE", "true", &loadErrors),
	}
}
