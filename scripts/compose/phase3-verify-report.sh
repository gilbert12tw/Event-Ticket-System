#!/usr/bin/env bash

write_verify_report() {
  {
    printf '# Phase 3 Verify Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Edge URL | `%s` |\n' "$EDGE_URL"
    printf '| Grafana URL | `%s` |\n' "$GRAFANA_URL"
    printf '| Prometheus URL | `%s` |\n' "$PROMETHEUS_URL"
    printf '| Loki URL | `%s` |\n' "$LOKI_URL"
    printf '| Tempo URL | `%s` |\n' "$TEMPO_URL"
    printf '| Pyroscope URL | `%s` |\n' "$PYROSCOPE_URL"
    printf '| k6 profile | `%s` |\n' "$VERIFY_K6_PROFILE"
    printf '| k6 summary evidence | `%s` |\n' "$K6_SUMMARY_EVIDENCE"
    printf '| k6 replica-spread evidence | `%s` |\n' "$K6_REPLICA_SPREAD_EVIDENCE"
    printf '| Tempo trace ID | `%s` |\n' "$TEMPO_TRACE_ID"
    printf '| Tempo controlled-error trace ID | `%s` |\n' "$TEMPO_ERROR_TRACE_ID"
    printf '| Prometheus targets evidence | `%s` |\n' "$PROM_TARGETS_EVIDENCE"
    printf '| Prometheus RED evidence | `%s` |\n' "$PROM_RED_EVIDENCE"
    printf '| Prometheus controlled-error RED evidence | `%s` |\n' "$PROM_CONTROLLED_ERROR_EVIDENCE"
    printf '| Tempo search evidence | `%s` |\n' "$TEMPO_SEARCH_EVIDENCE"
    printf '| Tempo trace evidence | `%s` |\n' "$TEMPO_TRACE_EVIDENCE"
    printf '| Tempo controlled-error trace evidence | `%s` |\n' "$TEMPO_ERROR_TRACE_EVIDENCE"
    printf '| Service graph evidence | `%s` |\n' "$SERVICE_GRAPH_EVIDENCE"
    printf '| Service graph backend dependency evidence | `%s` |\n' "$SERVICE_GRAPH_BACKEND_DEPENDENCY_EVIDENCE"
    printf '| Pyroscope profile evidence | `%s` |\n' "$PYROSCOPE_PROFILE_EVIDENCE"
    printf '| Loki trace-log evidence | `%s` |\n' "$LOKI_TRACE_LOG_EVIDENCE"
    printf '| Loki controlled-error trace-log evidence | `%s` |\n' "$LOKI_ERROR_TRACE_LOG_EVIDENCE"
    printf '| Loki redaction evidence | `%s` |\n' "$LOKI_REDACTION_EVIDENCE"
  } >"$VERIFY_REPORT"
  log "wrote Phase 3 verify report: $VERIFY_REPORT"
}
