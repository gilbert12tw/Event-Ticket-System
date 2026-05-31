#!/usr/bin/env bash
set -euo pipefail

browser="${1:-chromium}"
browsers_path="${PLAYWRIGHT_BROWSERS_PATH:-}"

if [[ "${ACT:-}" == "true" ]]; then
  if [[ -z "$browsers_path" && -d /ms-playwright ]]; then
    browsers_path=/ms-playwright
    export PLAYWRIGHT_BROWSERS_PATH="$browsers_path"
  fi

  if [[ -n "$browsers_path" ]] && find "$browsers_path" -maxdepth 1 -type d -name "${browser}-*" | grep -q .; then
    if ! pnpm --filter cets-web exec playwright --version >/dev/null 2>&1; then
      mkdir -p .pnpm-store
      if command -v flock >/dev/null 2>&1; then
        flock .pnpm-store/act-install.lock pnpm install --frozen-lockfile
      else
        pnpm install --frozen-lockfile
      fi
    fi
    echo "Using preinstalled Playwright ${browser} from ${browsers_path}."
    exit 0
  fi

  timeout 5m pnpm --filter cets-web exec playwright install --with-deps "$browser"
  exit 0
fi

pnpm --filter cets-web exec playwright install "$browser"
