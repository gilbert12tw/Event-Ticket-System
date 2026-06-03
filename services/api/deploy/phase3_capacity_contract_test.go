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

func TestPhase3CapacityReportK6MetricExtraction(t *testing.T) {
	requireCommand(t, "jq")
	summary := `{
	  "metrics": {
	    "http_reqs": {"count": 300, "rate": 250.5},
	    "http_req_failed{expected_error:false}": {"value": 0.004},
	    "checks": {"value": 0.995},
	    "http_req_duration": {"med": 10, "p(95)": 20, "p(99)": 30},
	    "http_req_duration{flow:read}": {"med": 11, "p(95)": 21, "p(99)": 31},
	    "k8s_booking_duration": {"med": 12, "p(95)": 22, "p(99)": 32},
	    "k8s_booking_conflicts": {"count": 2},
	    "k8s_gateway_replica_hits": {"count": 3},
	    "k8s_frontend_replica_hits": {"count": 4},
	    "k8s_backend_replica_hits": {"count": 5}
	  }
	}`
	filterPath := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-k6-report.jq")
	cmd := exec.Command("jq", "-r", "--arg", "target_rps", "250", "-f", filterPath)
	cmd.Stdin = strings.NewReader(summary)
	output, err := cmd.Output()
	require.NoError(t, err)
	content := string(output)

	for _, fragment := range []string{
		"target RPS: `250`",
		"HTTP throughput req/s: `250.5`",
		"unexpected error rate: `0.004`",
		"check pass rate: `0.995`",
		"http_req_duration p50: `10`",
		"read flow p99: `31`",
		"booking p99: `32`",
		"booking conflicts: `2`",
		"gateway replica hit samples: `3`",
		"frontend replica hit samples: `4`",
		"backend replica hit samples: `5`",
	} {
		assert.Contains(t, content, fragment)
	}
}

func TestPhase3K6LoadScriptDeclaresDistributionAndErrorContracts(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "..", "k6", "phase3-ha-lgtm.js")
	capacityScriptPath := filepath.Join("..", "..", "..", "k6", "k8s-capacity-rps.js")
	require.FileExists(t, scriptPath)
	script := readText(t, scriptPath)
	capacityScript := readText(t, capacityScriptPath)
	wrapper := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-k6.sh"))

	for _, fragment := range []string{
		"phase3_gateway_replica_hits",
		"phase3_frontend_replica_hits",
		"phase3_backend_replica_hits",
		"phase3_controlled_errors",
		"controlledErrorTraffic",
		"investigateBackendHotspot",
		"X-CETS-Gateway-Replica",
		"X-CETS-Frontend-Replica",
		"X-CETS-Backend-Replica",
	} {
		assert.Contains(t, script, fragment)
	}
	for _, fragment := range []string{
		"k8s_gateway_replica_hits",
		"k8s_frontend_replica_hits",
		"k8s_backend_replica_hits",
		"recordReplicas",
		"X-CETS-Gateway-Replica",
		"X-CETS-Frontend-Replica",
		"X-CETS-Backend-Replica",
	} {
		assert.Contains(t, capacityScript, fragment)
	}
	for _, fragment := range []string{
		"HEADER_REPLICAS_AWK=",
		"REPLICA_SPREAD_CHECK=",
		"REPLICA_SPREAD_FILE=",
		"phase3-${PROFILE}-replica-spread.txt",
		"phase3-header-replicas.awk",
		"phase3-replica-spread-check.awk",
		`rm -f "$REPLICA_SPREAD_FILE"`,
		`spread_tmp="$REPLICA_SPREAD_FILE.tmp.$$"`,
		"K6_PHASE3_REPLICA_SAMPLES",
		"require_replica_spread_summary",
		"Phase 3 k6 replica spread invariants failed",
		"printf 'gateway|%s\\n'",
		"printf 'frontend|%s\\n'",
		"printf 'backend|%s\\n'",
		"printf 'headers|%s\\n'",
		`require_replica_spread_summary "$spread_tmp"`,
		`mv "$spread_tmp" "$REPLICA_SPREAD_FILE"`,
		"grafana/k6",
	} {
		assert.Contains(t, wrapper, fragment)
	}
}

