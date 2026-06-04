package deploy

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase3VerifyCapturesControlledErrorMetricBaselineBeforeOwnedRequest(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify.sh"))
	prometheus := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-prometheus-evidence.sh"))

	assert.Contains(t, script, "phase3-verify-prometheus-evidence.sh")
	assert.Contains(t, script, `. "$ROOT_DIR/scripts/compose/phase3-verify-prometheus-evidence.sh"`)
	assert.Contains(t, prometheus, "capture_prometheus_controlled_error_baseline() {")
	assert.Contains(t, prometheus, "check_prometheus_controlled_error_metrics() {")

	mainIndex := strings.Index(script, "main() {")
	require.NotEqual(t, -1, mainIndex)
	mainBody := script[mainIndex:]

	traceIndex := strings.Index(mainBody, "check_trace_ingest\n")
	baselineIndex := strings.Index(mainBody, "capture_prometheus_controlled_error_baseline\n")
	errorTraceIndex := strings.Index(mainBody, "check_error_trace_ingest\n")
	metricIndex := strings.Index(mainBody, "check_prometheus_controlled_error_metrics\n")
	require.NotEqual(t, -1, traceIndex)
	require.NotEqual(t, -1, baselineIndex)
	require.NotEqual(t, -1, errorTraceIndex)
	require.NotEqual(t, -1, metricIndex)

	assert.Less(t, traceIndex, baselineIndex)
	assert.Less(t, baselineIndex, errorTraceIndex)
	assert.Less(t, errorTraceIndex, metricIndex)
}

func TestPhase3VerifyPersistsReducedLokiTraceLogProof(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-trace-evidence.sh"))

	assert.Contains(t, script, `write_loki_trace_proof "$TEMPO_TRACE_ID" "$LOKI_TRACE_LOG_EVIDENCE" "backend hot-path"`)
	assert.NotContains(t, script, `printf '%s\n' "$logs" >"$LOKI_TRACE_LOG_EVIDENCE"`)
}

func TestPhase3VerifyPrometheusEvidenceHelpersAreSourced(t *testing.T) {
	verify := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify.sh"))
	prometheus := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-prometheus-evidence.sh"))

	assert.Contains(t, verify, "phase3-verify-prometheus-evidence.sh")
	assert.Contains(t, verify, `. "$ROOT_DIR/scripts/compose/phase3-verify-prometheus-evidence.sh"`)
	assert.Contains(t, prometheus, "prom_query() {")
	assert.Contains(t, prometheus, "check_prometheus_targets() {")
	assert.Contains(t, prometheus, "check_prometheus_red_metrics() {")
	assert.Contains(t, prometheus, "capture_prometheus_controlled_error_baseline() {")
	assert.Contains(t, prometheus, "check_prometheus_controlled_error_metrics() {")
	assert.NotContains(t, verify, "\nprom_query() {")
	assert.NotContains(t, verify, "\ncheck_prometheus_red_metrics() {")
}

func TestPhase3VerifyLgtmHealthHelpersAreSourced(t *testing.T) {
	verify := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify.sh"))
	lgtm := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-lgtm-health.sh"))

	assert.Contains(t, verify, "phase3-verify-lgtm-health.sh")
	assert.Contains(t, verify, `. "$ROOT_DIR/scripts/compose/phase3-verify-lgtm-health.sh"`)
	assert.Contains(t, lgtm, "require_running() {")
	assert.Contains(t, lgtm, "check_datasource() {")
	assert.Contains(t, lgtm, "wait_http_grep() {")
	assert.Contains(t, lgtm, "check_lgtm_health() {")
	assert.Contains(t, lgtm, `wait_http_grep "Prometheus" "$PROMETHEUS_URL/-/ready"`)
	assert.Contains(t, lgtm, `for uid in Prometheus Loki Tempo Pyroscope; do`)
	assert.NotContains(t, verify, "\ncheck_lgtm_health() {")
	assert.NotContains(t, verify, "\nwait_http_grep() {")
}

func TestPhase3VerifyPersistsReducedLokiRedactionProof(t *testing.T) {
	verify := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify.sh"))
	loki := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-loki-evidence.sh"))

	assert.Contains(t, verify, "phase3-verify-loki-evidence.sh")
	assert.Contains(t, verify, `. "$ROOT_DIR/scripts/compose/phase3-verify-loki-evidence.sh"`)
	assert.Contains(t, loki, `write_loki_redaction_proof "$LOKI_REDACTION_EVIDENCE"`)
	assert.Contains(t, loki, `raw_secret_absent: true`)
	assert.NotContains(t, loki, `printf '%s\n' "$redacted_logs" >"$LOKI_REDACTION_EVIDENCE"`)
}

