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

func TestPhase3RunScriptWiresGatedEvidencePaths(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-run.sh"))

	for _, fragment := range []string{
		"CETS_PHASE3_RUN_ID",
		"CETS_PHASE3_RUN_ARTIFACT_DIR",
		"CETS_PHASE3_RUN_REPORT",
		"CETS_PHASE3_RUN_LOG",
		"CETS_PHASE3_RUN_INCLUDE_DRILL",
		"phase3-deploy.sh",
		"phase3-verify.sh",
		"phase3-drill.sh",
		"phase3-capacity.sh",
		"phase3-quality.sh",
		"CETS_PHASE3_VERIFY_RUN_ID=\"$RUN_ID\"",
		"CETS_PHASE3_VERIFY_ARTIFACT_DIR=\"$VERIFY_ARTIFACT_DIR\"",
		"CETS_PHASE3_CAPACITY_VERIFY_REPORT=\"$VERIFY_REPORT\"",
		"CETS_PHASE3_CAPACITY_REPORT=\"$CAPACITY_REPORT\"",
		"CETS_PHASE3_QUALITY_REPORT=\"$QUALITY_REPORT\"",
		"CETS_PHASE3_SONAR_RESULT_REPORT=\"$SONAR_RESULT_REPORT\"",
		"Phase 3 Gated Run Report",
		"Run log",
		"Failed step",
		"Exit code",
		`statuses=("${PIPESTATUS[@]}")`,
		"step_status=${statuses[0]}",
		"tee_status=${statuses[1]}",
		"trap finalize EXIT",
		missingPhase3ScriptError,
	} {
		assert.Contains(t, script, fragment)
	}
}

func TestPhase3RunScriptExecutesStepsAndWritesRunReport(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	runReport := filepath.Join(dir, phase3RunReport)
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{})

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=mock-run",
		phase3ArtifactDirEnv+artifactDir,
		phase3RunReportEnv+runReport,
	)

	require.NoError(t, err, output)
	report := readText(t, runReport)
	verifyReport := filepath.Join(artifactDir, "verify", "phase3-verify-report-mock-run.md")
	capacityReport := filepath.Join(artifactDir, "capacity", "capacity-report-mock-run.md")
	drillReport := filepath.Join(artifactDir, "drill", "phase3-drill-report-mock-run.md")
	qualityReport := filepath.Join(artifactDir, "quality", "phase3-quality-report-mock-run.md")
	sonarReport := filepath.Join(artifactDir, "quality", "sonar-result-mock-run.md")
	runLog := filepath.Join(artifactDir, "phase3-run-steps-mock-run.log")

	assert.Contains(t, output, "completed quality gate")
	assert.Contains(t, report, "| Status | `passed` |")
	assert.Contains(t, report, runLogRowPrefix+runLog+"` |")
	assert.Contains(t, report, "| Verify report | `"+verifyReport+"` |")
	assert.Contains(t, report, "| Recovery drill report | `"+drillReport+"` |")
	assert.Contains(t, report, "| Capacity report | `"+capacityReport+"` |")
	assert.Contains(t, report, "| Quality report | `"+qualityReport+"` |")
	assert.Contains(t, report, "| Sonar result report | `"+sonarReport+"` |")
	runLogContent := readText(t, runLog)
	assert.Contains(t, runLogContent, "deploy stdout")
	assert.Contains(t, runLogContent, "quality stdout")

	stepLog := readText(t, filepath.Join(dir, "steps.log"))
	assert.Equal(t, strings.Join([]string{
		"deploy",
		"verify|mock-run",
		"drill|mock-run",
		"capacity|" + verifyReport,
		"quality|" + capacityReport + "|" + qualityReport + "|" + sonarReport,
		"",
	}, "\n"), stepLog)
}

func TestPhase3RunScriptReportsPreflightFailure(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{})
	require.NoError(t, os.Remove(filepath.Join(scriptDir, "phase3-deploy.sh")))

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=missing-script",
		phase3ArtifactDirEnv+artifactDir,
	)

	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, output, missingPhase3ScriptError)
	report := readText(t, filepath.Join(artifactDir, "phase3-run-report-missing-script.md"))
	assert.Contains(t, report, statusFailedRow)
	assert.Contains(t, report, "| Failed step | `preflight` |")
	assert.Contains(t, report, exitCodeOneRow)
	runLogPath := filepath.Join(artifactDir, "phase3-run-steps-missing-script.log")
	assert.Contains(t, report, runLogRowPrefix+runLogPath+"` |")
	assert.Contains(t, readText(t, runLogPath), missingPhase3ScriptError)
}

func TestPhase3RunScriptReportsArtifactSetupFailure(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	reportPath := filepath.Join(dir, "reports", "phase3-run-report-setup-failed.md")
	blockedLogParent := filepath.Join(dir, "blocked-log-parent")
	runLogPath := filepath.Join(blockedLogParent, "phase3-run-steps-setup-failed.log")
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{})
	require.NoError(t, os.WriteFile(blockedLogParent, []byte("not a directory"), 0o600))

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=setup-failed",
		phase3ArtifactDirEnv+artifactDir,
		phase3RunReportEnv+reportPath,
		"CETS_PHASE3_RUN_LOG="+runLogPath,
	)

	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, output, "mkdir")
	report := readText(t, reportPath)
	assert.Contains(t, report, statusFailedRow)
	assert.Contains(t, report, "| Failed step | `artifact setup` |")
	assert.Contains(t, report, exitCodeOneRow)
	assert.Contains(t, report, runLogRowPrefix+runLogPath+"` |")
}

