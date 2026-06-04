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

func TestPhase3QualityGateRequiresCapacityAndSonarEvidence(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-quality.sh"))

	for _, fragment := range []string{
		"CETS_PHASE3_CAPACITY_REPORT",
		"CETS_PHASE3_QUALITY_ARTIFACT_DIR",
		"CETS_PHASE3_QUALITY_REPORT",
		"CETS_PHASE3_QUALITY_RUN_ID",
		"CETS_PHASE3_SONAR_RESULT_REPORT",
		"CETS_PHASE3_QUALITY_MIN_RPS",
		"CETS_PHASE3_QUALITY_REQUIRE_OPTIMIZATION",
		"CETS_PHASE3_SONAR_COMMAND",
		"Highest passing RPS",
		"k6 summary",
		"Replica spread summary",
		"Post-load correctness summary",
		"Prometheus RED sample",
		"Prometheus backend CPU sample",
		"Prometheus backend memory sample",
		"Prometheus DB pool wait sample",
		"Prometheus global DB lock wait sample",
		"LGTM verify report",
		"linked LGTM verify report is missing required field",
		"points to a missing or empty artifact",
		"Tempo trace evidence",
		"Service graph backend dependency evidence",
		"Pyroscope profile evidence",
		"Loki trace-log evidence",
		"Loki redaction evidence",
		"not linked",
		"Bottleneck",
		"Optimization result",
		"not identified in this run",
		"not yet optimized",
		"SONAR_HOST_URL is required",
		"SONAR_TOKEN is required",
		"sonar-scanner is required",
		"phase3-sonar-result.sh",
		"running configured Sonar command",
		"configured command completed",
		"Sonar result report",
		"Quality Gate Status",
		"Security problems",
		"wrote Phase 3 quality report",
	} {
		assert.Contains(t, script, fragment)
	}
	assert.NotContains(t, script, "running Sonar command: $SONAR_COMMAND")
}

func TestPhase3QualityGateRejectsLowRPSBeforeSonar(t *testing.T) {
	report := writePhase3QualityReport(t, t.TempDir(), "100", false)

	output, err := runPhase3Quality(t, report, "CETS_PHASE3_QUALITY_MIN_RPS=900")

	require.Error(t, err)
	assert.Contains(t, output, "highest passing RPS 100 is below required 900")
	assert.NotContains(t, output, "SONAR_HOST_URL is required")
	assert.NotContains(t, output, "sonar-scanner is required")
}

func TestPhase3QualityGateRejectsEmptyCapacityArtifactBeforeSonar(t *testing.T) {
	dir := t.TempDir()
	report := writePhase3QualityReport(t, dir, "950", false)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "k6.json"), nil, 0o600))

	output, err := runPhase3Quality(t, report)

	require.Error(t, err)
	assert.Contains(t, output, "capacity report field 'k6 summary' points to a missing or empty artifact")
	assert.NotContains(t, output, "SONAR_HOST_URL is required")
}

func TestPhase3QualityGateRejectsEmptyLGTMArtifactBeforeSonar(t *testing.T) {
	dir := t.TempDir()
	report := writePhase3QualityReport(t, dir, "950", true)

	output, err := runPhase3Quality(t, report)

	require.Error(t, err)
	assert.Contains(t, output, "linked LGTM verify report field 'Tempo trace evidence' points to a missing or empty artifact")
	assert.NotContains(t, output, "SONAR_HOST_URL is required")
}

