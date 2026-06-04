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

func TestBaremetalVerifyAllowsScaledReplicaSpread(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "60-verify.sh"))

	assert.Contains(t, script, "require_cmd jq")
	assert.Contains(t, script, `select(.status.phase == "Running")`)
	assert.Contains(t, script, `.type == "Ready" and .status == "True"`)
	assert.Contains(t, script, `"$pod_count" -ge 3`)
	assert.Contains(t, script, `expected app=$app to have at least 3 pods`)
	assert.Contains(t, script, `"$node_count" -ge 3`)
	assert.Contains(t, script, `expected app=$app to be spread across at least 3 nodes`)
	assert.NotContains(t, script, `"$pod_count" = "3"`)
	assert.NotContains(t, script, `"$node_count" = "3"`)
}

func TestBaremetalCapacityDoesNotExposeProviderTokenOnDockerCommandLine(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "64-benchmark-capacity.sh"))

	assert.Contains(t, script, `K6_ENV_FILES=()`)
	assert.Contains(t, script, `for env_file in "${K6_ENV_FILES[@]}"`)
	assert.Contains(t, script, `env_file=$(mktemp "$ARTIFACT_DIR/k6-env-$RUN_ID-rps-$rps.XXXXXX")`)
	assert.Contains(t, script, `K6_ENV_FILES+=("$env_file")`)
	assert.Contains(t, script, `chmod 0600 "$env_file"`)
	assert.Contains(t, script, `printf 'K6_PROVIDER_TOKEN_SECRET=%s\n' "$PROVIDER_TOKEN_SECRET"`)
	assert.Contains(t, script, `--env-file "$env_file"`)
	assert.Contains(t, script, `rm -f "$env_file"`)
	assert.NotContains(t, script, `-e K6_PROVIDER_TOKEN_SECRET="$PROVIDER_TOKEN_SECRET"`)
	assert.NotContains(t, script, `export PROVIDER_TOKEN_SECRET`)
}

func TestBaremetalErrorDemoUsesExpectedDefaultsAndEnvFileSecret(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "69-demo-error-rate.sh"))

	for _, fragment := range []string{
		`SCRIPT=/k6/k8s-error-rate-demo.js`,
		`ARTIFACT_DIR=${K8S_ERROR_DEMO_ARTIFACT_DIR:-$ROOT_DIR/artifacts/k8s-error-demo}`,
		`DURATION=${K8S_ERROR_DEMO_DURATION:-120s}`,
		`TARGET_RPS=${K8S_ERROR_DEMO_TARGET_RPS:-350}`,
		`ERROR_RATIO=${K8S_ERROR_DEMO_ERROR_RATIO:-0.4}`,
		`K6_ENV_FILE=$(mktemp "$ARTIFACT_DIR/k6-env-$RUN_ID.XXXXXX")`,
		`chmod 0600 "$K6_ENV_FILE"`,
		`printf 'K6_PROVIDER_TOKEN_SECRET=%s\n' "$(provider_secret)"`,
		`--env-file "$K6_ENV_FILE"`,
		`rm -f "$K6_ENV_FILE"`,
		`k8s_error_demo_expected_errors`,
		`http_req_failed`,
	} {
		assert.Contains(t, script, fragment)
	}
	assert.NotContains(t, script, `-e K6_PROVIDER_TOKEN_SECRET=`)
}

