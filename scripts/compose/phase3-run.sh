#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT_DIR=${CETS_PHASE3_RUN_SCRIPT_DIR:-$ROOT_DIR/scripts/compose}
RUN_ID=${CETS_PHASE3_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
ARTIFACT_ROOT=${CETS_PHASE3_RUN_ARTIFACT_DIR:-$ROOT_DIR/artifacts/phase3-run/$RUN_ID}
RUN_REPORT=${CETS_PHASE3_RUN_REPORT:-$ARTIFACT_ROOT/phase3-run-report-$RUN_ID.md}
RUN_LOG=${CETS_PHASE3_RUN_LOG:-$ARTIFACT_ROOT/phase3-run-steps-$RUN_ID.log}
VERIFY_ARTIFACT_DIR=${CETS_PHASE3_VERIFY_ARTIFACT_DIR:-$ARTIFACT_ROOT/verify}
CAPACITY_ARTIFACT_DIR=${CETS_PHASE3_CAPACITY_ARTIFACT_DIR:-$ARTIFACT_ROOT/capacity}
DRILL_ARTIFACT_DIR=${CETS_PHASE3_DRILL_ARTIFACT_DIR:-$ARTIFACT_ROOT/drill}
QUALITY_ARTIFACT_DIR=${CETS_PHASE3_QUALITY_ARTIFACT_DIR:-$ARTIFACT_ROOT/quality}
VERIFY_REPORT="$VERIFY_ARTIFACT_DIR/phase3-verify-report-$RUN_ID.md"
CAPACITY_REPORT="$CAPACITY_ARTIFACT_DIR/capacity-report-$RUN_ID.md"
DRILL_REPORT="$DRILL_ARTIFACT_DIR/phase3-drill-report-$RUN_ID.md"
QUALITY_REPORT="$QUALITY_ARTIFACT_DIR/phase3-quality-report-$RUN_ID.md"
SONAR_RESULT_REPORT="$QUALITY_ARTIFACT_DIR/sonar-result-$RUN_ID.md"
INCLUDE_DRILL=${CETS_PHASE3_RUN_INCLUDE_DRILL:-true}
CURRENT_STEP=not-started
RUN_STATUS=running
REPORT_WRITTEN=false

log() {
  printf '[phase3-run] %s\n' "$*"
}

die() {
  printf '[phase3-run] error: %s\n' "$*" >&2
  if [ -f "$RUN_LOG" ]; then
    printf '[phase3-run] error: %s\n' "$*" >>"$RUN_LOG" || true
  fi
  exit 1
}

usage() {
  cat <<'EOF'
Usage: scripts/compose/phase3-run.sh

Runs the full Phase 3 local HA gate: deploy, LGTM verification, optional recovery drill, capacity,
and post-capacity quality/Sonar evidence.

Important environment variables:
  CETS_PHASE3_RUN_ID              Shared run id for all Phase 3 gate artifacts.
  CETS_PHASE3_RUN_ARTIFACT_DIR    Artifact root, default artifacts/phase3-run/$RUN_ID.
  CETS_PHASE3_RUN_REPORT          Top-level run report path.
  CETS_PHASE3_RUN_LOG             Step output log path.
  CETS_PHASE3_RUN_INCLUDE_DRILL   Run recovery drill before capacity, default true.
  CETS_PHASE3_RUN_SCRIPT_DIR      Override Phase 3 script directory for contract tests.
EOF
}

require_script() {
  script=$1
  [ -x "$SCRIPT_DIR/$script" ] || die "required Phase 3 script is missing or not executable: $SCRIPT_DIR/$script"
}

require_report() {
  label=$1
  path=$2
  [ -s "$path" ] || die "$label report was not written or is empty: $path"
}

prepare_artifacts() {
  mkdir -p "$ARTIFACT_ROOT" "$VERIFY_ARTIFACT_DIR" "$CAPACITY_ARTIFACT_DIR" "$DRILL_ARTIFACT_DIR" "$QUALITY_ARTIFACT_DIR" "$(dirname "$RUN_REPORT")" "$(dirname "$RUN_LOG")"
  chmod 0700 "$ARTIFACT_ROOT" "$VERIFY_ARTIFACT_DIR" "$CAPACITY_ARTIFACT_DIR" "$DRILL_ARTIFACT_DIR" "$QUALITY_ARTIFACT_DIR"
  : >"$RUN_LOG"
  chmod 0600 "$RUN_LOG"
}

run_step() {
  label=$1
  shift
  CURRENT_STEP=$label
  log "starting $label"
  set +e
  "$@" 2>&1 | tee -a "$RUN_LOG"
  statuses=("${PIPESTATUS[@]}")
  step_status=${statuses[0]}
  tee_status=${statuses[1]}
  set -e
  [ "$step_status" -eq 0 ] || return "$step_status"
  [ "$tee_status" -eq 0 ] || return "$tee_status"
  log "completed $label"
  CURRENT_STEP=between-steps
}

run_deploy() {
  run_step "deploy" "$SCRIPT_DIR/phase3-deploy.sh"
}

run_verify() {
  CETS_PHASE3_VERIFY_RUN_ID="$RUN_ID" \
    CETS_PHASE3_VERIFY_ARTIFACT_DIR="$VERIFY_ARTIFACT_DIR" \
    run_step "LGTM verification" "$SCRIPT_DIR/phase3-verify.sh"
  CURRENT_STEP="verify report verification"
  require_report "Phase 3 verify" "$VERIFY_REPORT"
  CURRENT_STEP=between-steps
}

run_drill() {
  if [ "$INCLUDE_DRILL" != "true" ]; then
    log "skipping recovery drill"
    return
  fi
  CETS_PHASE3_DRILL_RUN_ID="$RUN_ID" \
    CETS_PHASE3_DRILL_ARTIFACT_DIR="$DRILL_ARTIFACT_DIR" \
    run_step "recovery drill" "$SCRIPT_DIR/phase3-drill.sh"
  CURRENT_STEP="recovery drill report verification"
  require_report "Phase 3 recovery drill" "$DRILL_REPORT"
  CURRENT_STEP=between-steps
}

run_capacity() {
  CETS_PHASE3_CAPACITY_RUN_ID="$RUN_ID" \
    CETS_PHASE3_CAPACITY_ARTIFACT_DIR="$CAPACITY_ARTIFACT_DIR" \
    CETS_PHASE3_CAPACITY_VERIFY_REPORT="$VERIFY_REPORT" \
    run_step "capacity benchmark" "$SCRIPT_DIR/phase3-capacity.sh"
  CURRENT_STEP="capacity report verification"
  require_report "Phase 3 capacity" "$CAPACITY_REPORT"
  CURRENT_STEP=between-steps
}

run_quality() {
  CETS_PHASE3_CAPACITY_REPORT="$CAPACITY_REPORT" \
    CETS_PHASE3_QUALITY_RUN_ID="$RUN_ID" \
    CETS_PHASE3_QUALITY_ARTIFACT_DIR="$QUALITY_ARTIFACT_DIR" \
    CETS_PHASE3_QUALITY_REPORT="$QUALITY_REPORT" \
    CETS_PHASE3_SONAR_RESULT_REPORT="$SONAR_RESULT_REPORT" \
    run_step "quality gate" "$SCRIPT_DIR/phase3-quality.sh"
  CURRENT_STEP="quality report verification"
  require_report "Phase 3 quality" "$QUALITY_REPORT"
  CURRENT_STEP="Sonar result report verification"
  require_report "Phase 3 Sonar result" "$SONAR_RESULT_REPORT"
  CURRENT_STEP=between-steps
}

write_run_report() {
  status=$1
  exit_code=${2:-0}
  {
    printf '# Phase 3 Gated Run Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Status | `%s` |\n' "$status"
    if [ "$status" != "passed" ]; then
      printf '| Failed step | `%s` |\n' "$CURRENT_STEP"
      printf '| Exit code | `%s` |\n' "$exit_code"
    fi
    printf '| Run log | `%s` |\n' "$RUN_LOG"
    printf '| Verify report | `%s` |\n' "$VERIFY_REPORT"
    if [ "$INCLUDE_DRILL" = "true" ]; then
      printf '| Recovery drill report | `%s` |\n' "$DRILL_REPORT"
    else
      printf '| Recovery drill report | `skipped` |\n'
    fi
    printf '| Capacity report | `%s` |\n' "$CAPACITY_REPORT"
    printf '| Quality report | `%s` |\n' "$QUALITY_REPORT"
    printf '| Sonar result report | `%s` |\n' "$SONAR_RESULT_REPORT"
  } >"$RUN_REPORT"
  chmod 0600 "$RUN_REPORT"
  log "wrote Phase 3 gated run report: $RUN_REPORT"
  REPORT_WRITTEN=true
}

finalize() {
  exit_code=$?
  if [ "$exit_code" -ne 0 ] && [ "$REPORT_WRITTEN" != "true" ]; then
    RUN_STATUS=failed
    mkdir -p "$(dirname "$RUN_REPORT")" || true
    write_run_report "$RUN_STATUS" "$exit_code" || true
  fi
  exit "$exit_code"
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

  CURRENT_STEP="artifact setup"
  trap finalize EXIT
  prepare_artifacts

  CURRENT_STEP=preflight
  for script in phase3-deploy.sh phase3-verify.sh phase3-capacity.sh phase3-quality.sh; do
    require_script "$script"
  done
  if [ "$INCLUDE_DRILL" = "true" ]; then
    require_script "phase3-drill.sh"
  fi
  CURRENT_STEP=between-steps

  run_deploy
  run_verify
  run_drill
  run_capacity
  run_quality
  RUN_STATUS=passed
  write_run_report "$RUN_STATUS" 0
}

main "$@"