func TestPhase3QualityGateWritesReportAfterSuccessfulSonarCommand(t *testing.T) {
	dir := t.TempDir()
	report := writePhase3QualityReport(t, dir, "950", false)
	qualityReport := filepath.Join(dir, "quality-report.md")
	artifactDir := filepath.Join(dir, "quality-artifacts")
	sonarResultReport := filepath.Join(artifactDir, "sonar-result-mock-quality-run.md")
	fakeBin := writePhase3QualityFakeTools(t, dir)

	output, err := runPhase3Quality(t, report,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SONAR_HOST_URL=http://sonar.local",
		"SONAR_TOKEN=phase3-secret-token",
		"CETS_PHASE3_SONAR_COMMAND="+writePhase3SonarResultCommand("passed", "0", "0", "0"),
		"CETS_PHASE3_QUALITY_ARTIFACT_DIR="+artifactDir,
		"CETS_PHASE3_QUALITY_REPORT="+qualityReport,
		"CETS_PHASE3_QUALITY_RUN_ID=mock-quality-run",
	)

	require.NoError(t, err, output)
	quality := readText(t, qualityReport)
	assert.Contains(t, output, "wrote Phase 3 quality report")
	assert.Contains(t, quality, "| Status | `passed` |")
	assert.Contains(t, quality, "| Run ID | `mock-quality-run` |")
	assert.Contains(t, quality, "| Highest passing RPS | `950` |")
	assert.Contains(t, quality, "| Required minimum RPS | `900` |")
	assert.Contains(t, quality, "| Sonar command | `configured command completed` |")
	assert.Contains(t, quality, "| Sonar result report | `"+sonarResultReport+"` |")
	assert.Contains(t, quality, "| Sonar Quality Gate Status | `passed` |")
	assert.Contains(t, quality, "| Sonar issues | `0` |")
	assert.Contains(t, quality, "| Sonar problems | `0` |")
	assert.Contains(t, quality, "| Sonar security problems | `0` |")
	assert.Contains(t, quality, "| Capacity report | `"+report+"` |")
	assert.NotContains(t, quality, "phase3-secret-token")
	assert.NotContains(t, quality, "CETS_PHASE3_SONAR_COMMAND")
	assert.NotContains(t, quality, "pnpm sonar:scan")
}

func TestPhase3QualityGateDoesNotWriteReportWhenSonarCommandFails(t *testing.T) {
	dir := t.TempDir()
	report := writePhase3QualityReport(t, dir, "950", false)
	qualityReport := filepath.Join(dir, "quality-report.md")
	fakeBin := writePhase3QualityFakeTools(t, dir)

	output, err := runPhase3Quality(t, report,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SONAR_HOST_URL=http://sonar.local",
		"SONAR_TOKEN=phase3-secret-token",
		"CETS_PHASE3_SONAR_COMMAND=false",
		"CETS_PHASE3_QUALITY_REPORT="+qualityReport,
	)

	require.Error(t, err)
	assert.Contains(t, output, "capacity evidence accepted; running configured Sonar command")
	assert.NoFileExists(t, qualityReport)
}

func TestPhase3QualityGateRejectsMissingSonarResultReport(t *testing.T) {
	dir := t.TempDir()
	report := writePhase3QualityReport(t, dir, "950", false)
	qualityReport := filepath.Join(dir, "quality-report.md")
	sonarResultReport := filepath.Join(dir, "missing-sonar-result.md")
	fakeBin := writePhase3QualityFakeTools(t, dir)

	output, err := runPhase3Quality(t, report,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SONAR_HOST_URL=http://sonar.local",
		"SONAR_TOKEN=phase3-secret-token",
		"CETS_PHASE3_SONAR_COMMAND=true",
		"CETS_PHASE3_QUALITY_REPORT="+qualityReport,
		"CETS_PHASE3_SONAR_RESULT_REPORT="+sonarResultReport,
	)

	require.Error(t, err)
	assert.Contains(t, output, "Sonar result report does not exist or is empty")
	assert.NoFileExists(t, qualityReport)
}

