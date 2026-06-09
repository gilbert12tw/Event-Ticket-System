#!/usr/bin/env bash
set -euo pipefail

changed_file="${CHANGED_FILES_FILE:-/tmp/changed-files}"
preset_changed_files="${CI_CHANGED_FILES_PRESET:-false}"
if [[ "$preset_changed_files" == "true" ]]; then
  cat > "$changed_file"
else
  : > "$changed_file"
fi

event_name="${GITHUB_EVENT_NAME:-}"
ref="${GITHUB_REF:-}"
force_full=false
release_full=false

if [[ "$preset_changed_files" == "true" ]]; then
  :
elif [[ "$event_name" == "release" || "$ref" == refs/tags/v* ]]; then
  release_full=true
elif [[ "$event_name" == "workflow_dispatch" ]]; then
  :
else
  if [[ "$event_name" == "pull_request" ]]; then
    base="${GITHUB_BASE_SHA:-}"
    head="${GITHUB_HEAD_SHA:-${GITHUB_SHA:-HEAD}}"
  else
    base="${GITHUB_BEFORE:-}"
    head="${GITHUB_AFTER:-${GITHUB_SHA:-HEAD}}"
  fi

  if [[ -z "$base" || "$base" =~ ^0+$ ]]; then
    base="$(git merge-base HEAD origin/main 2>/dev/null || true)"
    if [[ -z "$base" ]]; then
      git fetch --no-tags --depth=1 origin main:refs/remotes/origin/main 2>/dev/null || true
      base="$(git merge-base HEAD origin/main 2>/dev/null || true)"
    fi
    if [[ -z "$base" ]]; then
      base="$(git merge-base HEAD main 2>/dev/null || true)"
    fi
    if [[ -z "$base" && -n "${head:-}" ]]; then
      base="$(git rev-list --max-parents=0 "$head")"
    fi
  fi

  if [[ -z "${base:-}" || -z "${head:-}" ]]; then
    force_full=true
  elif ! git cat-file -e "$base^{commit}" 2>/dev/null; then
    git fetch --no-tags --depth=1 origin "$base" || force_full=true
  fi

  if [[ "$force_full" == "false" ]]; then
    git diff --check "$base" "$head"
    git diff --name-only "$base" "$head" > "$changed_file"
  else
    echo "Base commit ${base:-<empty>} is unavailable; running the full CI gate."
  fi

  if [[ "$force_full" == "false" && ! -s "$changed_file" && -n "${head:-}" ]]; then
    git show --pretty="" --name-only "$head" > "$changed_file"
  fi
fi

cat "$changed_file"

if grep -Eq '^(goal\.md|note\.md)$' "$changed_file"; then
  echo "goal.md and note.md are local agent notes and must not be included in PR or push diffs." >&2
  exit 1
fi

full=$force_full
backend=false
backend_lint=false
backend_test=false
frontend_static=false
frontend_unit=false
frontend_e2e=false
openapi=false
compose=false
live=false
k6=false
phase3=false

enable_fast_full() {
  backend_lint=true
  backend_test=true
  frontend_static=true
  frontend_unit=true
  frontend_e2e=true
  openapi=true
  compose=true
}

run_full_ci="${INPUT_RUN_FULL_CI:-false}"
run_live_playwright="${INPUT_RUN_LIVE_PLAYWRIGHT:-false}"
run_k6_smoke="${INPUT_RUN_K6_SMOKE:-false}"
run_k6_release="${INPUT_RUN_K6_RELEASE:-false}"
run_phase3_compose="${INPUT_RUN_PHASE3_COMPOSE:-false}"

if [[ "$release_full" == "true" ]]; then
  full=true
elif [[ "$event_name" == "workflow_dispatch" ]]; then
  if [[ "$run_full_ci" == "true" ]]; then
    full=true
  else
    [[ "$run_live_playwright" == "true" ]] && live=true
    if [[ "$run_k6_smoke" == "true" || "$run_k6_release" == "true" ]]; then
      live=true
      k6=true
    fi
    if [[ "$run_phase3_compose" == "true" ]]; then
      compose=true
      phase3=true
    fi
  fi
fi

ci_config_changed=false
if grep -Eq '^(\.actrc$|\.github/workflows/ci\.yml$|\.github/actions/setup-docker-act/|\.github/actions/setup-web/|scripts/ci/detect-changes\.sh$)' "$changed_file"; then
  ci_config_changed=true
fi

if [[ "$full" == "false" ]]; then
  [[ "$ci_config_changed" == "true" ]] && enable_fast_full

  grep -Eq '^(services/api/.*\.go|services/api/go\.(mod|sum)|go\.work(\.sum)?$)' "$changed_file" && backend_lint=true
  grep -Eq '^(services/api/.*\.go|services/api/go\.(mod|sum)|go\.work(\.sum)?$)' "$changed_file" && backend_test=true
  grep -Eq '^(docs/openapi\.yaml$|docs/openapi/|scripts/.*openapi.*)' "$changed_file" && openapi=true
  grep -Eq '^(services/api/deploy/|services/api/Dockerfile$|scripts/compose/)' "$changed_file" && compose=true
  grep -Eq '^(docs/specs/phase3-|docs/reports/phase3-)' "$changed_file" && phase3=true
  grep -Eq '^(apps/web/src/|apps/web/package\.json$|apps/web/vite\.config\.ts$|package\.json$|pnpm-lock\.yaml$|pnpm-workspace\.yaml$|turbo\.json$|eslint\.config\.mjs$|\.npmrc$|\.prettierignore$)' "$changed_file" && frontend_static=true
  grep -Eq '^(apps/web/src/|apps/web/package\.json$|apps/web/vite\.config\.ts$|package\.json$|pnpm-lock\.yaml$|pnpm-workspace\.yaml$|turbo\.json$)' "$changed_file" && frontend_unit=true
  grep -Eq '^(apps/web/src/|apps/web/e2e/|apps/web/playwright\.config\.ts$|apps/web/package\.json$|pnpm-lock\.yaml$)' "$changed_file" && frontend_e2e=true
else
  enable_fast_full
  live=true
  k6=true
  phase3=true
fi

[[ "$compose" == "true" || "$phase3" == "true" ]] && backend_test=true
[[ "$backend_lint" == "true" || "$backend_test" == "true" ]] && backend=true

quick=false
if [[ "$full" == "false" &&
  "$backend" == "false" &&
  "$frontend_static" == "false" &&
  "$frontend_unit" == "false" &&
  "$frontend_e2e" == "false" &&
  "$openapi" == "false" &&
  "$compose" == "false" &&
  "$live" == "false" &&
  "$k6" == "false" &&
  "$phase3" == "false" ]]; then
  quick=true
fi

{
  echo "backend=$backend"
  echo "backend_lint=$backend_lint"
  echo "backend_test=$backend_test"
  echo "frontend_static=$frontend_static"
  echo "frontend_unit=$frontend_unit"
  echo "frontend_e2e=$frontend_e2e"
  echo "openapi=$openapi"
  echo "compose=$compose"
  echo "live=$live"
  echo "k6=$k6"
  echo "phase3=$phase3"
  echo "quick=$quick"
  echo "full=$full"
} >> "${GITHUB_OUTPUT:-/dev/stdout}"