func TestPhase3RunScriptFailsWhenLinkedReportIsMissing(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{skipCapacityReport: true})

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=missing-report",
		phase3ArtifactDirEnv+artifactDir,
	)

	require.Error(t, err)
	assert.Contains(t, output, "Phase 3 capacity report was not written or is empty")
	report := readText(t, filepath.Join(artifactDir, "phase3-run-report-missing-report.md"))
	assert.Contains(t, report, statusFailedRow)
	assert.Contains(t, report, "| Failed step | `capacity report verification` |")
	assert.Contains(t, report, exitCodeOneRow)
	assert.Contains(t, report, runLogRowPrefix+filepath.Join(artifactDir, "phase3-run-steps-missing-report.log")+"` |")
}

func TestPhase3RunScriptFailsWhenSonarResultReportIsMissing(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{skipSonarReport: true})

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=missing-sonar",
		phase3ArtifactDirEnv+artifactDir,
	)

	require.Error(t, err)
	assert.Contains(t, output, "Phase 3 Sonar result report was not written or is empty")
	report := readText(t, filepath.Join(artifactDir, "phase3-run-report-missing-sonar.md"))
	assert.Contains(t, report, statusFailedRow)
	assert.Contains(t, report, "| Failed step | `Sonar result report verification` |")
	assert.Contains(t, report, exitCodeOneRow)
	assert.Contains(t, report, runLogRowPrefix+filepath.Join(artifactDir, "phase3-run-steps-missing-sonar.log")+"` |")
}

func TestPhase3RunScriptPreservesStepFailureExitCode(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{failQualityExitCode: 42})

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=quality-failed",
		phase3ArtifactDirEnv+artifactDir,
	)

	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 42, exitErr.ExitCode())
	assert.Contains(t, output, "starting quality gate")
	report := readText(t, filepath.Join(artifactDir, "phase3-run-report-quality-failed.md"))
	assert.Contains(t, report, statusFailedRow)
	assert.Contains(t, report, "| Failed step | `quality gate` |")
	assert.Contains(t, report, "| Exit code | `42` |")
	runLog := readText(t, filepath.Join(artifactDir, "phase3-run-steps-quality-failed.log"))
	assert.Contains(t, runLog, "quality stdout")
}

func TestPhase3RunScriptFailsWhenRunLogCaptureFails(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	fakeBin := filepath.Join(dir, "bin")
	artifactDir := filepath.Join(dir, "artifacts")
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{})
	require.NoError(t, os.Mkdir(fakeBin, 0o700))
	writePhase3RunFakeScript(t, fakeBin, "tee", `#!/usr/bin/env bash
cat >/dev/null
exit 9
`)

	output, err := runPhase3Run(t, scriptDir,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CETS_PHASE3_RUN_ID=tee-failed",
		phase3ArtifactDirEnv+artifactDir,
	)

	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 9, exitErr.ExitCode())
	assert.Contains(t, output, "starting deploy")
	report := readText(t, filepath.Join(artifactDir, "phase3-run-report-tee-failed.md"))
	assert.Contains(t, report, statusFailedRow)
	assert.Contains(t, report, "| Failed step | `deploy` |")
	assert.Contains(t, report, "| Exit code | `9` |")
	assert.Contains(t, report, runLogRowPrefix+filepath.Join(artifactDir, "phase3-run-steps-tee-failed.log")+"` |")
}

func TestPhase3RunScriptCanSkipRecoveryDrill(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	runReport := filepath.Join(dir, phase3RunReport)
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{})

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=no-drill",
		phase3ArtifactDirEnv+artifactDir,
		phase3RunReportEnv+runReport,
		"CETS_PHASE3_RUN_INCLUDE_DRILL=false",
	)

	require.NoError(t, err, output)
	assert.Contains(t, output, "skipping recovery drill")
	assert.Contains(t, readText(t, runReport), "| Recovery drill report | `skipped` |")
	assert.NotContains(t, readText(t, filepath.Join(dir, "steps.log")), "drill|")
}

func TestPhase3RunScriptDoesNotRequireDrillScriptWhenSkipped(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	artifactDir := filepath.Join(dir, "artifacts")
	runReport := filepath.Join(dir, phase3RunReport)
	writePhase3RunFakeScripts(t, scriptDir, fakeRunOptions{})
	require.NoError(t, os.Remove(filepath.Join(scriptDir, "phase3-drill.sh")))

	output, err := runPhase3Run(t, scriptDir,
		"CETS_PHASE3_RUN_ID=no-drill-script",
		phase3ArtifactDirEnv+artifactDir,
		phase3RunReportEnv+runReport,
		"CETS_PHASE3_RUN_INCLUDE_DRILL=false",
	)

	require.NoError(t, err, output)
	assert.Contains(t, readText(t, runReport), "| Recovery drill report | `skipped` |")
}
