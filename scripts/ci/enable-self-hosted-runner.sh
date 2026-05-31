#!/usr/bin/env bash
set -euo pipefail

repo="${GITHUB_REPOSITORY:-}"
labels_json="${CI_RUNNER_LABELS_VALUE:-[\"self-hosted\",\"linux\",\"x64\",\"cets-ci\"]}"
mode="${CETS_RUNNER_SWITCH_MODE:-enable}"
apply="${APPLY:-false}"
allow_busy="${CETS_ALLOW_BUSY_RUNNER:-false}"
runner_list_json="${CETS_RUNNER_LIST_JSON:-}"

log() {
  printf '[runner-switch] %s\n' "$*"
}

fail() {
  printf '[runner-switch] FAIL: %s\n' "$*" >&2
  exit 1
}

require_command() {
  local name="$1"
  command -v "$name" >/dev/null 2>&1 || fail "$name is required"
}

detect_repo() {
  if [[ -n "$repo" ]]; then
    return
  fi

  if git_url="$(git config --get remote.origin.url 2>/dev/null)" && [[ -n "$git_url" ]]; then
    case "$git_url" in
      git@github.com:*)
        repo="${git_url#git@github.com:}"
        repo="${repo%.git}"
        ;;
      https://github.com/*)
        repo="${git_url#https://github.com/}"
        repo="${repo%.git}"
        ;;
    esac
  fi

  [[ -n "$repo" ]] || fail "set GITHUB_REPOSITORY=owner/repo or configure a GitHub origin remote"
}

validate_mode() {
  case "$mode" in
    enable | disable | check)
      ;;
    *)
      fail "CETS_RUNNER_SWITCH_MODE must be enable, disable, or check"
      ;;
  esac
}

runner_json() {
  if [[ -n "$runner_list_json" ]]; then
    printf '%s\n' "$runner_list_json"
    return
  fi

  require_command gh
  local err_file
  err_file="$(mktemp)"
  if gh api "repos/${repo}/actions/runners" --paginate 2>"$err_file"; then
    rm -f "$err_file"
    return
  fi

  local err
  err="$(sed -n '1,3p' "$err_file")"
  rm -f "$err_file"
  fail "could not list GitHub self-hosted runners for ${repo}; repository admin permission is usually required. gh said: ${err}"
}

validate_runner_ready() {
  local json
  json="$(runner_json)"

  RUNNERS_JSON="$json" LABELS_JSON="$labels_json" ALLOW_BUSY="$allow_busy" python3 - <<'PY'
import json
import os
import sys

payload = json.loads(os.environ["RUNNERS_JSON"])
required = set(json.loads(os.environ["LABELS_JSON"]))
allow_busy = os.environ["ALLOW_BUSY"].lower() == "true"

if not required:
    print("required label list is empty", file=sys.stderr)
    sys.exit(1)

runners = payload.get("runners", [])
ready = []
near = []

for runner in runners:
    labels = {label.get("name", "") for label in runner.get("labels", [])}
    missing = sorted(required - labels)
    if missing:
        near.append(
            f"{runner.get('name', '<unnamed>')}: missing labels {', '.join(missing)}"
        )
        continue
    if runner.get("status") != "online":
        near.append(f"{runner.get('name', '<unnamed>')}: status={runner.get('status')}")
        continue
    if runner.get("busy") and not allow_busy:
        near.append(f"{runner.get('name', '<unnamed>')}: busy=true")
        continue
    ready.append(runner.get("name", "<unnamed>"))

if ready:
    print(f"ready runner: {ready[0]}")
    sys.exit(0)

if near:
    print("no ready runner found; closest candidates:", file=sys.stderr)
    for line in near[:10]:
        print(f"- {line}", file=sys.stderr)
else:
    print("no repository self-hosted runners returned by GitHub", file=sys.stderr)
sys.exit(1)
PY
}

set_runner_variable() {
  require_command gh
  gh variable set CI_RUNNER_LABELS --body "$labels_json" --repo "$repo"
}

delete_runner_variable() {
  require_command gh
  if gh variable list --repo "$repo" --json name --jq '.[].name' | grep -qx 'CI_RUNNER_LABELS'; then
    gh variable delete CI_RUNNER_LABELS --repo "$repo" --yes
  else
    log "CI_RUNNER_LABELS is already unset"
  fi
}

require_command python3
detect_repo
validate_mode

case "$mode" in
  check)
    validate_runner_ready
    log "runner is ready for ${repo}; CI_RUNNER_LABELS would be ${labels_json}"
    ;;
  enable)
    validate_runner_ready
    if [[ "$apply" == "true" ]]; then
      set_runner_variable
      log "set CI_RUNNER_LABELS=${labels_json} for ${repo}"
    else
      log "dry run only; rerun with APPLY=true to set CI_RUNNER_LABELS=${labels_json}"
    fi
    ;;
  disable)
    if [[ "$apply" == "true" ]]; then
      delete_runner_variable
      log "removed CI_RUNNER_LABELS for ${repo}; workflow will fall back to ubuntu-latest"
    else
      log "dry run only; rerun with APPLY=true CETS_RUNNER_SWITCH_MODE=disable to remove CI_RUNNER_LABELS"
    fi
    ;;
esac
