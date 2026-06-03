#!/usr/bin/env bash

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

require_docker_daemon() {
  docker info >/dev/null 2>&1 || die "docker daemon access is required"
}

validate_capacity_verify_report() {
  if [ -n "$CAPACITY_VERIFY_REPORT" ] && [ ! -f "$CAPACITY_VERIFY_REPORT" ]; then
    die "Phase 3 verify report does not exist: $CAPACITY_VERIFY_REPORT"
  fi
  if [ -n "$CAPACITY_VERIFY_REPORT" ]; then
    for field in \
      "Tempo trace evidence" \
      "Service graph backend dependency evidence" \
      "Pyroscope profile evidence" \
      "Loki trace-log evidence" \
      "Loki redaction evidence"; do
      grep -q "| $field |" "$CAPACITY_VERIFY_REPORT" ||
        die "Phase 3 verify report is missing required LGTM evidence field: $field"
    done
  fi
}

require_positive_integer() {
  name=$1
  value=$2
  case "$value" in
    '' | *[!0-9]*)
      die "$name must be a positive integer, got: ${value:-empty}"
      ;;
  esac
  [ "$value" -gt 0 ] || die "$name must be a positive integer, got: $value"
}

validate_capacity_inputs() {
  require_positive_integer "CETS_PHASE3_CAPACITY_START_RPS" "$START_RPS"
  require_positive_integer "CETS_PHASE3_CAPACITY_STEP_RPS" "$STEP_RPS"
  require_positive_integer "CETS_PHASE3_CAPACITY_MAX_RPS" "$MAX_RPS"
  require_positive_integer "CETS_PHASE3_CAPACITY_RESOLUTION_RPS" "$RESOLUTION_RPS"
  require_positive_integer "CETS_PHASE3_CAPACITY_EMPLOYEE_COUNT" "$EMPLOYEE_COUNT"
  require_positive_integer "CETS_PHASE3_CAPACITY_HOT_EVENT_CAPACITY" "$HOT_EVENT_CAPACITY"
  require_positive_integer "CETS_PHASE3_CAPACITY_REPLICA_SAMPLES" "$REPLICA_SAMPLES"
  [ "$MAX_RPS" -ge "$START_RPS" ] ||
    die "CETS_PHASE3_CAPACITY_MAX_RPS must be greater than or equal to CETS_PHASE3_CAPACITY_START_RPS"
  awk -v value="$READ_RATIO" 'BEGIN {
    exit(value ~ /^([0-9]+([.][0-9]+)?|[.][0-9]+)$/ && value >= 0 && value <= 1 ? 0 : 1)
  }' ||
    die "CETS_PHASE3_CAPACITY_READ_RATIO must be between 0 and 1, got: $READ_RATIO"
}
