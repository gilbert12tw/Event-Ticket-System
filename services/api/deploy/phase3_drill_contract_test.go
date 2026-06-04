package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase3DrillWritesFailedReportAndPreservesExitCode(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "docker"), `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "info" ]; then exit 0; fi
if [ "${1:-}" = "compose" ]; then
  shift
  case "$*" in
    *"ps -a -q"*) printf 'fake-container\n';;
  esac
  exit 0
fi
if [ "${1:-}" = "start" ]; then exit 0; fi
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 22
`)

	script := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-drill.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CETS_PHASE3_DRILL_ARTIFACT_DIR="+artifactDir,
		"CETS_PHASE3_DRILL_RUN_ID=mock-fail",
	)
	err := cmd.Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 22, exitErr.ExitCode())

	report := readText(t, filepath.Join(artifactDir, "phase3-drill-report-mock-fail.md"))
	events := readText(t, filepath.Join(artifactDir, "phase3-drill-events-mock-fail.txt"))
	assert.Contains(t, report, "| Status | `failed` |")
	assert.Contains(t, events, "gateway-1|stopped")
	assert.Contains(t, events, "gateway-1|failed")
	assert.Contains(t, events, "gateway-1|cleanup_restart_attempted")
	assert.Contains(t, events, "gateway-1|cleanup_restart_started")
}
