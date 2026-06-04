#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CAPACITY_REPORT=${CETS_PHASE3_CAPACITY_REPORT:-}
ARTIFACT_DIR=${CETS_PHASE3_QUALITY_ARTIFACT_DIR:-$ROOT_DIR/artifacts/phase3-quality}
RUN_ID=${CETS_PHASE3_QUALITY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
QUALITY_REPORT=${CETS_PHASE3_QUALITY_REPORT:-$ARTIFACT_DIR/phase3-quality-report-$RUN_ID.md}
SONAR_RESULT_REPORT=${CETS_PHASE3_SONAR_RESULT_REPORT:-$ARTIFACT_DIR/sonar-result-$RUN_ID.md}
MIN_RPS=${CETS_PHASE3_QUALITY_MIN_RPS:-900}
REQUIRE_OPTIMIZATION=${CETS_PHASE3_QUALITY_REQUIRE_OPTIMIZATION:-true}
SONAR_COMMAND=${CETS_PHASE3_SONAR_COMMAND:-scripts/compose/phase3-sonar-result.sh}

log() {
  printf '[phase3-quality] %s\n' "$*"
}

die() {
  printf '[phase3-quality] error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: CETS_PHASE3_CAPACITY_REPORT=<report.md> scripts/compose/phase3-quality.sh

Validates Phase 3 capacity and LGTM evidence before running the local Sonar scan.
The configured Sonar command must write a markdown result report to
CETS_PHASE3_SONAR_RESULT_REPORT.

Important environment variables:
  CETS_PHASE3_CAPACITY_REPORT                 Capacity report path.
  CETS_PHASE3_QUALITY_ARTIFACT_DIR            Report directory, default artifacts/phase3-quality.
  CETS_PHASE3_QUALITY_REPORT                  Report path override.
  CETS_PHASE3_QUALITY_RUN_ID                  Report run id override.
  CETS_PHASE3_SONAR_RESULT_REPORT             Sonar result evidence report path.
  CETS_PHASE3_QUALITY_MIN_RPS                 Minimum accepted RPS, default 900.
  CETS_PHASE3_QUALITY_REQUIRE_OPTIMIZATION    Require non-default bottleneck notes, default true.
  CETS_PHASE3_SONAR_COMMAND                   Command to run, default phase3-sonar-result.sh.
EOF
}

markdown_field() {
  report=$1
  field=$2
  awk -F'|' -v field="$field" '
    $2 ~ "^[[:space:]]*" field "[[:space:]]*$" {
      value = $3
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^`|`$/, "", value)
      print value
      exit
    }
  ' "$report"
}

report_field() {
  field=$1
  markdown_field "$CAPACITY_REPORT" "$field"
}

require_report_field() {
  field=$1
  value=$(report_field "$field")
  [ -n "$value" ] || die "capacity report is missing required field: $field"
  printf '%s' "$value"
}

require_existing_artifact_field() {
  field=$1
  path=$(require_report_field "$field")
  [ -s "$path" ] || die "capacity report field '$field' points to a missing or empty artifact: $path"
}

sonar_result_field() {
  field=$1
  markdown_field "$SONAR_RESULT_REPORT" "$field"
}

require_sonar_result_field() {
  field=$1
  value=$(sonar_result_field "$field")
  [ -n "$value" ] || die "Sonar result report is missing required field: $field"
  printf '%s' "$value"
}

require_zero_sonar_result_field() {
  field=$1
  value=$(require_sonar_result_field "$field")
  awk -v value="$value" 'BEGIN { exit(value ~ /^[0-9]+$/ && value + 0 == 0 ? 0 : 1) }' ||
    die "Sonar result report field '$field' must be 0, got: $value"
  printf '%s' "$value"
}

require_verify_report_artifact_field() {
  field=$1
  path=$(markdown_field "$verify_report" "$field")
  [ -n "$path" ] || die "linked LGTM verify report is missing required field: $field"
  [ "$path" != "not linked" ] || die "linked LGTM verify report field '$field' is not linked"
  [ -s "$path" ] || die "linked LGTM verify report field '$field' points to a missing or empty artifact: $path"
}

