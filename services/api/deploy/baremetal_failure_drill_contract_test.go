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

func TestBaremetalVerifyAllKeepsFailureDrillStateless(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "99-verify-all.sh"))

	assert.Contains(t, script, `env BAREMETAL_DRILL_NODE_DRAIN=false "$SCRIPT_DIR/`+failureDrillScript+`"`)
	assert.Contains(t, script, `RUN_POSTGRES_FAILOVER=${RUN_POSTGRES_FAILOVER:-false}`)
	assert.Contains(t, script, `env BAREMETAL_PG_FAILOVER_PREFLIGHT_ONLY=true "$SCRIPT_DIR/`+postgresFailoverScript+`"`)
	assert.Contains(t, script, `set RUN_POSTGRES_FAILOVER=true to execute it after an approved disruption window`)
	assert.NotContains(t, script, `run_check "stateless workload and node failure drill" "$SCRIPT_DIR/`+failureDrillScript+`"`)
}

func TestBaremetalFailureDrillWritesReportAndCleanupTrap(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", failureDrillScript))

	for _, fragment := range []string{
		"BAREMETAL_DRILL_ARTIFACT_DIR",
		"BAREMETAL_DRILL_RUN_ID",
		"BAREMETAL_DRILL_NODE_DRAIN",
		"BAREMETAL_DRILL_DATABASE_NODE_DRAIN",
		"BAREMETAL_DRILL_PREFLIGHT_ONLY",
		"baremetal-failure-drill-events-$RUN_ID.txt",
		"baremetal-failure-drill-report-$RUN_ID.md",
		"cleanup_uncordon_attempted",
		"cleanup_uncordon_succeeded",
		"cleanup_uncordon_failed",
		"node_drain_skipped",
		"node_drain_blocked_database_pods",
		"node_drain_blocked_database_lookup_failed",
		"preflight_only_passed",
		"Node drain requested",
		"Preflight only",
		"Database node drain allowed",
		"Database pods on target",
		"Failure reason",
		"trap finalize EXIT",
	} {
		assert.Contains(t, script, fragment)
	}
}

func TestBaremetalFailureDrillPreflightOnlyBlocksDatabaseNodeDrainWithoutPodDelete(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	kubectlLog := filepath.Join(t.TempDir(), kubectlLogFile)
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"cnpg.io/cluster=cets-postgres"*) printf 'cets-postgres-3 '; exit 0;;
  *"get pod"*"app=backend"*) printf 'backend-a'; exit 0;;
  *"get pod"*"app=cloudflared"*) printf 'cloudflared-a'; exit 0;;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", failureDrillScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		baremetalDrillArtifact+artifactDir,
		"BAREMETAL_DRILL_RUN_ID=mock-db-drain-preflight",
		"BAREMETAL_DRILL_PREFLIGHT_ONLY=true",
		targetNodeWork2Env,
		fakeKubectlLogEnv+kubectlLog,
	)
	err := cmd.Run()
	require.Error(t, err)

	report := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-report-mock-db-drain-preflight.md"))
	events := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-events-mock-db-drain-preflight.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, statusBlockedRow)
	assert.Contains(t, report, "| Preflight only | `true` |")
	assert.Contains(t, report, "| Database pods on target | `cets-postgres-3` |")
	assert.Contains(t, events, "work2|node_drain_blocked_database_pods")
	assert.NotContains(t, kubectlCalls, "delete pod")
	assert.NotContains(t, kubectlCalls, cordonWork2Command)
	assert.NotContains(t, kubectlCalls, drainWork2Command)
}

func TestBaremetalFailureDrillPreflightOnlyPassesWithoutNodeDrain(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	kubectlLog := filepath.Join(t.TempDir(), kubectlLogFile)
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"cnpg.io/cluster=cets-postgres"*) printf ''; exit 0;;
  *"get pod"*"app=backend"*) printf 'backend-a'; exit 0;;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", failureDrillScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		baremetalDrillArtifact+artifactDir,
		"BAREMETAL_DRILL_RUN_ID=mock-drain-preflight-pass",
		"BAREMETAL_DRILL_PREFLIGHT_ONLY=true",
		"TARGET_NODE=work4",
		fakeKubectlLogEnv+kubectlLog,
	)
	require.NoError(t, cmd.Run())

	report := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-report-mock-drain-preflight-pass.md"))
	events := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-events-mock-drain-preflight-pass.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, "| Status | `preflight-passed` |")
	assert.Contains(t, report, "| Preflight only | `true` |")
	assert.Contains(t, events, "work4|preflight_only_passed")
	assert.NotContains(t, kubectlCalls, "delete pod")
	assert.NotContains(t, kubectlCalls, "cordon work4")
	assert.NotContains(t, kubectlCalls, "drain work4")
}

func TestBaremetalFailureDrillFailureReportUncordonsNode(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	curlCount := filepath.Join(t.TempDir(), "curl-count")
	kubectlLog := filepath.Join(t.TempDir(), kubectlLogFile)
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"get pod"*"app=backend"*) printf 'backend-a';;
  *"get pod"*"app=cloudflared"*) printf 'cloudflared-a';;
  *"get deployment cloudflared"*) exit 0;;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
