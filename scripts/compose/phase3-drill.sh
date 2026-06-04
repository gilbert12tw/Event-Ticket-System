#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
EDGE_URL=${CETS_PHASE3_URL:-http://127.0.0.1:${PHASE3_EDGE_PORT:-18080}}
DRILL_SERVICES=(gateway-1 frontend-1 backend-1)
ARTIFACT_DIR=${CETS_PHASE3_DRILL_ARTIFACT_DIR:-$ROOT_DIR/artifacts/phase3-drill}
RUN_ID=${CETS_PHASE3_DRILL_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
DRILL_EVENTS="$ARTIFACT_DIR/phase3-drill-events-$RUN_ID.txt"
DRILL_REPORT="$ARTIFACT_DIR/phase3-drill-report-$RUN_ID.md"
CURRENT_SERVICE=""
SERVICE_STOPPED=false
DRILL_STATUS=failed
REPORT_WRITTEN=false

log() {
  printf '[phase3-compose-drill] %s\n' "$*"
}

die() {
  printf '[phase3-compose-drill] error: %s\n' "$*" >&2
  exit 1
}

require_docker_daemon() {
  docker info >/dev/null 2>&1 || die "docker daemon access is required"
}

compose() {
  docker compose \
    --env-file "$ENV_FILE" \
    -f "$ROOT_DIR/services/api/deploy/compose.yaml" \
    -f "$ROOT_DIR/services/api/deploy/compose.worker-isolation.yaml" \
    -f "$ROOT_DIR/services/api/deploy/compose.phase3-ha.yaml" \
    --profile phase3-ha \
    --profile worker-isolation \
    --profile observability \
    "$@"
}

http_get() {
  curl -fsS "$1" >/dev/null
}

record_event() {
  local service=$1
  local stage=$2
  local timestamp
  timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  printf '%s|%s|%s\n' "$timestamp" "$service" "$stage" >>"$DRILL_EVENTS"
}

cleanup_stopped_service() {
  [ "$SERVICE_STOPPED" = "true" ] || return
  [ -n "$CURRENT_SERVICE" ] || return
  record_event "$CURRENT_SERVICE" "cleanup_restart_attempted"
  container=$(compose ps -a -q "$CURRENT_SERVICE" 2>/dev/null || true)
  if [ -n "$container" ] && docker start "$container" >/dev/null 2>&1; then
    record_event "$CURRENT_SERVICE" "cleanup_restart_started"
  else
    record_event "$CURRENT_SERVICE" "cleanup_restart_failed"
  fi
}

finalize() {
  local exit_status=$?
  set +e
  if [ "$exit_status" -ne 0 ] && [ -n "$CURRENT_SERVICE" ] && [ -f "$DRILL_EVENTS" ]; then
    record_event "$CURRENT_SERVICE" "failed"
    cleanup_stopped_service
  fi
  if [ "$REPORT_WRITTEN" != "true" ] && [ -d "$ARTIFACT_DIR" ]; then
    write_report "$DRILL_STATUS"
  fi
  exit "$exit_status"
}

wait_healthy() {
  service=$1
  for _ in $(seq 1 60); do
    container=$(compose ps -q "$service")
    if [ -n "$container" ]; then
      status=$(docker inspect --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container" 2>/dev/null || true)
      case "$status" in
        "running healthy" | "running " | "running")
          return
          ;;
      esac
    fi
    sleep 2
  done
  die "$service did not recover"
}

start_existing_container() {
  service=$1
  container=$(compose ps -a -q "$service")
  [ -n "$container" ] || die "$service has no existing container to restart"
  docker start "$container" >/dev/null
}

smoke() {
  http_get "$EDGE_URL/healthz"
  http_get "$EDGE_URL/readyz"
  http_get "$EDGE_URL/"
}

drill_service() {
  service=$1
  CURRENT_SERVICE=$service
  SERVICE_STOPPED=false
  log "stopping $service"
  compose stop "$service" >/dev/null
  SERVICE_STOPPED=true
  record_event "$service" "stopped"
  smoke
  record_event "$service" "smoke_while_stopped"
  log "starting $service"
  start_existing_container "$service"
  record_event "$service" "started"
  wait_healthy "$service"
  SERVICE_STOPPED=false
  record_event "$service" "healthy_after_restart"
  smoke
  record_event "$service" "smoke_after_restart"
  CURRENT_SERVICE=""
}

write_report() {
  local report_status=$1
  {
    printf '# Phase 3 Recovery Drill Report\n\n'
    printf '| Field | Value |\n| --- | --- |\n'
    printf '| Run ID | `%s` |\n' "$RUN_ID"
    printf '| Status | `%s` |\n' "$report_status"
    printf '| Edge URL | `%s` |\n' "$EDGE_URL"
    printf '| Services | `%s` |\n' "${DRILL_SERVICES[*]}"
    printf '| Event evidence | `%s` |\n' "$DRILL_EVENTS"
  } >"$DRILL_REPORT"
  REPORT_WRITTEN=true
  log "wrote recovery drill report: $DRILL_REPORT"
}

main() {
  command -v docker >/dev/null 2>&1 || die "docker is required"
  command -v curl >/dev/null 2>&1 || die "curl is required"
  require_docker_daemon
  mkdir -p "$ARTIFACT_DIR"
  chmod 0700 "$ARTIFACT_DIR"
  : >"$DRILL_EVENTS"
  trap finalize EXIT
  for service in "${DRILL_SERVICES[@]}"; do
    drill_service "$service"
  done
  DRILL_STATUS=passed
  write_report "$DRILL_STATUS"
  log "Phase 3 Compose HA recovery drill completed"
}

main "$@"