func TestBaremetalCapacityReportCapturesResourceEvidence(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "64-benchmark-capacity.sh"))

	for _, fragment := range []string{
		`prometheus-cpu-$RUN_ID.json`,
		`prometheus-memory-$RUN_ID.json`,
		`prometheus-restarts-$RUN_ID.json`,
		`prometheus-throttling-$RUN_ID.json`,
		`container_cpu_usage_seconds_total{namespace="cets",pod=~"backend-.*",container!="POD",container!=""}`,
		`container_memory_working_set_bytes{namespace="cets",pod=~"backend-.*"`,
		`kube_pod_container_status_restarts_total{namespace="cets",pod=~"backend-.*"`,
		`container_cpu_cfs_throttled_seconds_total{namespace="cets",pod=~"backend-.*"`,
		`require_prometheus_vector_sample "$prom_cpu" "backend CPU"`,
		`require_prometheus_vector_sample "$prom_memory" "backend memory"`,
		`require_prometheus_vector_sample "$prom_restarts" "backend restart"`,
		`Prometheus memory sample`,
		`Prometheus restart sample`,
		`Prometheus throttling sample`,
		`## Backend Memory Samples`,
		`## Backend Restart Samples`,
		`## Backend CPU Throttling Samples`,
		`.status == "success" and ((.data.result // []) | length > 0)`,
		`.status == "success" and ((.data.result // []) | length == 0)`,
		`No backend throttling samples were returned by Prometheus for this query.`,
		`Prometheus throttling query did not return a successful response; inspect`,
	} {
		assert.Contains(t, script, fragment)
	}
}

func TestBaremetalManifestsExposeCapacityReplicaHeaders(t *testing.T) {
	networking := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "30-networking.sh"))
	app := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "deploy-cets", "app.sh"))

	assert.Contains(t, networking, "server-snippet: |")
	assert.Contains(t, networking, `add_header X-CETS-Gateway-Replica \$hostname always;`)
	assert.NotContains(t, app, "nginx.ingress.kubernetes.io/configuration-snippet")
	assert.Contains(t, app, `add_header X-CETS-Frontend-Replica \$hostname always;`)
	assert.Equal(t, 2, strings.Count(app, `add_header X-CETS-Frontend-Replica \$hostname always;`))
	assert.Contains(t, app, `add_header Content-Type text/plain;`)
}

func TestBaremetalPostgresFailoverWritesReport(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	stateDir := t.TempDir()
	primaryMoved := filepath.Join(stateDir, "primary-moved")
	kubectlLog := filepath.Join(stateDir, "kubectl.log")
	curlLog := filepath.Join(stateDir, "curl.log")
	smokeScript := filepath.Join(fakeBin, "smoke-ok")
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"jsonpath={.status.currentPrimary}"*)
    if [ -f "$FAKE_PRIMARY_MOVED" ]; then printf 'cets-postgres-2'; else printf 'cets-postgres-1'; fi
    ;;
  *"jsonpath={.status.readyInstances}"*) printf '3';;
  *"jsonpath={.status.phase}"*) printf 'Cluster in healthy state';;
  *"get cluster cets-postgres"*) printf 'NAME READY STATUS PRIMARY\ncets-postgres 3 healthy cets-postgres-2\n';;
  *"delete pod cets-postgres-1"*) : >"$FAKE_PRIMARY_MOVED";;
  *"SHOW synchronous_standby_names"*) printf 'sync-standby\n';;
  *"pg_stat_replication"*) printf '1\n';;
  *"INSERT INTO audit_logs"*) : ;;
  *"SELECT count(*) FROM audit_logs"*) printf '1\n';;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_CURL_LOG"