func TestPhase3CapacityScriptRequiresDistinctReplicaSpread(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity.sh"))
	helper := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-replica-evidence.sh"))
	report := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-report.sh"))
	checker := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-replica-spread-check.awk"))

	for _, fragment := range []string{
		"CETS_PHASE3_CAPACITY_REPLICA_SAMPLES",
		"phase3-capacity-report.sh",
		`. "$ROOT_DIR/scripts/compose/phase3-capacity-report.sh"`,
		"phase3-capacity-replica-evidence.sh",
		`. "$ROOT_DIR/scripts/compose/phase3-capacity-replica-evidence.sh"`,
		"phase3-header-replicas.awk",
		"phase3-replica-spread-check.awk",
	} {
		assert.Contains(t, script, fragment)
	}

	for _, fragment := range []string{
		"capacity-replica-headers-$RUN_ID-rps-$rps.txt",
		"capacity-replica-spread-$RUN_ID-rps-$rps.txt",
		"awk -v header=X-CETS-Gateway-Replica",
		"awk -v header=X-CETS-Frontend-Replica",
		"awk -v header=X-CETS-Backend-Replica",
		"$HEADER_REPLICAS_AWK",
		"$REPLICA_SPREAD_CHECK",
		"require_replica_spread_summary",
	} {
		assert.Contains(t, helper, fragment)
	}
	assert.Contains(t, helper, "write_replica_spread() {")
	assert.Contains(t, helper, "print_replica_spread() {")
	assert.Contains(t, helper, "require_replica_spread_summary() {")
	assert.NotContains(t, script, "\nwrite_replica_spread() {")
	assert.NotContains(t, script, "\nrequire_replica_spread_summary() {")

	for _, fragment := range []string{
		"Replica spread summary",
		"## Replica Spread",
		`[ -s "$replica_spread" ] || write_replica_spread "$best_rps"`,
		`require_replica_spread_summary "$replica_spread"`,
		`print_replica_spread "$replica_spread"`,
	} {
		assert.Contains(t, report, fragment)
	}

	for _, fragment := range []string{
		`required = "gateway frontend backend"`,
		`expected at least 3 " tier " replicas, got `,
		"missing \" tier \" replica spread evidence",
	} {
		assert.Contains(t, checker, fragment)
	}
}

func TestPhase3CapacityK6SearchHelpersAreSourced(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity.sh"))
	helper := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-k6-search.sh"))

	assert.Contains(t, script, "phase3-capacity-k6-search.sh")
	assert.Contains(t, script, `. "$ROOT_DIR/scripts/compose/phase3-capacity-k6-search.sh"`)
	assert.Contains(t, helper, "run_k6_candidate() {")
	assert.Contains(t, helper, "candidate_passes() {")
	assert.Contains(t, helper, "find_max_rps() {")
	assert.Contains(t, helper, `"$K6_IMAGE" run --summary-export`)
	assert.Contains(t, helper, `write_replica_spread "$rps"`)
	assert.Contains(t, helper, `last-pass-rps-$RUN_ID.txt`)
	assert.Contains(t, helper, `last-fail-rps-$RUN_ID.txt`)
	assert.Contains(t, helper, `while [ "$current" -le "$MAX_RPS" ]; do`)
	assert.Contains(t, helper, `while [ $((high - low)) -gt "$RESOLUTION_RPS" ]; do`)
	assert.NotContains(t, script, "\nrun_k6_candidate() {")
	assert.NotContains(t, script, "\nfind_max_rps() {")
	assert.Contains(t, script, `best_rps=$(find_max_rps)`)
}

func TestPhase3HeaderReplicaCounterCountsDistinctValues(t *testing.T) {
	requireCommand(t, "awk")
	filterPath := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-header-replicas.awk")
	headers := strings.Join([]string{
		"X-CETS-Gateway-Replica: gateway-1",
		"X-CETS-Gateway-Replica: gateway-2",
		"X-CETS-Gateway-Replica: gateway-2",
		"X-CETS-Gateway-Replica: gateway-3",
		"X-CETS-Frontend-Replica: frontend-1",
	}, "\n")
	cmd := exec.Command("awk", "-v", "header=X-CETS-Gateway-Replica", "-f", filterPath)
	cmd.Stdin = strings.NewReader(headers)
	output, err := cmd.Output()
	require.NoError(t, err)

	assert.Equal(t, "3", strings.TrimSpace(string(output)))
}

