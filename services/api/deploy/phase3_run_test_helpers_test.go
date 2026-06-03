package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func runPhase3Run(t *testing.T, scriptDir string, env ...string) (string, error) {
	t.Helper()
	script := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-run.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "CETS_PHASE3_RUN_SCRIPT_DIR="+scriptDir)
	cmd.Env = append(cmd.Env, env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

type fakeRunOptions struct {
	skipCapacityReport  bool
	skipSonarReport     bool
	failQualityExitCode int
}

func writePhase3RunFakeScripts(t *testing.T, scriptDir string, options fakeRunOptions) {
	t.Helper()
	require.NoError(t, os.MkdirAll(scriptDir, 0o700))
	logPath := filepath.Join(filepath.Dir(scriptDir), "steps.log")
	writePhase3RunFakeScript(t, scriptDir, "phase3-deploy.sh", `#!/usr/bin/env bash
set -euo pipefail
printf 'deploy stdout\n'
printf 'deploy\n' >>'`+logPath+`'
`)
	writePhase3RunFakeScript(t, scriptDir, "phase3-verify.sh", `#!/usr/bin/env bash
set -euo pipefail
printf 'verify stdout\n'
printf 'verify|%s\n' "$CETS_PHASE3_VERIFY_RUN_ID" >>'`+logPath+`'
mkdir -p "$CETS_PHASE3_VERIFY_ARTIFACT_DIR"
printf '# verify\n' >"$CETS_PHASE3_VERIFY_ARTIFACT_DIR/phase3-verify-report-$CETS_PHASE3_VERIFY_RUN_ID.md"
`)
	writePhase3RunFakeScript(t, scriptDir, "phase3-drill.sh", `#!/usr/bin/env bash
set -euo pipefail
printf 'drill stdout\n'
printf 'drill|%s\n' "$CETS_PHASE3_DRILL_RUN_ID" >>'`+logPath+`'
mkdir -p "$CETS_PHASE3_DRILL_ARTIFACT_DIR"
printf '# drill\n' >"$CETS_PHASE3_DRILL_ARTIFACT_DIR/phase3-drill-report-$CETS_PHASE3_DRILL_RUN_ID.md"
`)
	capacityBody := `#!/usr/bin/env bash
set -euo pipefail
printf 'capacity stdout\n'
printf 'capacity|%s\n' "$CETS_PHASE3_CAPACITY_VERIFY_REPORT" >>'` + logPath + `'
mkdir -p "$CETS_PHASE3_CAPACITY_ARTIFACT_DIR"
`
	if !options.skipCapacityReport {
		capacityBody += `printf '# capacity\n' >"$CETS_PHASE3_CAPACITY_ARTIFACT_DIR/capacity-report-$CETS_PHASE3_CAPACITY_RUN_ID.md"
`
	}
	writePhase3RunFakeScript(t, scriptDir, "phase3-capacity.sh", capacityBody)
	qualityBody := `#!/usr/bin/env bash
set -euo pipefail
printf 'quality stdout\n'
printf 'quality|%s|%s|%s\n' "$CETS_PHASE3_CAPACITY_REPORT" "$CETS_PHASE3_QUALITY_REPORT" "$CETS_PHASE3_SONAR_RESULT_REPORT" >>'` + logPath + `'
mkdir -p "$(dirname "$CETS_PHASE3_QUALITY_REPORT")"
`
	if options.failQualityExitCode > 0 {
		qualityBody += `exit ` + strconv.Itoa(options.failQualityExitCode) + `
`
	} else {
		qualityBody += `
printf '# quality\n' >"$CETS_PHASE3_QUALITY_REPORT"
`
		if !options.skipSonarReport {
			qualityBody += `printf '# sonar\n' >"$CETS_PHASE3_SONAR_RESULT_REPORT"
`
		}
	}
	writePhase3RunFakeScript(t, scriptDir, "phase3-quality.sh", qualityBody)
}

func writePhase3RunFakeScript(t *testing.T, scriptDir string, name string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(scriptDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, name), []byte(content), 0o700))
}