func TestPhase3QualityGateRejectsFailedSonarQualityGate(t *testing.T) {
	dir := t.TempDir()
	report := writePhase3QualityReport(t, dir, "950", false)
	qualityReport := filepath.Join(dir, "quality-report.md")
	sonarResultReport := writePhase3SonarResultReport(t, dir, "failed", "0", "0", "0")
	fakeBin := writePhase3QualityFakeTools(t, dir)

	output, err := runPhase3Quality(t, report,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SONAR_HOST_URL=http://sonar.local",
		"SONAR_TOKEN=phase3-secret-token",
		"CETS_PHASE3_SONAR_COMMAND=true",
		"CETS_PHASE3_QUALITY_REPORT="+qualityReport,
		"CETS_PHASE3_SONAR_RESULT_REPORT="+sonarResultReport,
	)

	require.Error(t, err)
	assert.Contains(t, output, "Sonar quality gate must be passed, got: failed")
	assert.NoFileExists(t, qualityReport)
}

func TestPhase3QualityGateRejectsInvalidSonarIssueCounts(t *testing.T) {
	cases := []struct {
		name             string
		issues           string
		problems         string
		securityProblems string
		wantError        string
	}{
		{
			name:             "non-zero issues",
			issues:           "1",
			problems:         "0",
			securityProblems: "0",
			wantError:        "Sonar result report field 'Issues' must be 0, got: 1",
		},
		{
			name:             "non-zero problems",
			issues:           "0",
			problems:         "2",
			securityProblems: "0",
			wantError:        "Sonar result report field 'Problems' must be 0, got: 2",
		},
		{
			name:             "non-zero security problems",
			issues:           "0",
			problems:         "0",
			securityProblems: "3",
			wantError:        "Sonar result report field 'Security problems' must be 0, got: 3",
		},
		{
			name:             "malformed issues",
			issues:           "not-a-number",
			problems:         "0",
			securityProblems: "0",
			wantError:        "Sonar result report field 'Issues' must be 0, got: not-a-number",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			report := writePhase3QualityReport(t, dir, "950", false)
			qualityReport := filepath.Join(dir, "quality-report.md")
			sonarResultReport := writePhase3SonarResultReport(
				t,
				dir,
				"passed",
				tc.issues,
				tc.problems,
				tc.securityProblems,
			)
			fakeBin := writePhase3QualityFakeTools(t, dir)

			output, err := runPhase3Quality(t, report,
				"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"SONAR_HOST_URL=http://sonar.local",
				"SONAR_TOKEN=phase3-secret-token",
				"CETS_PHASE3_SONAR_COMMAND=true",
				"CETS_PHASE3_QUALITY_REPORT="+qualityReport,
				"CETS_PHASE3_SONAR_RESULT_REPORT="+sonarResultReport,
			)

			require.Error(t, err)
			assert.Contains(t, output, tc.wantError)
			assert.NoFileExists(t, qualityReport)
		})
	}
}

func runPhase3Quality(t *testing.T, report string, env ...string) (string, error) {
	t.Helper()
	script := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-quality.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "CETS_PHASE3_CAPACITY_REPORT="+report)
	cmd.Env = append(cmd.Env, env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func writePhase3QualityFakeTools(t *testing.T, dir string) string {
	t.Helper()
	fakeBin := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(fakeBin, 0o700))
	for _, name := range []string{"pnpm", "sonar-scanner"} {
		require.NoError(t, os.WriteFile(filepath.Join(fakeBin, name), []byte("#!/usr/bin/env sh\nexit 0\n"), 0o700))
	}
	return fakeBin
}

func writePhase3SonarResultCommand(
	status string,
	issues string,
	problems string,
	securityProblems string,
) string {
	lines := []string{
		"mkdir -p \"$(dirname \"$CETS_PHASE3_SONAR_RESULT_REPORT\")\"",
		"printf '%s\\n' " +
			"'# Sonar Result Report' " +
			"'' " +
			"'| Field | Value |' " +
			"'| --- | --- |' " +
			"'| Quality Gate Status | `" + status + "` |' " +
			"'| Issues | `" + issues + "` |' " +
			"'| Problems | `" + problems + "` |' " +
			"'| Security problems | `" + securityProblems + "` |' " +
			"> \"$CETS_PHASE3_SONAR_RESULT_REPORT\"",
	}
	return strings.Join(lines, " && ")
}