func TestPhase3VerifyWritesLokiRedactionProofAfterStrictChecks(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-loki-evidence.sh"))
	checkIndex := strings.Index(script, "check_loki_logs_and_redaction() {")
	require.NotEqual(t, -1, checkIndex)
	proofHelperIndex := strings.Index(script, "\nwrite_loki_redaction_proof() {")
	require.NotEqual(t, -1, proofHelperIndex)
	require.Less(t, checkIndex, proofHelperIndex)
	checkBody := script[checkIndex:proofHelperIndex]

	canaryIndex := strings.Index(checkBody, `grep -q "phase3-redaction-canary"`)
	rawSecretIndex := strings.Index(checkBody, `grep -Eq "phase3-raw-(pii|signed|qr|provider|email-body|recipient-email|idempotency)-canary"`)
	markerIndex := strings.Index(checkBody, `grep -q "\[REDACTED\]"`)
	proofIndex := strings.Index(checkBody, `write_loki_redaction_proof "$LOKI_REDACTION_EVIDENCE"`)

	require.NotEqual(t, -1, canaryIndex)
	require.NotEqual(t, -1, rawSecretIndex)
	require.NotEqual(t, -1, markerIndex)
	require.NotEqual(t, -1, proofIndex)
	assert.Less(t, canaryIndex, rawSecretIndex)
	assert.Less(t, rawSecretIndex, markerIndex)
	assert.Less(t, markerIndex, proofIndex)
}

func TestPhase3VerifyChecksProfileDataBeforeLokiRedaction(t *testing.T) {
	verify := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify.sh"))
	profile := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-profile-evidence.sh"))

	assert.Contains(t, verify, "phase3-verify-profile-evidence.sh")
	assert.Contains(t, verify, `. "$ROOT_DIR/scripts/compose/phase3-verify-profile-evidence.sh"`)
	assert.Contains(t, profile, "check_profile_data() {")
	assert.Contains(t, profile, `process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="cets-backend"}`)
	assert.NotContains(t, verify, "\ncheck_profile_data() {")

	mainIndex := strings.Index(verify, "main() {")
	require.NotEqual(t, -1, mainIndex)
	mainBody := verify[mainIndex:]

	serviceGraphIndex := strings.Index(mainBody, "check_service_graph\n")
	profileIndex := strings.Index(mainBody, "check_profile_data\n")
	lokiIndex := strings.Index(mainBody, "check_loki_logs_and_redaction\n")
	require.NotEqual(t, -1, serviceGraphIndex)
	require.NotEqual(t, -1, profileIndex)
	require.NotEqual(t, -1, lokiIndex)

	assert.Less(t, serviceGraphIndex, profileIndex)
	assert.Less(t, profileIndex, lokiIndex)
}

func TestPhase3VerifyCapturesServiceGraphBaselineBeforeK6Load(t *testing.T) {
	script := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify.sh"))
	serviceGraph := readText(t, filepath.Join("..", "..", "..", "scripts", "compose", "phase3-verify-service-graph-evidence.sh"))

	prometheusSourceIndex := strings.Index(script, "phase3-verify-prometheus-evidence.sh")
	serviceGraphSourceIndex := strings.Index(script, "phase3-verify-service-graph-evidence.sh")
	require.NotEqual(t, -1, prometheusSourceIndex)
	require.NotEqual(t, -1, serviceGraphSourceIndex)
	assert.Less(t, prometheusSourceIndex, serviceGraphSourceIndex)
	assert.Contains(t, script, `. "$ROOT_DIR/scripts/compose/phase3-verify-service-graph-evidence.sh"`)
	assert.Contains(t, serviceGraph, "capture_service_graph_baseline() {")
	assert.Contains(t, serviceGraph, "check_service_graph() {")
	assert.Contains(t, serviceGraph, `prom_query "$SERVICE_GRAPH_INBOUND_QUERY"`)
	assert.NotContains(t, script, "\ncheck_service_graph() {")
	assert.NotContains(t, script, "\ncapture_service_graph_baseline() {")

	mainIndex := strings.Index(script, "main() {")
	require.NotEqual(t, -1, mainIndex)
	mainBody := script[mainIndex:]

	lgtmIndex := strings.Index(mainBody, "check_lgtm_health\n")
	targetsIndex := strings.Index(mainBody, "check_prometheus_targets\n")
	baselineIndex := strings.Index(mainBody, "capture_service_graph_baseline\n")
	k6Index := strings.Index(mainBody, "check_k6_distribution\n")
	graphIndex := strings.Index(mainBody, "check_service_graph\n")
	require.NotEqual(t, -1, lgtmIndex)
	require.NotEqual(t, -1, targetsIndex)
	require.NotEqual(t, -1, baselineIndex)
	require.NotEqual(t, -1, k6Index)
	require.NotEqual(t, -1, graphIndex)

	assert.Less(t, lgtmIndex, targetsIndex)
	assert.Less(t, targetsIndex, baselineIndex)
	assert.Less(t, baselineIndex, k6Index)
	assert.Less(t, k6Index, graphIndex)
}
