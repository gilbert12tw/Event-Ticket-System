#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
EDGE_URL=${CETS_PHASE3_URL:-http://127.0.0.1:${PHASE3_EDGE_PORT:-18080}}
DRILL_SERVICES=(gateway-1 frontend-1 backend-1)

log() {
  printf '[phase3-compose-drill] %s\n' "$*"
}

die() {
  printf '[phase3-compose-drill] error: %s\n' "$*" >&2
  exit 1
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
  log "stopping $service"
  compose stop "$service" >/dev/null
  smoke
  log "starting $service"
  start_existing_container "$service"
  wait_healthy "$service"
  smoke
}

main() {
  command -v docker >/dev/null 2>&1 || die "docker is required"
  command -v curl >/dev/null 2>&1 || die "curl is required"
  for service in "${DRILL_SERVICES[@]}"; do
    drill_service "$service"
  done
  log "Phase 3 Compose HA recovery drill completed"
}

main "$@"