func writePhase3SonarResultReport(
	t *testing.T,
	dir string,
	status string,
	issues string,
	problems string,
	securityProblems string,
) string {
	t.Helper()
	report := filepath.Join(dir, "sonar-result.md")
	lines := []string{
		"# Sonar Result Report",
		"",
		"| Field | Value |",
		"| --- | --- |",
		"| Quality Gate Status | `" + status + "` |",
		"| Issues | `" + issues + "` |",
		"| Problems | `" + problems + "` |",
		"| Security problems | `" + securityProblems + "` |",
	}
	require.NoError(t, os.WriteFile(report, []byte(strings.Join(lines, "\n")), 0o600))
	return report
}

func writePhase3QualityReport(t *testing.T, dir string, rps string, emptyLGTM bool) string {
	t.Helper()
	capacityArtifacts := map[string]string{
		"k6.json":            `{"metrics":{}}`,
		"replica.txt":        "gateway|3\nfrontend|3\nbackend|3\n",
		"correctness.txt":    "matching_events|1\n",
		"prom-red.json":      `{"data":{"result":[{}]}}`,
		"prom-cpu.json":      `{"data":{"result":[{}]}}`,
		"prom-memory.json":   `{"data":{"result":[{}]}}`,
		"prom-db-pool.json":  `{"data":{"result":[{}]}}`,
		"prom-db-locks.json": `{"data":{"result":[{}]}}`,
	}
	for name, content := range capacityArtifacts {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}

	verifyReport := filepath.Join(dir, "phase3-verify-report.md")
	lgtmFields := []string{
		"Tempo trace evidence",
		"Service graph backend dependency evidence",
		"Pyroscope profile evidence",
		"Loki trace-log evidence",
		"Loki redaction evidence",
	}
	var verifyLines []string
	verifyLines = append(verifyLines, "# Phase 3 Verify Report", "", "| Field | Value |", "| --- | --- |")
	for i, field := range lgtmFields {
		artifact := filepath.Join(dir, "lgtm-"+strings.ToLower(strings.ReplaceAll(field, " ", "-"))+".json")
		content := []byte(`{"ok":true}`)
		if emptyLGTM && i == 0 {
			content = nil
		}
		require.NoError(t, os.WriteFile(artifact, content, 0o600))
		verifyLines = append(verifyLines, "| "+field+" | `"+artifact+"` |")
	}
	require.NoError(t, os.WriteFile(verifyReport, []byte(strings.Join(verifyLines, "\n")), 0o600))

	report := filepath.Join(dir, "capacity-report.md")
	lines := []string{
		"# Phase3 Capacity Report",
		"",
		"| Field | Value |",
		"| --- | --- |",
		"| Highest passing RPS | `" + rps + "` |",
		"| k6 summary | `" + filepath.Join(dir, "k6.json") + "` |",
		"| Replica spread summary | `" + filepath.Join(dir, "replica.txt") + "` |",
		"| Post-load correctness summary | `" + filepath.Join(dir, "correctness.txt") + "` |",
		"| Prometheus RED sample | `" + filepath.Join(dir, "prom-red.json") + "` |",
		"| Prometheus backend CPU sample | `" + filepath.Join(dir, "prom-cpu.json") + "` |",
		"| Prometheus backend memory sample | `" + filepath.Join(dir, "prom-memory.json") + "` |",
		"| Prometheus DB pool wait sample | `" + filepath.Join(dir, "prom-db-pool.json") + "` |",
		"| Prometheus global DB lock wait sample | `" + filepath.Join(dir, "prom-db-locks.json") + "` |",
		"| LGTM verify report | `" + verifyReport + "` |",
		"| Bottleneck | `db pool contention` |",
		"| Optimization result | `pool and query tuning verified` |",
	}
	require.NoError(t, os.WriteFile(report, []byte(strings.Join(lines, "\n")), 0o600))
	return report
}
