#!/usr/bin/env bash
set -euo pipefail

channel="${1:-chrome}"

print_chrome_version() {
  local binary
  for binary in google-chrome google-chrome-stable chrome chromium chromium-browser; do
    if command -v "$binary" >/dev/null 2>&1; then
      "$binary" --version
      return 0
    fi
  done

  if [[ "$(uname -s)" == "Darwin" ]]; then
    local mac_chrome="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
    if [[ -x "$mac_chrome" ]]; then
      "$mac_chrome" --version
      return 0
    fi
  fi

  return 1
}

case "$channel" in
  chrome)
    if print_chrome_version; then
      exit 0
    fi
    echo "::error::Google Chrome was not found for Playwright channel=chrome."
    echo "::error::Install Chrome on the runner, or run local act with the bundled Playwright image."
    exit 1
    ;;
  chromium)
    scripts/ci/install-playwright.sh chromium
    ;;
  *)
    echo "::error::Unsupported Playwright runtime channel: ${channel}"
    exit 1
    ;;
esac
