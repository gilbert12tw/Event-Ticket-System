package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase3CapacityReportRecordsRPSSearchParameters(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", phase3CapacityScript))
	preflight := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", phase3CapacityPreflight))
	report := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-report.sh"))

	for _, fragment := range []string{
		phase3CapacityPreflight,
		`. "$ROOT_DIR/scripts/compose/phase3-capacity-preflight.sh"`,
		"START_RPS=${CETS_PHASE3_CAPACITY_START_RPS:-50}",
		"STEP_RPS=${CETS_PHASE3_CAPACITY_STEP_RPS:-50}",
		"MAX_RPS=${CETS_PHASE3_CAPACITY_MAX_RPS:-1000}",
		"RESOLUTION_RPS=${CETS_PHASE3_CAPACITY_RESOLUTION_RPS:-25}",
	} {
		assert.Contains(t, script, fragment)
	}

	for _, fragment := range []string{
		"RPS search start",
		"RPS search step",
		"RPS search max",
		"RPS search resolution",
		"$START_RPS",
		"$STEP_RPS",
		"$MAX_RPS",
		"$RESOLUTION_RPS",
	} {
		assert.Contains(t, report, fragment)
	}
	for _, fragment := range []string{
		"require_positive_integer",
		"validate_capacity_inputs",
		"CETS_PHASE3_CAPACITY_READ_RATIO must be between 0 and 1",
	} {
		assert.Contains(t, preflight, fragment)
	}
}

func TestPhase3CapacityReportLinksOptionalLGTMVerifyReport(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", phase3CapacityScript))
	preflight := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", phase3CapacityPreflight))
	report := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-report.sh"))

	for _, fragment := range []string{
		"CAPACITY_VERIFY_REPORT=${CETS_PHASE3_CAPACITY_VERIFY_REPORT:-}",
		"CETS_PHASE3_CAPACITY_VERIFY_REPORT   Optional Phase 3 verify report path to link in reports.",
		"validate_capacity_verify_report",
	} {
		assert.Contains(t, script, fragment)
	}
	for _, fragment := range []string{
		"Phase 3 verify report does not exist",
		"Phase 3 verify report is missing required LGTM evidence field",
		"Tempo trace evidence",
		"Service graph backend dependency evidence",
		"Pyroscope profile evidence",
		"Loki trace-log evidence",
		"Loki redaction evidence",
	} {
		assert.Contains(t, preflight, fragment)
	}

	for _, fragment := range []string{
		"validate_capacity_verify_report",
		"LGTM verify report",
		"$CAPACITY_VERIFY_REPORT",
		"not linked",
	} {
		assert.Contains(t, report, fragment)
	}

	mainIndex := strings.Index(script, "main() {")
	require.NotEqual(t, -1, mainIndex)
	mainBody := script[mainIndex:]
	validateIndex := strings.Index(mainBody, "validate_capacity_verify_report\n")
	dockerIndex := strings.Index(mainBody, "require_docker_daemon\n")
	require.NotEqual(t, -1, validateIndex)
	require.NotEqual(t, -1, dockerIndex)
	assert.Less(t, validateIndex, dockerIndex)
}

func TestPhase3CapacityRejectsIncompleteLGTMVerifyReportBeforeDocker(t *testing.T) {
	verifyReport := filepath.Join(t.TempDir(), "phase3-verify-report.md")
	require.NoError(t, os.WriteFile(verifyReport, []byte(strings.Join([]string{
		"# Phase 3 Verify Report",
		"",
		"| Field | Value |",
		"| --- | --- |",
		"| Tempo trace evidence | `/tmp/tempo-trace.json` |",
	}, "\n")), 0o600))

	script := filepath.Join("..", "..", "..", "scripts", "compose", phase3CapacityScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"CETS_PHASE3_CAPACITY_VERIFY_REPORT="+verifyReport,
		"CETS_PHASE3_CAPACITY_REPORT_ONLY_RPS=100",
	)
	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	assert.Contains(t, string(output), "Phase 3 verify report is missing required LGTM evidence field: Service graph backend dependency evidence")
	assert.NotContains(t, string(output), "docker daemon access is required")
}

func TestPhase3CapacityRejectsInvalidInputsBeforeDocker(t *testing.T) {
	for name, tc := range map[string]struct {
		env     string
		message string
	}{
		"zero start rps": {
			env:     "CETS_PHASE3_CAPACITY_START_RPS=0",
			message: "CETS_PHASE3_CAPACITY_START_RPS must be a positive integer",
		},
		"max below start": {
			env:     "CETS_PHASE3_CAPACITY_MAX_RPS=40",
			message: "CETS_PHASE3_CAPACITY_MAX_RPS must be greater than or equal to CETS_PHASE3_CAPACITY_START_RPS",
		},
		"invalid read ratio": {
			env:     "CETS_PHASE3_CAPACITY_READ_RATIO=not-a-number",
			message: "CETS_PHASE3_CAPACITY_READ_RATIO must be between 0 and 1",
		},
		"zero replica samples": {
			env:     "CETS_PHASE3_CAPACITY_REPLICA_SAMPLES=0",
			message: "CETS_PHASE3_CAPACITY_REPLICA_SAMPLES must be a positive integer",
		},
	} {
		t.Run(name, func(t *testing.T) {
			script := filepath.Join("..", "..", "..", "scripts", "compose", phase3CapacityScript)
			cmd := exec.Command("bash", script)
			cmd.Env = append(os.Environ(), tc.env)
			output, err := cmd.CombinedOutput()

			require.Error(t, err)
			assert.Contains(t, string(output), tc.message)
			assert.NotContains(t, string(output), "docker daemon access is required")
		})
	}
}
