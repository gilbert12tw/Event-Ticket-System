prom_query() {
  query=$1
  output=$2
  curl -fsS --get --data-urlencode "query=$query" "$PROMETHEUS_URL/api/v1/query" >"$output"
}

require_prometheus_vector_sample() {
  file=$1
  label=$2
  jq -e --arg expected_targets "$PHASE3_BACKEND_PROMETHEUS_TARGETS" -f "$PROM_VECTOR_TARGET_FILTER" "$file" >/dev/null ||
    die "Prometheus did not return backend $label evidence for every Phase 3 backend target: $PHASE3_BACKEND_PROMETHEUS_TARGETS"
}

require_prometheus_vector_any_sample() {
  file=$1
  label=$2
  jq -e '.data.result | length > 0' "$file" >/dev/null ||
    die "Prometheus did not return $label evidence"
}

prom_scalar() {
  query=$1
  curl -fsS --get --data-urlencode "query=$query" "$PROMETHEUS_URL/api/v1/query" |
    jq -r 'if .data.resultType == "scalar" then .data.result[1] else (.data.result[0].value[1] // empty) end'
}

require_prometheus_evidence() {
  replicas=$(prom_scalar 'scalar(count(count by (instance) (increase(cets_http_requests_total[15m]) > 0)))')
  booking_samples=$(prom_scalar 'sum(increase(cets_booking_stage_seconds_count[15m]))')
  awk -v value="${replicas:-0}" 'BEGIN { exit(value >= 3 ? 0 : 1) }' ||
    die "benchmark did not produce traffic on all 3 backend instances; observed $replicas"
  awk -v value="${booking_samples:-0}" 'BEGIN { exit(value > 0 ? 0 : 1) }' ||
    die "benchmark did not produce booking stage metrics in Prometheus"
}