exit 0
`)
	writeExecutable(t, smokeScript, `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "71-verify-postgres-failover.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BAREMETAL_PG_FAILOVER_ARTIFACT_DIR="+artifactDir,
		"BAREMETAL_PG_FAILOVER_RUN_ID=mock-pg-failover",
		"BAREMETAL_PG_FAILOVER_SMOKE_SCRIPT="+smokeScript,
		"POSTGRES_DB=cets",
		"POSTGRES_USER=cets",
		"FAKE_PRIMARY_MOVED="+primaryMoved,
		"FAKE_KUBECTL_LOG="+kubectlLog,
		"FAKE_CURL_LOG="+curlLog,
	)
	require.NoError(t, cmd.Run())

	report := readText(t, filepath.Join(artifactDir, "postgres-failover-report-mock-pg-failover.md"))
	events := readText(t, filepath.Join(artifactDir, "postgres-failover-events-mock-pg-failover.txt"))
	kubectlCalls := readText(t, kubectlLog)
	curlCalls := readText(t, curlLog)
	assert.Contains(t, report, "| Status | `passed` |")
	assert.Contains(t, report, "| Old primary | `cets-postgres-1` |")
	assert.Contains(t, report, "| New primary | `cets-postgres-2` |")
	assert.Contains(t, report, "| Probe audit ID | `ha-failover-probe-mock-pg-failover` |")
	assert.Contains(t, report, "| Failure reason | `none` |")
	assert.Contains(t, events, "cets-postgres-1|old_primary_detected")
	assert.Contains(t, events, "cets-postgres-1|sync_replication_before_ok")
	assert.Contains(t, events, "cets-postgres-1|probe_committed")
	assert.Contains(t, events, "cets-postgres-1|primary_deleted")
	assert.Contains(t, events, "cets-postgres-2|new_primary_detected")
	assert.Contains(t, events, "cets-postgres|cluster_healthy")
	assert.Contains(t, events, "cets-postgres-2|sync_replication_after_ok")
	assert.Contains(t, events, "cets-postgres-2|probe_survived")
	assert.Contains(t, events, "ingress|readyz_after_failover")
	assert.Contains(t, events, "app-smoke|passed")
	assert.Contains(t, kubectlCalls, "delete pod cets-postgres-1")
	assert.Contains(t, curlCalls, "/readyz")
}

func TestBaremetalPostgresFailoverPreflightsDangerousInputs(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "71-verify-postgres-failover.sh"))

	assert.Contains(t, script, `POSTGRES_DB=${POSTGRES_DB:-cets}`)
	assert.Contains(t, script, `POSTGRES_USER=${POSTGRES_USER:-postgres}`)
	assert.NotContains(t, script, `require_env POSTGRES_DB`)
	assert.NotContains(t, script, `require_env POSTGRES_USER`)
	assert.Contains(t, script, `[[ "$RUN_ID" =~ ^[A-Za-z0-9._-]+$ ]]`)
	assert.Contains(t, script, `[ -x "$SMOKE_SCRIPT" ]`)
	assert.Contains(t, script, "BAREMETAL_PG_FAILOVER_RUN_ID contains unsupported characters")
	assert.Contains(t, script, "PostgreSQL failover smoke script is not executable")
	assert.Contains(t, script, `old_primary=$(current_primary) || fail`)
	assert.Contains(t, script, "could not read synchronous_standby_names")
	assert.Contains(t, script, "could not read synchronous standby count")
	assert.Contains(t, script, "sync_state IN ('sync', 'quorum')")
	assert.Contains(t, script, "could not write failover probe")
	assert.Contains(t, script, "could not delete PostgreSQL primary pod")
	assert.Contains(t, script, "could not verify committed failover probe")
}

func TestBaremetalPostgresFailoverSmokePreflightWritesFailedReport(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	smokeScript := filepath.Join(t.TempDir(), "not-executable-smoke")
	require.NoError(t, os.WriteFile(smokeScript, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o600))
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "71-verify-postgres-failover.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BAREMETAL_PG_FAILOVER_ARTIFACT_DIR="+artifactDir,
		"BAREMETAL_PG_FAILOVER_RUN_ID=mock-smoke-preflight",
		"BAREMETAL_PG_FAILOVER_SMOKE_SCRIPT="+smokeScript,
		"POSTGRES_DB=cets",
		"POSTGRES_USER=cets",
	)
	err := cmd.Run()
	require.Error(t, err)

	report := readText(t, filepath.Join(artifactDir, "postgres-failover-report-mock-smoke-preflight.md"))
	events := readText(t, filepath.Join(artifactDir, "postgres-failover-events-mock-smoke-preflight.txt"))
	assert.Contains(t, report, "| Status | `failed` |")
	assert.Contains(t, report, "| Smoke script | `"+smokeScript+"` |")
	assert.Contains(t, report, "| Failure reason | `PostgreSQL failover smoke script is not executable: "+smokeScript+"` |")
	assert.Contains(t, events, "drill|failed")
}

