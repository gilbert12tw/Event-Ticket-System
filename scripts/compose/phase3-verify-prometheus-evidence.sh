prom_query() {
  curl -fsS --get --data-urlencode "query=$1" "$PROMETHEUS_URL/api/v1/query"
}

json_scalar_value() {
  sed -n \
    -e 's/.*"value":\[[^]]*,"\([0-9.][0-9.]*\)".*/\1/p' \
    -e 's/.*"result":\[[^]]*,"\([0-9.][0-9.]*\)".*/\1/p' |
    tail -n 1
}

prom_query_nonzero() {
  query=$1
  response=$(prom_query "$query" 2>/dev/null || true)
  printf '%s\n' "$response" | grep -Eq '"value":\[[^]]+,"[0-9.]*[1-9][0-9.]*"\]'
}

prom_query_at_least() {
  query=$1
  minimum=$2
  response=$(prom_query "$query" 2>/dev/null || true)
  value=$(printf '%s\n' "$response" | json_scalar_value)
  [ -n "$value" ] || return 1
  awk -v value="$value" -v minimum="$minimum" 'BEGIN { exit(value >= minimum ? 0 : 1) }'
}

check_prometheus_targets() {
  log "checking Prometheus backend targets"
  for target in backend-1:8080 backend-2:8080 backend-3:8080; do
    prom_query_at_least "up{job=\"cets-backend\",instance=\"$target\"}" 1 ||
      die "Prometheus cets-backend target $target is not scraping up"
  done
  prom_query 'up{job="cets-backend"}' >"$PROM_TARGETS_EVIDENCE"
}

check_prometheus_red_metrics() {
  log "checking Prometheus RED metrics from k6 load"
  for _ in $(seq 1 24); do
    if prom_query_nonzero 'sum(increase(cets_http_requests_total[15m]))' &&
      prom_query_nonzero 'sum(increase(cets_http_requests_total{status_class=~"4xx|5xx"}[15m]))' &&
      prom_query_nonzero 'sum(increase(cets_http_request_seconds_count[15m]))' &&
      prom_query_nonzero 'sum(increase(cets_booking_stage_seconds_count[15m]))' &&
      prom_query_nonzero 'sum(increase(cets_reservation_attempt_total[15m]))' &&
      prom_query_at_least 'scalar(count(count by (route, method, status_class) (increase(cets_http_requests_total[15m]) > 0)))' 3 &&
      prom_query_at_least 'scalar(count(count by (instance) (increase(cets_http_requests_total[15m]) > 0)))' 3; then
      {
        printf '# cets_http_requests_total by route/method/status/instance\n'
        prom_query 'sum by (route,method,status_class,instance) (increase(cets_http_requests_total[15m]))'
        printf '\n# cets_http_request_seconds_count by route/method/status/instance\n'
        prom_query 'sum by (route,method,status_class,instance) (increase(cets_http_request_seconds_count[15m]))'
        printf '\n# cets_http_request_seconds p95 by route/method/status/instance\n'
        prom_query 'histogram_quantile(0.95, sum by (route,method,status_class,instance,le) (rate(cets_http_request_seconds_bucket[5m])))'
        printf '\n# cets_http_request_seconds p99 by route/method/status/instance\n'
        prom_query 'histogram_quantile(0.99, sum by (route,method,status_class,instance,le) (rate(cets_http_request_seconds_bucket[5m])))'
        printf '\n# cets_booking_stage_seconds_count by stage/outcome\n'
        prom_query 'sum by (stage,outcome) (increase(cets_booking_stage_seconds_count[15m]))'
        printf '\n# cets_reservation_attempt_total by outcome/capacity_type/outage_mode\n'
        prom_query 'sum by (outcome,capacity_type,outage_mode) (increase(cets_reservation_attempt_total[15m]))'
      } >"$PROM_RED_EVIDENCE"
      return
    fi
    sleep 5
  done
  die "Prometheus did not return complete RED and booking bottleneck evidence by route, status class, latency, backend instance, booking stage, and reservation outcome"
}

check_prometheus_controlled_error_metrics() {
  log "checking Prometheus controlled-error RED metrics"
  for _ in $(seq 1 24); do
    response=$(prom_query "$PROM_CONTROLLED_ERROR_QUERY" 2>/dev/null || true)
    value=$(printf '%s\n' "$response" | json_scalar_value)
    if [ -n "$value" ] &&
      awk -v before="$PROM_CONTROLLED_ERROR_BASELINE" -v after="$value" 'BEGIN { exit(after > before ? 0 : 1) }'; then
      {
        printf '# controlled-error cets_http_requests_total before verifier-owned request\n%s\n' "$PROM_CONTROLLED_ERROR_BASELINE"
        printf '\n# controlled-error cets_http_requests_total after verifier-owned request\n'
        printf '%s\n' "$response"
      } >"$PROM_CONTROLLED_ERROR_EVIDENCE"
      return
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Prometheus controlled-error RED counter did not increase for POST /api/v1/auth/mock-provider-token 4xx"
}

capture_prometheus_controlled_error_baseline() {
  log "capturing Prometheus controlled-error RED baseline"
  for _ in $(seq 1 12); do
    response=$(prom_query "$PROM_CONTROLLED_ERROR_QUERY" 2>/dev/null || true)
    if printf '%s\n' "$response" | grep -q '"status":"success"'; then
      value=$(printf '%s\n' "$response" | json_scalar_value)
      if [ -n "$value" ]; then
        PROM_CONTROLLED_ERROR_BASELINE=$value
      fi
      return
    fi
    sleep 5
  done
  die "Prometheus controlled-error RED baseline could not be read"
}
