#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENV_FILE=${CETS_PHASE3_ENV_FILE:-$ROOT_DIR/services/api/deploy/.env.example}
PROFILE=${K6_PHASE3_PROFILE:-smoke}
PHASE3_EDGE_PORT=${PHASE3_EDGE_PORT:-}
EDGE_URL=${CETS_PHASE3_URL:-}
ARTIFACT_DIR=${CETS_PHASE3_K6_ARTIFACT_DIR:-$ROOT_DIR/artifacts/phase3-k6}
SUMMARY_FILE=${CETS_PHASE3_K6_SUMMARY:-$ARTIFACT_DIR/phase3-${PROFILE}-summary.json}
REPLICA_SPREAD_FILE=${CETS_PHASE3_K6_REPLICA_SPREAD:-$ARTIFACT_DIR/phase3-${PROFILE}-replica-spread.txt}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.7.1-with-browser}
SCRIPT=/k6/phase3-ha-lgtm.js
HEADER_REPLICAS_AWK="$ROOT_DIR/scripts/compose/phase3-header-replicas.awk"
REPLICA_SPREAD_CHECK="$ROOT_DIR/scripts/compose/phase3-replica-spread-check.awk"

log() {
  printf '[phase3-k6] %s\n' "$*"
}

die() {
  printf '[phase3-k6] error: %s\n' "$*" >&2
  exit 1
}

require_docker_daemon() {
  docker info >/dev/null 2>&1 || die "docker daemon access is required"
}

env_value() {
  key=$1
  default=$2
  if [ -f "$ENV_FILE" ]; then
    value=$(awk -F= -v key="$key" '
      $0 !~ /^[[:space:]]*#/ && $1 == key {
        print substr($0, index($0, "=") + 1)
        exit
      }
    ' "$ENV_FILE")
    if [ -n "$value" ]; then
      printf '%s' "$value"
      return
    fi
  fi
  printf '%s' "$default"
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

compose_network() {
  docker inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' cets-phase3-edge-lb 2>/dev/null | head -n 1
}

run_k6() {
  mkdir -p "$ARTIFACT_DIR"
  chmod 0777 "$ARTIFACT_DIR"
  rm -f "$SUMMARY_FILE"
  rm -f "$REPLICA_SPREAD_FILE"
  case "$PROFILE" in
    smoke | stress)
      log "running $PROFILE profile through $EDGE_URL"
      docker run --rm --network host \
        -v "$ROOT_DIR/k6:/k6:ro" \
        -v "$ARTIFACT_DIR:/artifacts" \
        -e BASE_URL="$EDGE_URL" \
        -e EDGE_URL="$EDGE_URL" \
        -e K6_PHASE3_PROFILE="$PROFILE" \
        -e K6_PHASE3_DURATION="${K6_PHASE3_DURATION:-}" \
        "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$SUMMARY_FILE")" "$SCRIPT"
      ;;
    investigate)
      network=${CETS_PHASE3_COMPOSE_NETWORK:-$(compose_network)}
      [ -n "$network" ] || die "could not determine Phase 3 Compose network"
      log "running investigate profile through $EDGE_URL and direct backend-1 on $network"
      docker run --rm --network "$network" \
        -v "$ROOT_DIR/k6:/k6:ro" \
        -v "$ARTIFACT_DIR:/artifacts" \
        -e BASE_URL="${CETS_PHASE3_INTERNAL_EDGE_URL:-http://edge-lb:8080}" \
        -e EDGE_URL="${CETS_PHASE3_INTERNAL_EDGE_URL:-http://edge-lb:8080}" \
        -e DIRECT_BACKEND_URL="${DIRECT_BACKEND_URL:-http://backend-1:8080}" \
        -e K6_PHASE3_PROFILE="$PROFILE" \
        -e K6_PHASE3_DURATION="${K6_PHASE3_DURATION:-}" \
        "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$SUMMARY_FILE")" "$SCRIPT"
      ;;
    *)
      die "K6_PHASE3_PROFILE must be one of smoke|stress|investigate"
      ;;
  esac
}

header_replicas() {
  header=$1
  sample_file=$2
  awk -v header="$header" -f "$HEADER_REPLICAS_AWK" "$sample_file"
}

require_replica_spread_summary() {
  summary=$1
  awk -f "$REPLICA_SPREAD_CHECK" "$summary" || die "Phase 3 k6 replica spread invariants failed"
}

require_replica_spread() {
  [ -f "$SUMMARY_FILE" ] || die "k6 summary was not written: $SUMMARY_FILE"
  sample_file="$ARTIFACT_DIR/phase3-${PROFILE}-headers.txt"
  : >"$sample_file"
  for _ in $(seq 1 "${K6_PHASE3_REPLICA_SAMPLES:-90}"); do
    curl -fsSI "$EDGE_URL/readyz" >>"$sample_file"
    printf '\n' >>"$sample_file"
  done
  gateway=$(header_replicas X-CETS-Gateway-Replica "$sample_file")
  frontend=$(header_replicas X-CETS-Frontend-Replica "$sample_file")
  backend=$(header_replicas X-CETS-Backend-Replica "$sample_file")
  spread_tmp="$REPLICA_SPREAD_FILE.tmp.$$"
  {
    printf 'gateway|%s\n' "$gateway"
    printf 'frontend|%s\n' "$frontend"
    printf 'backend|%s\n' "$backend"
    printf 'headers|%s\n' "$sample_file"
  } >"$spread_tmp"
  log "observed replicas: gateway=$gateway frontend=$frontend backend=$backend"
  require_replica_spread_summary "$spread_tmp"
  mv "$spread_tmp" "$REPLICA_SPREAD_FILE"
}

main() {
  command -v docker >/dev/null 2>&1 || die "docker is required"
  require_docker_daemon
  PHASE3_EDGE_PORT=${PHASE3_EDGE_PORT:-$(env_value PHASE3_EDGE_PORT 18080)}
  EDGE_URL=${EDGE_URL:-http://127.0.0.1:${PHASE3_EDGE_PORT}}
  run_k6
  require_replica_spread
  log "Phase 3 k6 $PROFILE profile passed"
}

main "$@"
