#!/usr/bin/env bash

tempo_trace_ids() {
  jq -r -f "$TEMPO_TRACE_IDS_FILTER" | awk '!seen[$0]++'
}

tempo_trace_has_backend_route() {
  jq -e '
    (.batches // .resourceSpans // [])
    | any(.[]?;
        ([.resource.attributes[]? | select(.key == "service.name") | .value.stringValue] | any(. == "cets-backend")) and
        (
          (.scopeSpans // .instrumentationLibrarySpans // [])
          | any(.[]?.spans[]?;
              [.attributes[]? | select(.key == "http.route" or .key == "cets.route")] | length > 0
            )
        ) and
        (
          (.scopeSpans // .instrumentationLibrarySpans // [])
          | any(.[]?.spans[]?;
              (.name // "") == "booking.event_lock" or
              ([.attributes[]? | select(.key == "cets.booking.stage") | .value.stringValue]
                | any(. == "event_lock" or . == "capacity" or . == "create_response"))
            )
        ) and
        (
          (.scopeSpans // .instrumentationLibrarySpans // [])
          | any(.[]?.spans[]?;
              (.name // "" | startswith("db.")) and
              ([.attributes[]? | select(.key == "cets.db.statement_class") | .value.stringValue]
                | any(. == "select" or . == "insert" or . == "update" or . == "with")) and
              (
                ([.attributes[]? |
                  select(.key == "server.address" or .key == "db.namespace") |
                  .value.stringValue] | any(length > 0)) or
                ([.attributes[]? |
                  select(.key == "server.port") |
                  (.value.intValue // .value.stringValue // empty) |
                  tonumber?] | any(. > 0))
              )
            )
        )
      )
  ' >/dev/null
}

tempo_trace_has_backend_error() {
  jq -e '
    def attr_route:
      [.attributes[]? |
        select(.key == "http.route" or .key == "cets.route") |
        .value.stringValue] |
        any(. == "/api/v1/auth/mock-provider-token");
    def attr_status_code:
      [.attributes[]? |
        select(.key == "http.response.status_code" or .key == "http.status_code") |
        (.value.intValue // .value.stringValue // empty) |
        tonumber?];
    (.batches // .resourceSpans // [])
    | any(.[]?;
        ([.resource.attributes[]? | select(.key == "service.name") | .value.stringValue] | any(. == "cets-backend")) and
        (
          (.scopeSpans // .instrumentationLibrarySpans // [])
          | any(.[]?.spans[]?;
              attr_route and
              (attr_status_code | any(. >= 400 and . < 600))
            )
        )
      )
  ' >/dev/null
}

check_trace_ingest() {
  log "checking Tempo trace ingest"
  for _ in $(seq 1 24); do
    traces=$(curl -fsS "$TEMPO_URL/api/search?tags=service.name%3Dcets-backend&limit=20" 2>/dev/null || true)
    for trace_id in $(printf '%s\n' "$traces" | tempo_trace_ids); do
      trace_detail=$(curl -fsS "$TEMPO_URL/api/traces/$trace_id" 2>/dev/null || true)
      if printf '%s\n' "$trace_detail" | tempo_trace_has_backend_route; then
        printf '%s\n' "$traces" >"$TEMPO_SEARCH_EVIDENCE"
        printf '%s\n' "$trace_detail" >"$TEMPO_TRACE_EVIDENCE"
        TEMPO_TRACE_ID=$trace_id
        return
      fi
    done
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Tempo did not return cets-backend traces with route, booking-stage, and DB span evidence"
}

check_error_trace_ingest() {
  log "checking Tempo controlled-error trace ingest"
  response_status=$(curl -sS -o /dev/null -w '%{http_code}' \
    -H "Content-Type: application/json" \
    -H "traceparent: 00-$CONTROLLED_ERROR_TRACE_ID-$CONTROLLED_ERROR_SPAN_ID-01" \
    -d '{"profile_id":"unknown"}' \
    "$EDGE_URL/api/v1/auth/mock-provider-token" 2>/dev/null || true)
  [ "$response_status" = "401" ] ||
    die "controlled-error trace request returned HTTP $response_status instead of 401"

  for _ in $(seq 1 24); do
    trace_detail=$(curl -fsS "$TEMPO_URL/api/traces/$CONTROLLED_ERROR_TRACE_ID" 2>/dev/null || true)
    if printf '%s\n' "$trace_detail" | tempo_trace_has_backend_error; then
      printf '%s\n' "$trace_detail" >"$TEMPO_ERROR_TRACE_EVIDENCE"
      TEMPO_ERROR_TRACE_ID=$CONTROLLED_ERROR_TRACE_ID
      [ "$TEMPO_ERROR_TRACE_ID" != "$TEMPO_TRACE_ID" ] ||
        die "controlled-error trace reused the hot-path Tempo trace ID"
      return
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Tempo did not return cets-backend controlled-error traces with route and 4xx/5xx evidence"
}

check_loki_hot_path_trace_log_correlation() {
  for _ in $(seq 1 12); do
    logs=$(curl -fsS --get \
      --data-urlencode "query={service_name=~\"backend-.*\"} |= \"$TEMPO_TRACE_ID\" |= \"otel_trace_id\"" \
      --data-urlencode "limit=1" \
      "$LOKI_URL/loki/api/v1/query_range" 2>/dev/null || true)
    if printf '%s\n' "$logs" | grep -q "otel_trace_id" &&
      printf '%s\n' "$logs" | grep -q "$TEMPO_TRACE_ID"; then
      write_loki_trace_proof "$TEMPO_TRACE_ID" "$LOKI_TRACE_LOG_EVIDENCE" "backend hot-path"
      return
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Loki did not return backend logs for Tempo trace $TEMPO_TRACE_ID"
}

check_loki_trace_log_correlation() {
  trace_id=$1
  evidence_file=$2
  label=$3
  for _ in $(seq 1 12); do
    logs=$(curl -fsS --get \
      --data-urlencode "query={service_name=~\"backend-.*\"} |= \"$trace_id\" |= \"otel_trace_id\"" \
      --data-urlencode "limit=1" \
      "$LOKI_URL/loki/api/v1/query_range" 2>/dev/null || true)
    if printf '%s\n' "$logs" | grep -q "otel_trace_id" &&
      printf '%s\n' "$logs" | grep -q "$trace_id"; then
      write_loki_trace_proof "$trace_id" "$evidence_file" "$label"
      return
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Loki did not return $label backend logs for Tempo trace $trace_id"
}

write_loki_trace_proof() {
  trace_id=$1
  evidence_file=$2
  label=$3
  jq -n \
    --arg label "$label" \
    --arg trace_id "$trace_id" \
    --arg field "otel_trace_id" \
    '{label: $label, trace_id: $trace_id, matched_field: $field, matched: true}' >"$evidence_file"
}
