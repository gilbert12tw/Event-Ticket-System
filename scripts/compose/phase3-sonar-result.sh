#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RUN_ID=${CETS_PHASE3_QUALITY_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
ARTIFACT_DIR=${CETS_PHASE3_QUALITY_ARTIFACT_DIR:-$ROOT_DIR/artifacts/phase3-quality}
SONAR_RESULT_REPORT=${CETS_PHASE3_SONAR_RESULT_REPORT:-$ARTIFACT_DIR/sonar-result-$RUN_ID.md}
SONAR_TASK_FILE=${CETS_PHASE3_SONAR_TASK_FILE:-$ROOT_DIR/.scannerwork/report-task.txt}
SONAR_PROJECT_KEY=${CETS_PHASE3_SONAR_PROJECT_KEY:-event-ticket-system}
SONAR_WAIT_ATTEMPTS=${CETS_PHASE3_SONAR_WAIT_ATTEMPTS:-60}
SONAR_WAIT_SECONDS=${CETS_PHASE3_SONAR_WAIT_SECONDS:-2}

log() {
  printf '[phase3-sonar-result] %s\n' "$*"
}

die() {
  printf '[phase3-sonar-result] error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: scripts/compose/phase3-sonar-result.sh

Runs Phase 3 coverage plus sonar-scanner, waits for Sonar background-task completion, and writes
the markdown result evidence consumed by scripts/compose/phase3-quality.sh.

Important environment variables:
  SONAR_HOST_URL                         Sonar server URL.
  SONAR_TOKEN                            Sonar API/scanner token.
  CETS_PHASE3_SONAR_RESULT_REPORT        Output markdown result report path.
  CETS_PHASE3_SONAR_PROJECT_KEY          Sonar project key, default event-ticket-system.
  CETS_PHASE3_SONAR_TASK_FILE            Scanner task file path, default .scannerwork/report-task.txt.
  CETS_PHASE3_SONAR_WAIT_ATTEMPTS        Background-task poll attempts, default 60.
  CETS_PHASE3_SONAR_WAIT_SECONDS         Seconds between polls, default 2.
EOF
}

require_environment() {
  command -v pnpm >/dev/null 2>&1 || die "pnpm is required"
  command -v sonar-scanner >/dev/null 2>&1 || die "sonar-scanner is required"
  command -v curl >/dev/null 2>&1 || die "curl is required"
  command -v jq >/dev/null 2>&1 || die "jq is required"
  [ -n "${SONAR_HOST_URL:-}" ] || die "SONAR_HOST_URL is required"
  [ -n "${SONAR_TOKEN:-}" ] || die "SONAR_TOKEN is required"
}

task_file_field() {
  field=$1
  awk -v field="$field" '
    index($0, field "=") == 1 {
      value = $0
      sub("^[^=]+=", "", value)
      print value
      exit
    }
  ' "$SONAR_TASK_FILE"
}

sonar_get() {
  path=$1
  shift
  curl -fsS \
    --header "Authorization: Bearer $SONAR_TOKEN" \
    --get \
    "$SONAR_HOST_URL$path" \
    "$@"
}

read_scanner_task() {
  [ -s "$SONAR_TASK_FILE" ] || die "scanner task file does not exist or is empty: $SONAR_TASK_FILE"
  ce_task_id=$(task_file_field "ceTaskId")
  ce_task_url=$(task_file_field "ceTaskUrl")
  project_key=$(task_file_field "projectKey")
  [ -n "$ce_task_id" ] || die "scanner task file is missing ceTaskId"
  [ -n "$project_key" ] || project_key="$SONAR_PROJECT_KEY"
}

