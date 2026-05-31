#!/usr/bin/env bash
set -euo pipefail

min_cpus="${CETS_RUNNER_MIN_CPUS:-4}"
min_mem_gib="${CETS_RUNNER_MIN_MEM_GIB:-12}"
min_disk_gib="${CETS_RUNNER_MIN_DISK_GIB:-80}"
run_docker_probe="${CETS_RUNNER_DOCKER_PROBE:-true}"
allow_non_linux="${CETS_RUNNER_ALLOW_NON_LINUX:-false}"

failures=0

log() {
  printf '[runner-preflight] %s\n' "$*"
}

fail() {
  log "FAIL: $*"
  failures=$((failures + 1))
}

pass() {
  log "PASS: $*"
}

require_command() {
  local name="$1"
  if command -v "$name" >/dev/null 2>&1; then
    pass "$name found: $(command -v "$name")"
  else
    fail "$name is not installed or not on PATH"
  fi
}

version_major() {
  local version="$1"
  printf '%s\n' "$version" | sed -E 's/^[^0-9]*([0-9]+).*/\1/'
}

check_cpu() {
  local cpus
  cpus="$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || printf '0')"
  if [[ "$cpus" =~ ^[0-9]+$ ]] && (( cpus >= min_cpus )); then
    pass "CPU cores ${cpus} >= ${min_cpus}"
  else
    fail "CPU cores ${cpus:-unknown} < ${min_cpus}"
  fi
}

check_os_arch() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"

  if [[ "$allow_non_linux" == "true" ]]; then
    log "INFO: skipping Linux x64 enforcement for local script validation on ${os}/${arch}"
    return
  fi

  if [[ "$os" == "Linux" ]]; then
    pass "operating system is Linux"
  else
    fail "operating system is ${os}; recommended runner must be Linux"
  fi

  case "$arch" in
    x86_64 | amd64)
      pass "architecture is x64"
      ;;
    *)
      fail "architecture is ${arch}; recommended runner must be x64"
      ;;
  esac
}

check_memory() {
  local mem_kib mem_bytes mem_gib
  if [[ -r /proc/meminfo ]]; then
    mem_kib="$(awk '/MemTotal/ {print $2}' /proc/meminfo)"
    mem_gib="$((mem_kib / 1024 / 1024))"
  else
    mem_bytes="$(sysctl -n hw.memsize 2>/dev/null || printf '0')"
    mem_gib="$((mem_bytes / 1024 / 1024 / 1024))"
  fi

  if (( mem_gib >= min_mem_gib )); then
    pass "memory ${mem_gib} GiB >= ${min_mem_gib} GiB"
  else
    fail "memory ${mem_gib} GiB < ${min_mem_gib} GiB"
  fi
}

check_disk() {
  local disk_gib
  disk_gib="$(df -Pk . | awk 'NR == 2 {print int($4 / 1024 / 1024)}')"
  if (( disk_gib >= min_disk_gib )); then
    pass "free disk ${disk_gib} GiB >= ${min_disk_gib} GiB"
  else
    fail "free disk ${disk_gib} GiB < ${min_disk_gib} GiB"
  fi
}

check_node() {
  require_command node
  if command -v node >/dev/null 2>&1; then
    local major
    major="$(version_major "$(node --version)")"
    if [[ "$major" =~ ^[0-9]+$ ]] && (( major >= 24 )); then
      pass "Node.js major ${major} >= 24"
    else
      fail "Node.js major ${major:-unknown} < 24"
    fi
  fi
}

check_go() {
  require_command go
  if command -v go >/dev/null 2>&1; then
    pass "$(go version)"
  fi
}

check_docker() {
  require_command docker
  if ! command -v docker >/dev/null 2>&1; then
    return
  fi

  if docker info >/dev/null 2>&1; then
    pass "Docker daemon is reachable by current user"
  else
    fail "Docker daemon is not reachable by current user"
  fi

  if docker compose version >/dev/null 2>&1; then
    pass "$(docker compose version)"
  else
    fail "Docker Compose plugin is unavailable"
  fi

  if docker buildx version >/dev/null 2>&1; then
    pass "$(docker buildx version)"
  else
    fail "Docker Buildx is unavailable"
  fi

  if [[ "$run_docker_probe" == "true" ]]; then
    if docker run --rm hello-world >/dev/null 2>&1; then
      pass "Docker can pull and run a public test image"
    else
      fail "Docker could not pull/run hello-world; check network and Docker Hub access"
    fi
  fi
}

check_playwright_cache_hint() {
  if [[ -d /ms-playwright ]]; then
    pass "Playwright image browser cache exists at /ms-playwright"
  elif [[ -d "$HOME/.cache/ms-playwright" ]]; then
    pass "Playwright browser cache exists under HOME"
  else
    log "INFO: Playwright browsers are not preinstalled; CI will run scripts/ci/install-playwright.sh"
  fi
}

check_github_runner_labels() {
  local labels="${CETS_RUNNER_LABELS:-}"
  if [[ -z "$labels" ]]; then
    log "INFO: set repository variable CI_RUNNER_LABELS to [\"self-hosted\",\"linux\",\"x64\",\"cets-ci\"] after registering the runner"
    return
  fi

  for required in self-hosted linux x64 cets-ci; do
    if [[ "$labels" == *"$required"* ]]; then
      pass "runner label '$required' present in CETS_RUNNER_LABELS"
    else
      fail "runner label '$required' missing from CETS_RUNNER_LABELS"
    fi
  done
}

check_os_arch
check_cpu
check_memory
check_disk
check_node
require_command pnpm
check_go
require_command ruby
require_command curl
require_command git
check_docker
check_playwright_cache_hint
check_github_runner_labels

if (( failures > 0 )); then
  log "self-hosted runner preflight failed with ${failures} issue(s)"
  exit 1
fi

log "self-hosted runner preflight passed"