func TestPhase3ReplicaSpreadSummaryCheck(t *testing.T) {
	requireCommand(t, "awk")
	filterPath := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-replica-spread-check.awk")
	passing := strings.Join([]string{
		"gateway|3",
		"frontend|3",
		"backend|3",
		"headers|/tmp/replica-headers.txt",
	}, "\n")

	require.NoError(t, runAwkReplicaSpreadCheck(t, filterPath, passing))

	for name, content := range map[string]string{
		"low gateway spread": strings.Replace(passing, "gateway|3", "gateway|1", 1),
		"missing frontend":   strings.Replace(passing, "frontend|3\n", "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, runAwkReplicaSpreadCheck(t, filterPath, content))
		})
	}
}

func TestPhase3CapacityReportPrometheusVectorFormatting(t *testing.T) {
	requireCommand(t, "jq")
	response := `{
	  "data": {
	    "result": [
	      {"metric": {"instance": "backend-1:8080"}, "value": [1760000000, "0.125"]},
	      {"metric": {"instance": "backend-2:8080"}, "value": [1760000000, "0.250"]},
	      {"metric": {"instance": "backend-3:8080"}, "value": [1760000000, "0.375"]}
	    ]
	  }
	}`
	filterPath := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-prometheus-vector-report.jq")
	cmd := exec.Command("jq", "-r", "-f", filterPath)
	cmd.Stdin = strings.NewReader(response)
	output, err := cmd.Output()
	require.NoError(t, err)
	content := string(output)

	assert.Contains(t, content, "backend-1:8080")
	assert.Contains(t, content, "`0.125`")
	assert.Contains(t, content, "backend-2:8080")
	assert.Contains(t, content, "`0.250`")
	assert.Contains(t, content, "backend-3:8080")
	assert.Contains(t, content, "`0.375`")
}

func TestPhase3CapacityPrometheusEvidenceHelpersAreSourced(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity.sh"))
	helper := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-prometheus-evidence.sh"))
	report := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-report.sh"))

	assert.Contains(t, script, "phase3-capacity-prometheus-evidence.sh")
	assert.Contains(t, script, `. "$ROOT_DIR/scripts/compose/phase3-capacity-prometheus-evidence.sh"`)
	assert.Contains(t, helper, "prom_query() {")
	assert.Contains(t, helper, "require_prometheus_vector_sample() {")
	assert.Contains(t, helper, "require_prometheus_vector_any_sample() {")
	assert.Contains(t, helper, "prom_scalar() {")
	assert.Contains(t, helper, "require_prometheus_evidence() {")
	assert.Contains(t, helper, `PHASE3_BACKEND_PROMETHEUS_TARGETS`)
	assert.NotContains(t, script, "\nprom_query() {")
	assert.NotContains(t, script, "\nrequire_prometheus_evidence() {")
	assert.Contains(t, report, "require_prometheus_evidence")
	assert.Contains(t, report, `require_prometheus_vector_sample "$prom_cpu" "CPU"`)
}

func TestPhase3CapacityCorrectnessEvidenceHelpersAreSourced(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity.sh"))
	helper := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-correctness-evidence.sh"))
	report := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-report.sh"))

	assert.Contains(t, script, "phase3-capacity-correctness-evidence.sh")
	assert.Contains(t, script, `. "$ROOT_DIR/scripts/compose/phase3-capacity-correctness-evidence.sh"`)
	assert.Contains(t, helper, "write_correctness_summary() {")
	assert.Contains(t, helper, "require_post_load_correctness() {")
	assert.Contains(t, helper, "print_correctness_summary() {")
	assert.Contains(t, helper, `psql_with_event_title "$event_title" "$CORRECTNESS_SUMMARY_SQL"`)
	assert.Contains(t, helper, `awk -f "$CORRECTNESS_CHECK" "$summary"`)
	assert.NotContains(t, script, "\nwrite_correctness_summary() {")
	assert.NotContains(t, script, "\nrequire_post_load_correctness() {")
	assert.Contains(t, report, `write_correctness_summary "$best_rps" "$correctness"`)
	assert.Contains(t, report, `require_post_load_correctness "$correctness"`)
	assert.Contains(t, report, `print_correctness_summary "$correctness"`)
}