validate_capacity_report() {
  [ -n "$CAPACITY_REPORT" ] || die "CETS_PHASE3_CAPACITY_REPORT is required"
  [ -f "$CAPACITY_REPORT" ] || die "capacity report does not exist: $CAPACITY_REPORT"

  rps=$(require_report_field "Highest passing RPS")
  awk -v value="$rps" -v minimum="$MIN_RPS" 'BEGIN { exit(value + 0 >= minimum + 0 ? 0 : 1) }' ||
    die "highest passing RPS $rps is below required $MIN_RPS"

  require_existing_artifact_field "k6 summary"
  require_existing_artifact_field "Replica spread summary"
  require_existing_artifact_field "Post-load correctness summary"
  require_existing_artifact_field "Prometheus RED sample"
  require_existing_artifact_field "Prometheus backend CPU sample"
  require_existing_artifact_field "Prometheus backend memory sample"
  require_existing_artifact_field "Prometheus DB pool wait sample"
  require_existing_artifact_field "Prometheus global DB lock wait sample"

  verify_report=$(require_report_field "LGTM verify report")
  [ "$verify_report" != "not linked" ] || die "capacity report must link a Phase 3 LGTM verify report"
  [ -f "$verify_report" ] || die "linked LGTM verify report does not exist: $verify_report"
  require_verify_report_artifact_field "Tempo trace evidence"
  require_verify_report_artifact_field "Service graph backend dependency evidence"
  require_verify_report_artifact_field "Pyroscope profile evidence"
  require_verify_report_artifact_field "Loki trace-log evidence"
  require_verify_report_artifact_field "Loki redaction evidence"

  if [ "$REQUIRE_OPTIMIZATION" = "true" ]; then
    bottleneck=$(require_report_field "Bottleneck")
    optimization=$(require_report_field "Optimization result")
    [ "$bottleneck" != "not identified in this run" ] ||
      die "capacity report must record the measured bottleneck before Sonar"
    [ "$optimization" != "not yet optimized" ] ||
      die "capacity report must record the optimization result before Sonar"
  fi
}

validate_sonar_environment() {
  command -v pnpm >/dev/null 2>&1 || die "pnpm is required"
  command -v sonar-scanner >/dev/null 2>&1 || die "sonar-scanner is required"
  [ -n "${SONAR_HOST_URL:-}" ] || die "SONAR_HOST_URL is required"
  [ -n "${SONAR_TOKEN:-}" ] || die "SONAR_TOKEN is required"
}

validate_sonar_result_report() {
  [ -s "$SONAR_RESULT_REPORT" ] || die "Sonar result report does not exist or is empty: $SONAR_RESULT_REPORT"

  sonar_quality_gate_status=$(require_sonar_result_field "Quality Gate Status")
  normalized_status=$(printf '%s' "$sonar_quality_gate_status" | tr '[:upper:]' '[:lower:]')
  case "$normalized_status" in
    passed | ok)
      ;;
    *)
      die "Sonar quality gate must be passed, got: $sonar_quality_gate_status"
      ;;
  esac

  sonar_issues=$(require_zero_sonar_result_field "Issues")
  sonar_problems=$(require_zero_sonar_result_field "Problems")
  sonar_security_problems=$(require_zero_sonar_result_field "Security problems")
}

write_quality_report() {
  mkdir -p "$(dirname "$QUALITY_REPORT")"
  {
    printf '# Phase 3 Quality Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Status | `passed` |\n'
    printf '| Capacity report | `%s` |\n' "$CAPACITY_REPORT"
    printf '| LGTM verify report | `%s` |\n' "$verify_report"
    printf '| Highest passing RPS | `%s` |\n' "$rps"
    printf '| Required minimum RPS | `%s` |\n' "$MIN_RPS"
    printf '| Require optimization evidence | `%s` |\n' "$REQUIRE_OPTIMIZATION"
    printf '| Sonar command | `configured command completed` |\n'
    printf '| Sonar result report | `%s` |\n' "$SONAR_RESULT_REPORT"
    printf '| Sonar Quality Gate Status | `%s` |\n' "$sonar_quality_gate_status"
    printf '| Sonar issues | `%s` |\n' "$sonar_issues"
    printf '| Sonar problems | `%s` |\n' "$sonar_problems"
    printf '| Sonar security problems | `%s` |\n' "$sonar_security_problems"
  } >"$QUALITY_REPORT"
  chmod 0600 "$QUALITY_REPORT"
}

main() {
  case "${1:-}" in
    -h | --help)
      usage
      return
      ;;
    "")
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac

  validate_capacity_report
  validate_sonar_environment
  log "capacity evidence accepted; running configured Sonar command"
  cd "$ROOT_DIR"
  export SONAR_RESULT_REPORT
  export CETS_PHASE3_SONAR_RESULT_REPORT="$SONAR_RESULT_REPORT"
  sh -c "$SONAR_COMMAND"
  validate_sonar_result_report
  write_quality_report
  log "wrote Phase 3 quality report: $QUALITY_REPORT"
}

main "$@"
