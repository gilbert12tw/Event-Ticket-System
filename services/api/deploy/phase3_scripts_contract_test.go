package deploy

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase3ComposeScriptsDeclareDeployVerifyAndDrillContracts(t *testing.T) {
	scripts := readFilesUnder(t, filepath.Join("..", "..", "..", "scripts", "compose"))

	for _, fragment := range []string{
		"compose.phase3-ha.yaml",
		"docker daemon access is required",
		"phase3-k6.sh",
		"phase3-ha-lgtm.js",
		"K6_PHASE3_PROFILE",
		"K6_PHASE3_REPLICA_SAMPLES",
		"phase3-header-replicas.awk",
		"phase3-replica-spread-check.awk",
		"phase3-${PROFILE}-replica-spread.txt",
		"require_replica_spread_summary",
		"--profile phase3-ha",
		"--profile worker-isolation",
		"--profile observability",
		"--profile phase3-canary",
		"GRAFANA_ADMIN_USER",
		"GRAFANA_ADMIN_PASSWORD",
		"backend-1 backend-2 backend-3",
		"frontend-1 frontend-2 frontend-3",
		"gateway-1 gateway-2 gateway-3",
		"EDGE_URL",
		"/healthz",
		"/readyz",
		"api/datasources/uid",
		`up{job=\"cets-backend\",instance=\"$target\"}`,
		"for target in backend-1:8080 backend-2:8080 backend-3:8080",
		"Prometheus cets-backend target $target is not scraping up",
		"api/search?tags=service.name%3Dcets-backend&limit=20",
		"phase3-tempo-trace-ids.jq",
		"tempo_trace_ids",
		`for trace_id in $(printf '%s\n' "$traces" | tempo_trace_ids)`,
		"api/traces/$trace_id",
		"tempo_trace_has_backend_route",
		`select(.key == "service.name")`,
		`any(. == "cets-backend")`,
		`select(.key == "http.route" or .key == "cets.route")`,
		`select(.key == "cets.booking.stage")`,
		"booking.event_lock",
		`startswith("db.")`,
		`select(.key == "cets.db.statement_class")`,
		`select(.key == "server.address" or .key == "db.namespace")`,
		`select(.key == "server.port")`,
		`any(length > 0)`,
		`any(. > 0)`,
		"Tempo did not return cets-backend traces with route, booking-stage, and DB span evidence",
		"TEMPO_ERROR_TRACE_ID",
		"TEMPO_ERROR_TRACE_EVIDENCE=",
		"LOKI_ERROR_TRACE_LOG_EVIDENCE=",
		"CONTROLLED_ERROR_TRACE_ID=",
		"CONTROLLED_ERROR_SPAN_ID=",
		`traceparent: 00-$CONTROLLED_ERROR_TRACE_ID-$CONTROLLED_ERROR_SPAN_ID-01`,
		`-d '{"profile_id":"unknown"}'`,
		"$EDGE_URL/api/v1/auth/mock-provider-token",
		"tempo_trace_has_backend_error",
		`any(. == "/api/v1/auth/mock-provider-token")`,
		`select(.key == "http.response.status_code" or .key == "http.status_code")`,
		`any(. >= 400 and . < 600)`,
		"Tempo did not return cets-backend controlled-error traces with route and 4xx/5xx evidence",
		"check_loki_trace_log_correlation \"$TEMPO_ERROR_TRACE_ID\"",
		`write_loki_trace_proof "$TEMPO_TRACE_ID" "$LOKI_TRACE_LOG_EVIDENCE" "backend hot-path"`,
		"write_loki_trace_proof",
		"matched_field",
		"Tempo controlled-error trace ID",
		"Loki controlled-error trace-log evidence",
		"traces_service_graph_request_total",
		`traces_service_graph_request_total{server="cets-backend"}`,
		`traces_service_graph_request_total{client="cets-backend"}`,
		"prometheus-service-graph-backend-dependency-$RUN_ID.json",
		"SERVICE_GRAPH_BACKEND_DEPENDENCY_EVIDENCE=",
		"SERVICE_GRAPH_INBOUND_QUERY=",
		"SERVICE_GRAPH_BACKEND_DEPENDENCY_QUERY=",
		"SERVICE_GRAPH_INBOUND_BASELINE=0",
		"SERVICE_GRAPH_BACKEND_DEPENDENCY_BASELINE=0",
		"capture_service_graph_baseline",
		"cets-backend inbound service graph counter before load",
		"cets-backend outbound dependency service graph counter before load",
		"Prometheus service graph baseline could not be read",
		"Prometheus did not return service graph metrics for cets-backend inbound and outbound dependency edges",
		"pyroscope/render",
		"otel_trace_id",
		"TEMPO_TRACE_ID",
		"$TEMPO_TRACE_ID",
		"cets_http_request_seconds_count",
		"cets_http_request_seconds p95",
		"cets_http_request_seconds p99",
		"histogram_quantile(0.95",
		"histogram_quantile(0.99",
		"cets_http_request_seconds_bucket",
		"count by (route, method, status_class)",
		"count by (instance)",
		"prometheus-backend-cpu",
		"prometheus-backend-memory",
		"prometheus-db-pool-wait",
		"prometheus-db-lock-waits",
		"post-load-correctness",
		"capacity-replica-spread",
		"CETS_PHASE3_CAPACITY_REPLICA_SAMPLES",
		"phase3-correctness-check.awk",
		"phase3-correctness-summary.sql",
		"phase3-capacity-report.sh",
		`. "$ROOT_DIR/scripts/compose/phase3-capacity-report.sh"`,
		"phase3-capacity-k6-report.jq",
		"phase3-prometheus-vector-report.jq",
		"phase3-prometheus-backend-targets-required.jq",
		"PHASE3_BACKEND_PROMETHEUS_TARGETS",
		`rate(process_cpu_seconds_total{job="cets-backend"}[5m])`,
		`go_memstats_heap_alloc_bytes{job="cets-backend"}`,
		`rate(cets_db_pool_acquire_wait_seconds_total{job="cets-backend"}[5m])`,
		`label_replace(max by (job) (cets_db_lock_waiting_sessions{job="cets-backend"}), "instance", "global-postgres", "job", ".*")`,
		"Prometheus backend CPU sample",
		"Prometheus backend memory sample",
		"Prometheus DB pool wait sample",
		"Prometheus global DB lock wait sample",
		"Post-load correctness summary",
		"Post-Load Correctness",
		"Replica spread summary",
		"Replica Spread",
		"matching_events",
		"event_title",
		"confirmed_missing_ticket",
		"duplicate_active_registrations",
		"duplicate_checkins",
		"booking.confirmed audit count does not match confirmed registrations",
		"ticket.issued audit count does not match tickets",
		"confirmed registrations missing tickets",
		"confirmed registrations have inactive tickets",
		"Backend CPU Samples",
		"Backend Memory Samples",
		"DB Pool Wait Samples",
		"Global DB Lock Wait Samples",
		"Bottleneck",
		"Optimization result",
		"HTTP throughput req/s",
		"unexpected error rate",
		"http_req_duration p50",
		"http_req_duration p99",
		"booking conflicts",
		"phase3-redaction-canary",
		`service_name%3D%22redaction-canary%22`,
		"write_loki_redaction_proof",
		"raw_secret_absent",
		"CETS_PHASE3_VERIFY_ARTIFACT_DIR",
		"CETS_PHASE3_VERIFY_RUN_ID",
		`chmod 0700 "$ARTIFACT_DIR"`,
		"K6_ARTIFACT_DIR=",
		"K6_SUMMARY_EVIDENCE=",
		"K6_REPLICA_SPREAD_EVIDENCE=",
		"CETS_PHASE3_K6_ARTIFACT_DIR",
		"CETS_PHASE3_K6_SUMMARY",
		"CETS_PHASE3_K6_REPLICA_SPREAD",
		"k6 summary evidence",
		"k6 replica-spread evidence",
		"phase3-verify-report-$RUN_ID.md",
		"phase3-verify-report.sh",
		`. "$ROOT_DIR/scripts/compose/phase3-verify-report.sh"`,
		"phase3-verify-trace-evidence.sh",
		`. "$ROOT_DIR/scripts/compose/phase3-verify-trace-evidence.sh"`,
		"prometheus-targets-$RUN_ID.txt",
		"prometheus-red-$RUN_ID.txt",
		"prometheus-controlled-error-$RUN_ID.txt",
		"PROM_CONTROLLED_ERROR_EVIDENCE=",
		"PROM_CONTROLLED_ERROR_QUERY=",
		"PROM_CONTROLLED_ERROR_BASELINE=0",
		"capture_prometheus_controlled_error_baseline",
		"check_prometheus_controlled_error_metrics",
		`cets_http_requests_total{route="/api/v1/auth/mock-provider-token",method="POST",status_class="4xx"}`,
		"controlled-error cets_http_requests_total before verifier-owned request",
		"controlled-error cets_http_requests_total after verifier-owned request",
		"Prometheus controlled-error RED baseline could not be read",
		"Prometheus controlled-error RED counter did not increase for POST /api/v1/auth/mock-provider-token 4xx",
		"tempo-search-$RUN_ID.json",
		"tempo-trace-$RUN_ID.json",
		"prometheus-service-graph-$RUN_ID.json",
		"pyroscope-profile-$RUN_ID.json",
		"loki-trace-logs-$RUN_ID.json",
		"loki-redaction-$RUN_ID.json",
		"write_verify_report",
		"wrote Phase 3 verify report",
		"Prometheus RED evidence",
		"Prometheus controlled-error RED evidence",
		"Tempo trace evidence",
		"Service graph evidence",
		"Service graph backend dependency evidence",
		"Pyroscope profile evidence",
		"Loki trace-log evidence",
		"Loki redaction evidence",
		"DRILL_SERVICES=(gateway-1 frontend-1 backend-1)",
		"CETS_PHASE3_DRILL_ARTIFACT_DIR",
		"CETS_PHASE3_DRILL_RUN_ID",
		"phase3-drill-events-$RUN_ID.txt",
		"phase3-drill-report-$RUN_ID.md",
		`chmod 0700 "$ARTIFACT_DIR"`,
		"CURRENT_SERVICE=",
		"SERVICE_STOPPED=false",
		"DRILL_STATUS=failed",
		"REPORT_WRITTEN=false",
		"record_event",
		"cleanup_stopped_service",
		"cleanup_restart_attempted",
		"cleanup_restart_started",
		"cleanup_restart_failed",
		"trap finalize EXIT",
		"smoke_while_stopped",
		"healthy_after_restart",
		"smoke_after_restart",
		`write_report "$DRILL_STATUS"`,
		"Phase 3 Recovery Drill Report",
		"Status",
		"Event evidence",
		"compose stop \"$service\"",
		"compose ps -a -q \"$service\"",
		"docker start \"$container\"",
	} {
		assert.Contains(t, scripts, fragment, "Phase 3 Compose script contract is missing %q", fragment)
	}

	for _, forbidden := range []string{
		"kubectl",
		"scripts/k3s",
		"deploy/k8s-phase3",
		"observability/k8s-lgtm",
		"rm -rf /etc/rancher",
		"k3s-uninstall.sh",
		"docker system prune",
	} {
		assert.NotContains(t, scripts, forbidden, "Phase 3 Compose scripts must not require %q", forbidden)
	}
}