func TestBaremetalPostgresFailoverPreflightOnlyStopsBeforePrimaryDelete(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	kubectlLog := filepath.Join(t.TempDir(), "kubectl.log")
	smokeScript := filepath.Join(fakeBin, "smoke-ok")
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"jsonpath={.status.currentPrimary}"*) printf 'cets-postgres-1';;
  *"SHOW synchronous_standby_names"*) printf 'ANY 1 ("cets-postgres-2","cets-postgres-3")\n';;
  *"pg_stat_replication"*) printf '2\n';;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)
	writeExecutable(t, smokeScript, `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "71-verify-postgres-failover.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BAREMETAL_PG_FAILOVER_ARTIFACT_DIR="+artifactDir,
		"BAREMETAL_PG_FAILOVER_RUN_ID=mock-pg-preflight",
		"BAREMETAL_PG_FAILOVER_PREFLIGHT_ONLY=true",
		"BAREMETAL_PG_FAILOVER_SMOKE_SCRIPT="+smokeScript,
		"POSTGRES_DB=cets",
		"POSTGRES_USER=cets",
		"FAKE_KUBECTL_LOG="+kubectlLog,
	)
	require.NoError(t, cmd.Run())

	report := readText(t, filepath.Join(artifactDir, "postgres-failover-report-mock-pg-preflight.md"))
	events := readText(t, filepath.Join(artifactDir, "postgres-failover-events-mock-pg-preflight.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, "| Status | `preflight-passed` |")
	assert.Contains(t, report, "| Old primary | `cets-postgres-1` |")
	assert.Contains(t, report, "| New primary | `not-run` |")
	assert.Contains(t, report, "| Probe audit ID | `not-run` |")
	assert.Contains(t, report, "| Failure reason | `none` |")
	assert.Contains(t, events, "cets-postgres-1|old_primary_detected")
	assert.Contains(t, events, "cets-postgres-1|sync_replication_before_ok")
	assert.Contains(t, events, "cets-postgres-1|preflight_only_passed")
	assert.NotContains(t, kubectlCalls, "INSERT INTO audit_logs")
	assert.NotContains(t, kubectlCalls, "delete pod cets-postgres-1")
}

func TestBaremetalPostgresFailoverWritesFailedReport(t *testing.T) {
	fakeBin := t.TempDir()
	artifactDir := t.TempDir()
	kubectlLog := filepath.Join(t.TempDir(), "kubectl.log")
	writeExecutable(t, filepath.Join(fakeBin, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_KUBECTL_LOG"
args="$*"
case "$args" in
  *"jsonpath={.status.currentPrimary}"*) printf 'cets-postgres-1';;
  *"SHOW synchronous_standby_names"*) printf 'sync-standby\n';;
  *"pg_stat_replication"*) printf '0\n';;
esac
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
exit 0
`)

	script := filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "71-verify-postgres-failover.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BAREMETAL_PG_FAILOVER_ARTIFACT_DIR="+artifactDir,
		"BAREMETAL_PG_FAILOVER_RUN_ID=mock-pg-fail",
		"POSTGRES_DB=cets",
		"POSTGRES_USER=cets",
		"FAKE_KUBECTL_LOG="+kubectlLog,
	)
	err := cmd.Run()
	require.Error(t, err)

	report := readText(t, filepath.Join(artifactDir, "postgres-failover-report-mock-pg-fail.md"))
	events := readText(t, filepath.Join(artifactDir, "postgres-failover-events-mock-pg-fail.txt"))
	kubectlCalls := readText(t, kubectlLog)
	assert.Contains(t, report, "| Status | `failed` |")
	assert.Contains(t, report, "| Old primary | `cets-postgres-1` |")
	assert.Contains(t, report, "| Failure reason | `expected at least one sync or quorum standby on cets-postgres-1, got 0` |")
	assert.Contains(t, events, "cets-postgres-1|old_primary_detected")
	assert.Contains(t, events, "drill|failed")
	assert.NotContains(t, kubectlCalls, "delete pod cets-postgres-1")
}
