#!/usr/bin/env bash

write_report() {
  best_rps=$1
  summary="$ARTIFACT_DIR/k6-$RUN_ID-rps-$best_rps.json"
  report="$ARTIFACT_DIR/capacity-report-$RUN_ID.md"
  correctness="$ARTIFACT_DIR/post-load-correctness-$RUN_ID.txt"
  replica_spread="$ARTIFACT_DIR/capacity-replica-spread-$RUN_ID-rps-$best_rps.txt"
  prom_red="$ARTIFACT_DIR/prometheus-red-$RUN_ID.json"
  prom_booking="$ARTIFACT_DIR/prometheus-booking-stages-$RUN_ID.json"
  prom_reservation="$ARTIFACT_DIR/prometheus-reservation-$RUN_ID.json"
  prom_cpu="$ARTIFACT_DIR/prometheus-backend-cpu-$RUN_ID.json"
  prom_memory="$ARTIFACT_DIR/prometheus-backend-memory-$RUN_ID.json"
  prom_db_pool="$ARTIFACT_DIR/prometheus-db-pool-wait-$RUN_ID.json"
  prom_db_locks="$ARTIFACT_DIR/prometheus-db-lock-waits-$RUN_ID.json"

  validate_capacity_verify_report

  write_correctness_summary "$best_rps" "$correctness"
  require_post_load_correctness "$correctness"
  [ -s "$replica_spread" ] || write_replica_spread "$best_rps"
  require_replica_spread_summary "$replica_spread"
  require_prometheus_evidence
  prom_query 'sum by (route,method,status_class,instance) (rate(cets_http_requests_total[5m]))' "$prom_red" || true
  prom_query 'histogram_quantile(0.95, sum by (stage,outcome,le) (rate(cets_booking_stage_seconds_bucket[5m])))' "$prom_booking" || true
  prom_query 'sum by (outcome,capacity_type,outage_mode) (increase(cets_reservation_attempt_total[15m]))' "$prom_reservation" || true
  prom_query 'sum by (instance) (rate(process_cpu_seconds_total{job="cets-backend"}[5m]))' "$prom_cpu"
  require_prometheus_vector_sample "$prom_cpu" "CPU"
  prom_query 'sum by (instance) (go_memstats_heap_alloc_bytes{job="cets-backend"})' "$prom_memory"
  require_prometheus_vector_sample "$prom_memory" "memory"
  prom_query 'sum by (instance) (rate(cets_db_pool_acquire_wait_seconds_total{job="cets-backend"}[5m]))' "$prom_db_pool"
  require_prometheus_vector_sample "$prom_db_pool" "DB pool wait"
  prom_query 'label_replace(max by (job) (cets_db_lock_waiting_sessions{job="cets-backend"}), "instance", "global-postgres", "job", ".*")' "$prom_db_locks"
  require_prometheus_vector_any_sample "$prom_db_locks" "global DB lock wait"

  {
    printf '# Phase3 Capacity Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Base URL | `%s` |\n' "$BASE_URL"
    printf '| Highest passing RPS | `%s` |\n' "$best_rps"
    printf '| RPS search start | `%s` |\n' "$START_RPS"
    printf '| RPS search step | `%s` |\n' "$STEP_RPS"
    printf '| RPS search max | `%s` |\n' "$MAX_RPS"
    printf '| RPS search resolution | `%s` |\n' "$RESOLUTION_RPS"
    printf '| Duration | `%s` |\n' "$DURATION"
    printf '| Traffic mix | `%s read / %s booking` |\n' "$READ_RATIO" "$(awk -v r="$READ_RATIO" 'BEGIN { printf "%.2f", 1-r }')"
    printf '| Employee fixture | `%s` employees, prefix `%s` |\n' "$EMPLOYEE_COUNT" "$EMPLOYEE_PREFIX"
    printf '| Hot event capacity | `%s` |\n' "$HOT_EVENT_CAPACITY"
    printf '| k6 summary | `%s` |\n' "$summary"
    printf '| Replica spread summary | `%s` |\n' "$replica_spread"
    printf '| Post-load correctness summary | `%s` |\n' "$correctness"
    printf '| Prometheus RED sample | `%s` |\n' "$prom_red"
    printf '| Prometheus booking stage sample | `%s` |\n' "$prom_booking"
    printf '| Prometheus reservation sample | `%s` |\n' "$prom_reservation"
    printf '| Prometheus backend CPU sample | `%s` |\n' "$prom_cpu"
    printf '| Prometheus backend memory sample | `%s` |\n' "$prom_memory"
    printf '| Prometheus DB pool wait sample | `%s` |\n' "$prom_db_pool"
    printf '| Prometheus global DB lock wait sample | `%s` |\n' "$prom_db_locks"
    if [ -n "$CAPACITY_VERIFY_REPORT" ]; then
      printf '| LGTM verify report | `%s` |\n' "$CAPACITY_VERIFY_REPORT"
    else
      printf '| LGTM verify report | `not linked` |\n'
    fi
    printf '| Bottleneck | `%s` |\n' "$BOTTLENECK_NOTE"
    printf '| Optimization result | `%s` |\n' "$OPTIMIZATION_RESULT"
    if [ -f "$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt" ]; then
      printf '| Highest failing RPS tested | `%s` |\n' "$(cat "$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt")"
    fi
    printf '\n## k6 Metrics\n\n'
    jq -r --arg target_rps "$best_rps" -f "$K6_REPORT_FILTER" "$summary"
    printf '\n## Post-Load Correctness\n\n'
    print_correctness_summary "$correctness"
    printf '\n## Replica Spread\n\n'
    print_replica_spread "$replica_spread"
    printf '\n## Backend CPU Samples\n\n'
    jq -r -f "$PROM_VECTOR_REPORT_FILTER" "$prom_cpu"
    printf '\n## Backend Memory Samples\n\n'
    jq -r -f "$PROM_VECTOR_REPORT_FILTER" "$prom_memory"
    printf '\n## DB Pool Wait Samples\n\n'
    jq -r -f "$PROM_VECTOR_REPORT_FILTER" "$prom_db_pool"
    printf '\n## Global DB Lock Wait Samples\n\n'
    jq -r -f "$PROM_VECTOR_REPORT_FILTER" "$prom_db_locks"
  } >"$report"
  log "wrote capacity report: $report"
}