func TestPhase3RuntimeScriptsCheckDockerDaemonBeforeRuntimeWork(t *testing.T) {
	scriptDir := filepath.Join("..", "..", "..", "scripts", "compose")
	firstRuntimeCallByScript := map[string]string{
		"phase3-deploy.sh":   "compose build",
		"phase3-k6.sh":       "run_k6",
		"phase3-verify.sh":   "check_replicas",
		"phase3-drill.sh":    "drill_service",
		"phase3-capacity.sh": "curl -fsS \"$BASE_URL/readyz\"",
	}
	for name, runtimeCall := range firstRuntimeCallByScript {
		script := readText(t, filepath.Join(scriptDir, name))
		messageSource := script
		if name == "phase3-capacity.sh" {
			messageSource += readText(t, filepath.Join(scriptDir, "phase3-capacity-preflight.sh"))
		}
		require.Contains(t, messageSource, "docker daemon access is required")
		require.Contains(t, script, "require_docker_daemon")

		mainIndex := strings.Index(script, "main() {")
		require.NotEqual(t, -1, mainIndex, "%s must define main", name)
		mainBody := script[mainIndex:]
		preflightIndex := strings.Index(mainBody, "require_docker_daemon\n")
		require.NotEqual(t, -1, preflightIndex, "%s must call require_docker_daemon in main", name)
		callIndex := strings.Index(mainBody, runtimeCall)
		require.NotEqual(t, -1, callIndex, "%s must keep expected runtime entrypoint %s", name, runtimeCall)
		assert.Less(t, preflightIndex, callIndex, "%s must check daemon access before %s", name, runtimeCall)
	}
}

func TestPhase3TempoTraceIDFilterReturnsSearchResultsInOrder(t *testing.T) {
	requireCommand(t, "jq")
	filterPath := filepath.Join("..", "..", "..", "scripts", "compose", "phase3-tempo-trace-ids.jq")
	search := strings.Join([]string{
		`{"traces":[`,
		`{"traceID":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},`,
		`{"traceID":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},`,
		`{"traceID":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`,
		`]}`,
	}, "")
	cmd := exec.Command("jq", "-r", "-f", filterPath)
	cmd.Stdin = strings.NewReader(search)
	output, err := cmd.Output()
	require.NoError(t, err)

	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n", string(output))
}
