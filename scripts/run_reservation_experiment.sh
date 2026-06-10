#!/usr/bin/env bash
# Reproducible harness for the PH2-22 Redis reservation gate experiment.
# Runs the booking burst twice — once with BOOKING_PREADMISSION=off and once
# with =on — against the same Postgres + Redis containers, capturing each
# run's JSON summary. The plotter (scripts/plot_reservation_experiment.py)
# consumes the two JSON files and emits PNG figures into artifacts/reservation-experiment/figures/.
#
# Usage:
#   scripts/run_reservation_experiment.sh                 # defaults: 200 VUs, capacity 10
#   EXPERIMENT_VUS=500 EXPERIMENT_CAPACITY=25 scripts/run_reservation_experiment.sh
#
# Requires: docker compose + a running daemon, Go toolchain, and Python 3.
# The script creates scripts/.venv and ensures the checked-in plotter
# requirements are installed.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

VUS="${EXPERIMENT_VUS:-200}"
CAPACITY="${EXPERIMENT_CAPACITY:-10}"
OUT_DIR="${EXPERIMENT_OUT_DIR:-artifacts/reservation-experiment/figures}"
DATA_DIR="${EXPERIMENT_DATA_DIR:-artifacts/reservation-experiment/data}"
PLOT_REQUIREMENTS="${REPO_ROOT}/scripts/requirements-reservation-experiment.txt"

mkdir -p "${OUT_DIR}" "${DATA_DIR}"

COMPOSE_ENV_FILE="services/api/deploy/.env.example"
COMPOSE_ARGS=(--env-file "${COMPOSE_ENV_FILE}" -f services/api/deploy/compose.yaml)

echo "[1/5] Bringing up Postgres + Redis"
docker compose "${COMPOSE_ARGS[@]}" up -d postgres redis >/dev/null
docker compose "${COMPOSE_ARGS[@]}" exec -T postgres bash -lc 'until pg_isready -U cets >/dev/null 2>&1; do sleep 0.3; done'

POSTGRES_PASSWORD="$(awk -F= '$1 == "POSTGRES_PASSWORD" { print $2 }' "${COMPOSE_ENV_FILE}")"
DATABASE_URL="postgresql://cets:${POSTGRES_PASSWORD}@localhost:${POSTGRES_PORT:-5432}/cets"
REDIS_URL="redis://localhost:${REDIS_PORT:-6379}/0"
export DATABASE_URL REDIS_URL EXPERIMENT_VUS="${VUS}" EXPERIMENT_CAPACITY="${CAPACITY}" EXPERIMENT_ALLOW_DESTRUCTIVE=1

echo "[2/5] Building experiment binary"
(cd services/api && go build -o "${REPO_ROOT}/bin/reservation_experiment" ./cmd/reservation_experiment)

run_mode() {
  local mode="$1"
  local out="${DATA_DIR}/reservation_experiment_${mode}.json"
  echo "    -> mode=${mode}, VUs=${VUS}, capacity=${CAPACITY}"
  EXPERIMENT_MODE="${mode}" "${REPO_ROOT}/bin/reservation_experiment" > "${out}"
  python3 -c "
import json, sys
d = json.load(open('${out}'))
print('       wall=%.1fms  rps=%.0f  p50=%.1f p95=%.1f p99=%.1f  confirmed=%d waitlisted=%d errors=%d db_confirmed=%d' % (
    d['wall_clock_ms'], d['rps'],
    d['latency_ms']['p50'], d['latency_ms']['p95'], d['latency_ms']['p99'],
    d['outcomes']['confirmed'], d['outcomes']['waitlisted'], d['outcomes']['error'],
    d['db_confirmed_count'],
))"
}

echo "[3/5] Running gate=off (Phase 1 DB-only path)"
run_mode off

echo "[4/5] Running gate=on (PH2-22 Redis pre-admission)"
run_mode on

echo "[5/5] Generating figures"
if [[ ! -x "${REPO_ROOT}/scripts/.venv/bin/python" ]]; then
  echo "    -> creating venv at scripts/.venv"
  python3 -m venv "${REPO_ROOT}/scripts/.venv"
fi
"${REPO_ROOT}/scripts/.venv/bin/python" -m pip install --quiet -r "${PLOT_REQUIREMENTS}"
"${REPO_ROOT}/scripts/.venv/bin/python" "${REPO_ROOT}/scripts/plot_reservation_experiment.py" \
  --off "${DATA_DIR}/reservation_experiment_off.json" \
  --on  "${DATA_DIR}/reservation_experiment_on.json" \
  --out "${OUT_DIR}"

echo "Done. Figures in ${OUT_DIR}/, raw data in ${DATA_DIR}/."