poll_background_task() {
  attempts=$SONAR_WAIT_ATTEMPTS
  while [ "$attempts" -gt 0 ]; do
    if [ -n "$ce_task_url" ]; then
      task_json=$(curl -fsS --header "Authorization: Bearer $SONAR_TOKEN" "$ce_task_url")
    else
      task_json=$(sonar_get "/api/ce/task" --data-urlencode "id=$ce_task_id")
    fi

    task_status=$(printf '%s' "$task_json" | jq -r '.task.status // empty')
    case "$task_status" in
      SUCCESS)
        analysis_id=$(printf '%s' "$task_json" | jq -r '.task.analysisId // empty')
        [ -n "$analysis_id" ] || die "Sonar background task succeeded without analysisId"
        return
        ;;
      FAILED | CANCELED)
        die "Sonar background task ended with status: $task_status"
        ;;
      PENDING | IN_PROGRESS)
        ;;
      *)
        die "Sonar background task returned unknown status: ${task_status:-missing}"
        ;;
    esac

    attempts=$((attempts - 1))
    sleep "$SONAR_WAIT_SECONDS"
  done

  die "Sonar background task did not finish after $SONAR_WAIT_ATTEMPTS attempts"
}

load_quality_gate() {
  quality_json=$(sonar_get "/api/qualitygates/project_status" --data-urlencode "analysisId=$analysis_id")
  quality_gate_status=$(printf '%s' "$quality_json" | jq -r '.projectStatus.status // empty')
  [ -n "$quality_gate_status" ] || die "Sonar quality gate response is missing projectStatus.status"
}

issue_total() {
  sonar_get "/api/issues/search" "$@" --data-urlencode "ps=1" |
    jq -r '.total // empty'
}

hotspot_total() {
  sonar_get "/api/hotspots/search" "$@" --data-urlencode "ps=1" |
    jq -r '.paging.total // .total // empty'
}

require_numeric_total() {
  label=$1
  value=$2
  printf '%s' "$value" | awk 'BEGIN { ok = 0 } /^[0-9]+$/ { ok = 1 } END { exit(ok ? 0 : 1) }' ||
    die "Sonar $label total is not numeric: ${value:-missing}"
}

load_issue_totals() {
  issues=$(issue_total --data-urlencode "componentKeys=$project_key" --data-urlencode "resolved=false")
  problems=$(
    issue_total \
      --data-urlencode "componentKeys=$project_key" \
      --data-urlencode "resolved=false" \
      --data-urlencode "types=BUG,CODE_SMELL"
  )
  vulnerabilities=$(
    issue_total \
      --data-urlencode "componentKeys=$project_key" \
      --data-urlencode "resolved=false" \
      --data-urlencode "types=VULNERABILITY"
  )
  hotspots=$(hotspot_total --data-urlencode "projectKey=$project_key" --data-urlencode "status=TO_REVIEW")
  require_numeric_total "issues" "$issues"
  require_numeric_total "problems" "$problems"
  require_numeric_total "vulnerabilities" "$vulnerabilities"
  require_numeric_total "security hotspots" "$hotspots"
  security_problems=$((vulnerabilities + hotspots))
}

write_result_report() {
  mkdir -p "$(dirname "$SONAR_RESULT_REPORT")"
  {
    printf '# Sonar Result Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Quality Gate Status | `%s` |\n' "$quality_gate_status"
    printf '| Issues | `%s` |\n' "$issues"
    printf '| Problems | `%s` |\n' "$problems"
    printf '| Security problems | `%s` |\n' "$security_problems"
  } >"$SONAR_RESULT_REPORT"
  chmod 0600 "$SONAR_RESULT_REPORT"
}

enforce_passed_result() {
  normalized_status=$(printf '%s' "$quality_gate_status" | tr '[:upper:]' '[:lower:]')
  [ "$normalized_status" = "ok" ] || [ "$normalized_status" = "passed" ] ||
    die "Sonar quality gate must be passed, got: $quality_gate_status"
  [ "$issues" = "0" ] || die "Sonar issues must be 0, got: $issues"
  [ "$problems" = "0" ] || die "Sonar problems must be 0, got: $problems"
  [ "$security_problems" = "0" ] || die "Sonar security problems must be 0, got: $security_problems"
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

  require_environment
  cd "$ROOT_DIR"
  log "running coverage and Sonar scanner"
  pnpm test:coverage
  sonar-scanner
  read_scanner_task
  poll_background_task
  load_quality_gate
  load_issue_totals
  write_result_report
  enforce_passed_result
  log "wrote Sonar result report: $SONAR_RESULT_REPORT"
}

main "$@"