set -euo pipefail
count=0
if [ -f "$FAKE_CURL_COUNT" ]; then count=$(cat "$FAKE_CURL_COUNT"); fi
count=$((count + 1))
printf '%s\n' "$count" >"$FAKE_CURL_COUNT"
if [ "$count" -ge 2 ]; then exit 22; fi
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", failureDrillScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		baremetalDrillArtifact+artifactDir,
		"BAREMETAL_DRILL_RUN_ID=mock-fail",
		"TARGET_NODE=work3",
		"FAKE_CURL_COUNT="+curlCount,
		fakeKubectlLogEnv+kubectlLog,
	)
	err := cmd.Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 22, exitErr.ExitCode())

	report := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-report-mock-fail.md"))
	events := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-events-mock-fail.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, "| Status | `failed` |")
	assert.Contains(t, events, "work3|cordoned")
	assert.Contains(t, events, "work3|drained")
	assert.Contains(t, events, drillFailedEvent)
	assert.Contains(t, events, "work3|cleanup_uncordon_attempted")
	assert.Contains(t, events, "work3|cleanup_uncordon_succeeded")
	assert.Contains(t, kubectlCalls, "uncordon work3")
}

func TestBaremetalFailureDrillDrainFailureRetriesUncordon(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	stateDir := t.TempDir()
	kubectlLog := filepath.Join(stateDir, kubectlLogFile)
	uncordonCount := filepath.Join(stateDir, "uncordon-count")
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"get pod"*"app=backend"*) printf 'backend-a'; exit 0;;
  *"get pod"*"app=cloudflared"*) printf 'cloudflared-a'; exit 0;;
  *"get deployment cloudflared"*) exit 0;;
  "drain work3"*) exit 1;;
  "uncordon work3")
    count=0
    if [ -f "$FAKE_UNCORDON_COUNT" ]; then count=$(cat "$FAKE_UNCORDON_COUNT"); fi
    count=$((count + 1))
    printf '%s\n' "$count" >"$FAKE_UNCORDON_COUNT"
    if [ "$count" -eq 1 ]; then exit 1; fi
    exit 0
    ;;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", failureDrillScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		baremetalDrillArtifact+artifactDir,
		"BAREMETAL_DRILL_RUN_ID=mock-drain-fail",
		"TARGET_NODE=work3",
		fakeKubectlLogEnv+kubectlLog,
		"FAKE_UNCORDON_COUNT="+uncordonCount,
	)
	err := cmd.Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())

	report := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-report-mock-drain-fail.md"))
	events := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-events-mock-drain-fail.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, "| Status | `failed` |")
	assert.Contains(t, events, "work3|cordoned")
	assert.Contains(t, events, "work3|drain_failed_uncordon_failed")
	assert.Contains(t, events, drillFailedEvent)
	assert.Contains(t, events, "work3|cleanup_uncordon_attempted")
	assert.Contains(t, events, "work3|cleanup_uncordon_succeeded")
	assert.Equal(t, 2, strings.Count(kubectlCalls, "uncordon work3"))
}

func TestBaremetalFailureDrillBlocksDatabaseNodeDrainWithoutOptIn(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	kubectlLog := filepath.Join(t.TempDir(), kubectlLogFile)
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"get pod"*"app=backend"*) printf 'backend-a'; exit 0;;
  *"get pod"*"app=cloudflared"*) printf 'cloudflared-a'; exit 0;;
  *"get deployment cloudflared"*) exit 0;;
  *"cnpg.io/cluster=cets-postgres"*) printf 'cets-postgres-3 '; exit 0;;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", failureDrillScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		baremetalDrillArtifact+artifactDir,
		"BAREMETAL_DRILL_RUN_ID=mock-db-drain-blocked",
		targetNodeWork2Env,
		fakeKubectlLogEnv+kubectlLog,
	)
	err := cmd.Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())

	report := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-report-mock-db-drain-blocked.md"))
	events := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-events-mock-db-drain-blocked.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, statusBlockedRow)
	assert.Contains(t, report, "| Node drain requested | `true` |")
	assert.Contains(t, report, "| Database node drain allowed | `false` |")
	assert.Contains(t, report, "| Database pods on target | `cets-postgres-3` |")
	assert.Contains(t, report, "database pods on target node require BAREMETAL_DRILL_DATABASE_NODE_DRAIN=true")
	assert.Contains(t, events, "work2|node_drain_blocked_database_pods")
	assert.Contains(t, events, drillFailedEvent)
	assert.NotContains(t, kubectlCalls, cordonWork2Command)
	assert.NotContains(t, kubectlCalls, drainWork2Command)
}

func TestBaremetalFailureDrillBlocksDatabaseLookupFailureBeforeDrain(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	kubectlLog := filepath.Join(t.TempDir(), kubectlLogFile)
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"get pod"*"app=backend"*) printf 'backend-a'; exit 0;;
  *"get pod"*"app=cloudflared"*) printf 'cloudflared-a'; exit 0;;
  *"get deployment cloudflared"*) exit 0;;
  *"cnpg.io/cluster=cets-postgres"*) exit 2;;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", failureDrillScript)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		baremetalDrillArtifact+artifactDir,
		"BAREMETAL_DRILL_RUN_ID=mock-db-lookup-fail",
		targetNodeWork2Env,
		fakeKubectlLogEnv+kubectlLog,
	)
	err := cmd.Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())

	report := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-report-mock-db-lookup-fail.md"))
	events := readText(t, filepath.Join(artifactDir, "baremetal-failure-drill-events-mock-db-lookup-fail.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, statusBlockedRow)
	assert.Contains(t, report, "could not inspect CloudNativePG pods on target node before drain")
	assert.Contains(t, events, "work2|node_drain_blocked_database_lookup_failed")
	assert.Contains(t, events, drillFailedEvent)
	assert.NotContains(t, kubectlCalls, cordonWork2Command)
	assert.NotContains(t, kubectlCalls, drainWork2Command)
}
