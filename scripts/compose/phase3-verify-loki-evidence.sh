#!/usr/bin/env bash

check_loki_logs_and_redaction() {
  log "checking Loki trace logs and redaction"
  [ -n "$TEMPO_TRACE_ID" ] || die "Tempo trace ID is required before Loki trace-log correlation"
  [ -n "$TEMPO_ERROR_TRACE_ID" ] || die "Tempo controlled-error trace ID is required before Loki trace-log correlation"
  check_loki_hot_path_trace_log_correlation
  check_loki_trace_log_correlation "$TEMPO_ERROR_TRACE_ID" "$LOKI_ERROR_TRACE_LOG_EVIDENCE" "controlled-error"

  compose rm -sf redaction-canary >/dev/null 2>&1 || true
  docker rm -f cets-phase3-redaction-canary >/dev/null 2>&1 || true
  compose up -d --force-recreate redaction-canary >/dev/null
  for _ in $(seq 1 24); do
    redacted_logs=$(curl -fsS "$LOKI_URL/loki/api/v1/query_range?query=%7Bservice_name%3D%22redaction-canary%22%7D%20%7C%3D%20%22phase3-redaction-canary%22&limit=5" 2>/dev/null || true)
    if printf '%s\n' "$redacted_logs" | grep -q "phase3-redaction-canary"; then
      printf '%s\n' "$redacted_logs" | grep -Eq "phase3-raw-(pii|signed|qr|provider|email-body|recipient-email|idempotency)-canary" &&
        die "Loki contains a raw redaction canary secret"
      printf '%s\n' "$redacted_logs" | grep -q "\[REDACTED\]" ||
        die "Loki canary log was found but sensitive fields were not redacted"
      write_loki_redaction_proof "$LOKI_REDACTION_EVIDENCE"
      compose rm -sf redaction-canary >/dev/null 2>&1 || true
      return
    fi
    sleep 5
  done
  compose rm -sf redaction-canary >/dev/null 2>&1 || true
  die "Loki did not return the redaction canary log"
}

write_loki_redaction_proof() {
  evidence_file=$1
  jq -n \
    --arg label "redaction-canary" \
    --arg canary "phase3-redaction-canary" \
    --arg marker "[REDACTED]" \
    '{
      label: $label,
      canary: $canary,
      redaction_marker: $marker,
      canary_matched: true,
      redaction_marker_matched: true,
      raw_secret_absent: true
    }' >"$evidence_file"
}
