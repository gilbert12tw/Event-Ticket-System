#!/usr/bin/env bash
set -euo pipefail

browser="${1:-chromium}"
browsers_path="${PLAYWRIGHT_BROWSERS_PATH:-}"
install_timeout="${PLAYWRIGHT_INSTALL_TIMEOUT:-12m}"
require_cache="${PLAYWRIGHT_REQUIRE_CACHE:-false}"
export PLAYWRIGHT_SKIP_BROWSER_GC="${PLAYWRIGHT_SKIP_BROWSER_GC:-1}"

default_browsers_path() {
  case "$(uname -s)" in
    Darwin) printf '%s\n' "$HOME/Library/Caches/ms-playwright" ;;
    *) printf '%s\n' "$HOME/.cache/ms-playwright" ;;
  esac
}

ensure_playwright_cli() {
  if pnpm --filter cets-web exec playwright --version >/dev/null 2>&1; then
    return 0
  fi
  mkdir -p .pnpm-store
  if command -v flock >/dev/null 2>&1; then
    flock .pnpm-store/act-install.lock pnpm install --frozen-lockfile
  else
    pnpm install --frozen-lockfile
  fi
}

browser_cached() {
  local install_path
  local found=false

  [[ -n "$browsers_path" ]] && [[ -d "$browsers_path" ]] || return 1
  ensure_playwright_cli

  while IFS= read -r install_path; do
    [[ -n "$install_path" ]] || continue
    found=true
    [[ -d "$install_path" ]] || return 1
  done < <(
    pnpm --filter cets-web exec playwright install --dry-run "$browser" |
      awk -F 'Install location:[[:space:]]*' '/Install location:/ { print $2 }'
  )

  [[ "$found" == "true" ]]
}

run_with_timeout() {
  if command -v timeout >/dev/null 2>&1; then
    timeout "$install_timeout" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$install_timeout" "$@"
  else
    "$@"
  fi
}

if [[ -z "$browsers_path" ]]; then
  browsers_path="$(default_browsers_path)"
  export PLAYWRIGHT_BROWSERS_PATH="$browsers_path"
fi

if [[ "${ACT:-}" == "true" ]]; then
  if [[ "${PLAYWRIGHT_BROWSERS_PATH:-}" == "$(default_browsers_path)" && -d /ms-playwright ]]; then
    browsers_path=/ms-playwright
    export PLAYWRIGHT_BROWSERS_PATH="$browsers_path"
  fi

  if browser_cached; then
    ensure_playwright_cli
    echo "Using preinstalled Playwright ${browser} from ${browsers_path}."
    exit 0
  fi

  run_with_timeout pnpm --filter cets-web exec playwright install --with-deps "$browser"
  exit 0
fi

if browser_cached; then
  ensure_playwright_cli
  echo "Using cached Playwright ${browser} from ${browsers_path}."
  exit 0
fi

if [[ "$require_cache" == "true" ]]; then
  echo "::error::Playwright ${browser} cache was required but not found at ${browsers_path}."
  echo "::error::Run the playwright-browsers job first or clear the cache key if the browser revision changed."
  exit 1
fi

run_with_timeout pnpm --filter cets-web exec playwright install "$browser"
