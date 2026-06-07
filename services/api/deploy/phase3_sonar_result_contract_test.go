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

func TestPhase3SonarResultWrapperDeclaresEvidenceContract(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", phase3SonarResultScript))
	qualityScript := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-quality.sh"))

	for _, fragment := range []string{
		"pnpm test:coverage",
		"sonar-scanner",
		".scannerwork/report-task.txt",
		"ceTaskId",
		"ceTaskUrl",
		"/api/ce/task",
		"/api/qualitygates/project_status",
		"/api/issues/search",
		"/api/hotspots/search",
		"Authorization: Bearer $SONAR_TOKEN",
		"CETS_PHASE3_SONAR_RESULT_REPORT",
		"CETS_PHASE3_SONAR_PROJECT_KEY",
		"Quality Gate Status",
		"Issues",
		"Problems",
		"Security problems",
		"Sonar quality gate must be passed",
		"Sonar issues must be 0",
		"Sonar security problems must be 0",
	} {
		assert.Contains(t, script, fragment)
	}
	assert.Contains(t, qualityScript, "scripts/compose/phase3-sonar-result.sh")
	assert.NotContains(t, script, "SONAR_TOKEN=")
}

func TestPhase3SonarResultWrapperPreservesScannerTaskURLQuery(t *testing.T) {
	dir := t.TempDir()
	resultReport := filepath.Join(dir, sonarResultReportFile)
	taskFile := filepath.Join(dir, "scannerwork", sonarTaskFile)
	fakeBin := writePhase3SonarResultFakeTools(t, dir)

	output, err := runPhase3SonarResult(t, fakeBin,
		sonarHostLocalEnv,
		sonarTokenEnv,
		sonarResultReportEnv+resultReport,
		sonarTaskFileEnv+taskFile,
		sonarWaitDisabledEnv,
	)

	require.NoError(t, err, output)
	assert.Contains(t, readText(t, resultReport), "| Quality Gate Status | `OK` |")
	assert.NotContains(t, output, "missing Sonar CE task id")
}

func TestPhase3SonarResultWrapperWritesPassedResultReport(t *testing.T) {
	dir := t.TempDir()
	resultReport := filepath.Join(dir, sonarResultReportFile)
	taskFile := filepath.Join(dir, "scannerwork", sonarTaskFile)
	fakeBin := writePhase3SonarResultFakeTools(t, dir)

	output, err := runPhase3SonarResult(t, fakeBin,
		sonarHostLocalEnv,
		sonarTokenEnv,
		sonarResultReportEnv+resultReport,
		sonarTaskFileEnv+taskFile,
		sonarWaitDisabledEnv,
	)

	require.NoError(t, err, output)
	result := readText(t, resultReport)
	assert.Contains(t, output, "wrote Sonar result report")
	assert.Contains(t, result, "| Quality Gate Status | `OK` |")
	assert.Contains(t, result, "| Issues | `0` |")
	assert.Contains(t, result, "| Problems | `0` |")
	assert.Contains(t, result, "| Security problems | `0` |")
	assert.NotContains(t, output, sonarTokenValue)
	assert.NotContains(t, result, sonarTokenValue)
}

func TestPhase3SonarResultWrapperRejectsFailedQualityGate(t *testing.T) {
	dir := t.TempDir()
	resultReport := filepath.Join(dir, sonarResultReportFile)
	taskFile := filepath.Join(dir, "scannerwork", sonarTaskFile)
	fakeBin := writePhase3SonarResultFakeTools(t, dir)

	output, err := runPhase3SonarResult(t, fakeBin,
		sonarHostLocalEnv,
		sonarTokenEnv,
		sonarResultReportEnv+resultReport,
		sonarTaskFileEnv+taskFile,
		sonarWaitDisabledEnv,
		"CETS_FAKE_SONAR_QUALITY_STATUS=ERROR",
	)

	require.Error(t, err)
	assert.Contains(t, output, "Sonar quality gate must be passed, got: ERROR")
	assert.Contains(t, readText(t, resultReport), "| Quality Gate Status | `ERROR` |")
	assert.NotContains(t, output, sonarTokenValue)
}

func TestPhase3SonarResultWrapperRejectsNonZeroIssueTotals(t *testing.T) {
	dir := t.TempDir()
	resultReport := filepath.Join(dir, sonarResultReportFile)
	taskFile := filepath.Join(dir, "scannerwork", sonarTaskFile)
	fakeBin := writePhase3SonarResultFakeTools(t, dir)

	output, err := runPhase3SonarResult(t, fakeBin,
		sonarHostLocalEnv,
		sonarTokenEnv,
		sonarResultReportEnv+resultReport,
		sonarTaskFileEnv+taskFile,
		sonarWaitDisabledEnv,
		"CETS_FAKE_SONAR_HOTSPOTS=2",
	)

	require.Error(t, err)
	assert.Contains(t, output, "Sonar security problems must be 0, got: 2")
	assert.Contains(t, readText(t, resultReport), "| Security problems | `2` |")
	assert.NotContains(t, output, sonarTokenValue)
}

