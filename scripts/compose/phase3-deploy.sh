#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
PHASE3_SERVICES=(
  backend-1 backend-2 backend-3 backend-lb
  frontend-1 frontend-2 frontend-3 frontend-lb
  gateway-1 gateway-2 gateway-3 edge-lb
  worker-notification worker-projection worker-compensation worker-export
)
OBSERVABILITY_SERVICES=(loki prometheus tempo pyroscope alloy grafana)

log() {
  printf '[phase3-compose-deploy] %s\n' "$*"
}

die() {
  printf '[phase3-compose-deploy] error: %s\n' "$*" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
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

wait_container_healthy() {
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
  compose ps
  die "$service did not become healthy"
}

wait_container_exit_success() {
  service=$1
  for _ in $(seq 1 60); do
    container=$(compose ps -a -q "$service")
    if [ -n "$container" ]; then
      status=$(docker inspect --format '{{.State.Status}} {{.State.ExitCode}}' "$container" 2>/dev/null || true)
      case "$status" in
        "exited 0")
          return
          ;;
        exited\ *)
          compose logs "$service"
          die "$service exited unsuccessfully: $status"
          ;;
      esac
    fi
    sleep 2
  done
  compose ps
  die "$service did not complete"
}

main() {
  have docker || die "docker is required"
  docker compose version >/dev/null || die "docker compose v2 is required"
  require_docker_daemon

  log "building backend and frontend images"
  compose build backend-1 frontend-1

  log "starting backing services and LGTM"
  compose up -d postgres redis minio mailhog "${OBSERVABILITY_SERVICES[@]}"
  wait_container_healthy postgres
  wait_container_healthy redis

  log "initializing object storage"
  compose up -d --force-recreate minio-init
  wait_container_exit_success minio-init

  log "running migrate and seed admin processes"
  compose up -d --force-recreate migrate
  wait_container_exit_success migrate
  compose up -d --force-recreate seed
  wait_container_exit_success seed

  log "starting Phase 3 HA topology"
  compose up -d "${PHASE3_SERVICES[@]}" "${OBSERVABILITY_SERVICES[@]}"
  for service in "${PHASE3_SERVICES[@]}"; do
    wait_container_healthy "$service"
  done

  log "Phase 3 Compose HA simulation deployed"
}

main "$@"