func TestPhase3CapacityPrometheusTargetGuardRequiresEveryBackend(t *testing.T) {
	requireCommand(t, "jq")
	filterPath := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-prometheus-backend-targets-required.jq")
	complete := `{"data":{"result":[
	  {"metric":{"instance":"backend-1:8080"},"value":[1760000000,"0.125"]},
	  {"metric":{"instance":"backend-2:8080"},"value":[1760000000,"0.250"]},
	  {"metric":{"instance":"backend-3:8080"},"value":[1760000000,"0.375"]}
	]}}`
	missingBackend := `{"data":{"result":[
	  {"metric":{"instance":"backend-1:8080"},"value":[1760000000,"0.125"]},
	  {"metric":{"instance":"backend-2:8080"},"value":[1760000000,"0.250"]}
	]}}`

	completeCmd := exec.Command("jq", "-e", "--arg", "expected_targets", "backend-1:8080 backend-2:8080 backend-3:8080", "-f", filterPath)
	completeCmd.Stdin = strings.NewReader(complete)
	require.NoError(t, completeCmd.Run())

	missingCmd := exec.Command("jq", "-e", "--arg", "expected_targets", "backend-1:8080 backend-2:8080 backend-3:8080", "-f", filterPath)
	missingCmd.Stdin = strings.NewReader(missingBackend)
	assert.Error(t, missingCmd.Run())
}

func TestPhase3CapacityPostLoadCorrectnessCheck(t *testing.T) {
	requireCommand(t, "awk")
	filterPath := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-correctness-check.awk")
	passing := strings.Join([]string{
		"matching_events|1",
		"capacity|3",
		"confirmed|2",
		"waitlisted|1",
		"tickets|2",
		"confirmed_missing_ticket|0",
		"confirmed_inactive_ticket|0",
		"duplicate_active_registrations|0",
		"duplicate_checkins|0",
		"booking_confirmed_audits|2",
		"ticket_issued_audits|2",
	}, "\n")

	require.NoError(t, runAwkCorrectnessCheck(t, filterPath, passing))

	for name, content := range map[string]string{
		"duplicate event": strings.Replace(passing, "matching_events|1", "matching_events|2", 1),
		"missing ticket":  strings.Replace(passing, "confirmed_missing_ticket|0", "confirmed_missing_ticket|1", 1),
		"audit drift":     strings.Replace(passing, "ticket_issued_audits|2", "ticket_issued_audits|1", 1),
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, runAwkCorrectnessCheck(t, filterPath, content))
		})
	}
}

func TestPhase3CapacityCorrectnessSummarySQLIsWired(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity.sh"))
	helper := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-correctness-evidence.sh"))
	sql := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-correctness-summary.sql"))
	report := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-capacity-report.sh"))

	assert.Contains(t, script, "CORRECTNESS_SUMMARY_SQL=")
	assert.Contains(t, script, "phase3-capacity-correctness-evidence.sh")
	assert.Contains(t, helper, `psql_with_event_title "$event_title" "$CORRECTNESS_SUMMARY_SQL"`)
	assert.Contains(t, script, `-v event_title="$EVENT_TITLE"`)
	assert.Contains(t, sql, "WHERE title = :'event_title'")
	assert.Contains(t, report, `write_correctness_summary "$best_rps" "$correctness"`)
	assert.Contains(t, report, `require_post_load_correctness "$correctness"`)

	for _, label := range []string{
		"matching_events",
		"confirmed_missing_ticket",
		"confirmed_inactive_ticket",
		"duplicate_active_registrations",
		"duplicate_checkins",
		"booking_confirmed_audits",
		"ticket_issued_audits",
	} {
		assert.Contains(t, sql, label)
	}
}

func runAwkCorrectnessCheck(t *testing.T, filterPath string, content string) error {
	t.Helper()
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.txt")
	require.NoError(t, os.WriteFile(summaryPath, []byte(content+"\n"), 0o600))
	return exec.Command("awk", "-f", filterPath, summaryPath).Run()
}

func runAwkReplicaSpreadCheck(t *testing.T, filterPath string, content string) error {
	t.Helper()
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "replica-spread.txt")
	require.NoError(t, os.WriteFile(summaryPath, []byte(content+"\n"), 0o600))
	return exec.Command("awk", "-f", filterPath, summaryPath).Run()
}