func TestPhase3SonarResultWrapperRejectsMissingScannerTaskFile(t *testing.T) {
	dir := t.TempDir()
	resultReport := filepath.Join(dir, sonarResultReportFile)
	taskFile := filepath.Join(dir, "scannerwork", sonarTaskFile)
	fakeBin := writePhase3SonarResultFakeTools(t, dir)

	output, err := runPhase3SonarResult(t, fakeBin,
		sonarHostLocalEnv,
		sonarTokenEnv,
		sonarResultReportEnv+resultReport,
		sonarTaskFileEnv+taskFile,
		"CETS_FAKE_SONAR_SKIP_TASK_FILE=true",
	)

	require.Error(t, err)
	assert.Contains(t, output, "scanner task file does not exist or is empty")
	assert.NoFileExists(t, resultReport)
	assert.NotContains(t, output, sonarTokenValue)
}

func runPhase3SonarResult(t *testing.T, fakeBin string, env ...string) (string, error) {
	t.Helper()
	script := filepath.Join("..", "..", "..", "scripts", "compose", phase3SonarResultScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func writePhase3SonarResultFakeTools(t *testing.T, dir string) string {
	t.Helper()
	fakeBin := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(fakeBin, 0o700))
	writeFakeExecutable(t, fakeBin, "pnpm", "#!/usr/bin/env sh\nexit 0\n")
	writeFakeExecutable(t, fakeBin, "jq", "#!/usr/bin/env sh\nexec /usr/bin/jq \"$@\"\n")
	writeFakeExecutable(t, fakeBin, "sonar-scanner", phase3FakeSonarScanner())
	writeFakeExecutable(t, fakeBin, "curl", phase3FakeSonarCurl())
	return fakeBin
}

func writeFakeExecutable(t *testing.T, dir string, name string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o700))
}

func phase3FakeSonarScanner() string {
	return `#!/usr/bin/env sh
set -eu
[ "${CETS_FAKE_SONAR_SKIP_TASK_FILE:-false}" = "true" ] && exit 0
task_file="${CETS_PHASE3_SONAR_TASK_FILE:-.scannerwork/report-task.txt}"
mkdir -p "$(dirname "$task_file")"
cat > "$task_file" <<EOF
projectKey=event-ticket-system
serverUrl=${SONAR_HOST_URL}
ceTaskId=phase3-task
ceTaskUrl=${SONAR_HOST_URL}/api/ce/task?id=phase3-task
EOF
`
}

func phase3FakeSonarCurl() string {
	return `#!/usr/bin/env sh
set -eu
args="$*"
case "$args" in
  *"/api/ce/task"*)
    case "$args" in
      *"id=phase3-task"*) ;;
      *)
        printf 'missing Sonar CE task id in curl args: %s\n' "$args" >&2
        exit 1
        ;;
    esac
    printf '{"task":{"status":"SUCCESS","analysisId":"analysis-1"}}'
    ;;
  *"/api/qualitygates/project_status"*)
    printf '{"projectStatus":{"status":"%s"}}' "${CETS_FAKE_SONAR_QUALITY_STATUS:-OK}"
    ;;
  *"types=VULNERABILITY,SECURITY_HOTSPOT"*)
    printf 'invalid issue types: %s\n' "$args" >&2
    exit 1
    ;;
  *"types=VULNERABILITY"*)
    printf '{"total":%s}' "${CETS_FAKE_SONAR_VULNERABILITIES:-0}"
    ;;
  *"types=BUG,CODE_SMELL"*)
    printf '{"total":%s}' "${CETS_FAKE_SONAR_PROBLEMS:-0}"
    ;;
  *"/api/hotspots/search"*)
    printf '{"paging":{"total":%s}}' "${CETS_FAKE_SONAR_HOTSPOTS:-0}"
    ;;
  *"/api/issues/search"*)
    printf '{"total":%s}' "${CETS_FAKE_SONAR_ISSUES:-0}"
    ;;
  *)
    printf 'unexpected curl args: %s\n' "$args" >&2
    exit 1
    ;;
esac
`
}

func TestPhase3SonarResultWrapperUsesProjectKeyForIssueQueries(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", phase3SonarResultScript))

	for _, fragment := range []string{
		`--data-urlencode "componentKeys=$project_key"`,
		`--data-urlencode "types=BUG,CODE_SMELL"`,
		`--data-urlencode "types=VULNERABILITY"`,
		`--data-urlencode "projectKey=$project_key"`,
		`--data-urlencode "status=TO_REVIEW"`,
	} {
		assert.Contains(t, script, fragment)
	}
	assert.NotContains(t, strings.ToLower(script), "echo $sonar_token")
}
