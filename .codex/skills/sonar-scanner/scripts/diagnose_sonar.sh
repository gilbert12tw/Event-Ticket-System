#!/usr/bin/env bash
set -u

print_section() {
  printf '\n== %s ==\n' "$1"
}

ok() {
  printf '[ok] %s\n' "$1"
}

warn() {
  printf '[warn] %s\n' "$1"
}

info() {
  printf '[info] %s\n' "$1"
}

command_path() {
  command -v "$1" 2>/dev/null || true
}

has_command() {
  [ -n "$(command_path "$1")" ]
}

repo_root() {
  if has_command git; then
    git rev-parse --show-toplevel 2>/dev/null && return 0
  fi
  pwd
}

tool_status() {
  name=$1
  path=$(command_path "$name")
  if [ -n "$path" ]; then
    ok "$name: $path"
  else
    warn "$name: missing"
  fi
}

property_value() {
  file=$1
  key=$2
  awk -F= -v key="$key" '
    $1 == key {
      value = $0
      sub("^[^=]+=", "", value)
      print value
      exit
    }
  ' "$file" 2>/dev/null || true
}

safe_url() {
  url=$1
  case "$url" in
    *://*@*)
      scheme=${url%%://*}
      rest=${url#*://}
      after_userinfo=${rest#*@}
      printf '%s://***@%s' "$scheme" "$after_userinfo"
      ;;
    *)
      printf '%s' "$url"
      ;;
  esac
}

ROOT_DIR=$(repo_root)
cd "$ROOT_DIR" || exit 1

print_section "Sonar toolchain"
for tool in sonar-scanner docker brew curl jq pnpm; do
  tool_status "$tool"
done

print_section "Sonar environment"
if [ -n "${SONAR_HOST_URL:-}" ]; then
  ok "SONAR_HOST_URL is set: $(safe_url "$SONAR_HOST_URL")"
else
  warn "SONAR_HOST_URL is not set"
fi

if [ -n "${SONAR_TOKEN:-}" ]; then
  ok "SONAR_TOKEN is set"
else
  warn "SONAR_TOKEN is not set"
fi

if [ -n "${SONAR_HOST_URL:-}" ]; then
  if has_command curl; then
    status_url="${SONAR_HOST_URL%/}/api/system/status"
    if curl -fsS --max-time 3 "$status_url" >/dev/null 2>&1; then
      ok "Sonar server status endpoint is reachable"
    else
      warn "Sonar server status endpoint is not reachable from this shell"
    fi
  else
    info "Skipping reachability check because curl is missing"
  fi
fi

print_section "Repo Sonar config"
if [ -f sonar-project.properties ]; then
  ok "Found sonar-project.properties"
  project_key=$(property_value sonar-project.properties sonar.projectKey)
  host_url=$(property_value sonar-project.properties sonar.host.url)
  token_property=$(property_value sonar-project.properties sonar.token)
  [ -n "$project_key" ] && info "sonar.projectKey=$project_key"
  [ -n "$host_url" ] && info "sonar.host.url=$host_url"
  if [ -n "$token_property" ]; then
    case "$token_property" in
      *SONAR_TOKEN*) info "sonar.token is env-based" ;;
      *) warn "sonar.token is not obviously env-based; do not commit real tokens" ;;
    esac
  fi
else
  warn "sonar-project.properties is missing"
fi

if [ -f package.json ]; then
  if grep -q '"sonar:scan"' package.json; then
    ok "Found package.json sonar:scan script"
  else
    info "package.json has no sonar:scan script"
  fi
  if grep -q '"test:coverage"' package.json; then
    ok "Found package.json test:coverage script"
  else
    info "package.json has no test:coverage script"
  fi
else
  info "package.json not found"
fi

if [ -f scripts/compose/phase3-sonar-result.sh ]; then
  if [ -x scripts/compose/phase3-sonar-result.sh ]; then
    ok "Found executable Phase 3 Sonar wrapper"
  else
    ok "Found Phase 3 Sonar wrapper"
  fi
else
  info "Phase 3 Sonar wrapper not found"
fi

print_section "Recommended next step"
if ! has_command sonar-scanner; then
  warn "Install SonarScanner CLI before using repo commands that call sonar-scanner."
  if has_command brew; then
    info "macOS/Homebrew option: brew install sonar-scanner"
  fi
  if has_command docker; then
    info "Docker scanner option: docker run --rm -e SONAR_HOST_URL -e SONAR_TOKEN -v \"\$PWD:/usr/src\" sonarsource/sonar-scanner-cli"
    info "If SonarQube runs on the macOS host, use SONAR_HOST_URL=http://host.docker.internal:9000 for the scanner container."
  fi
elif [ -z "${SONAR_HOST_URL:-}" ]; then
  warn "Set SONAR_HOST_URL before scanning, for example: export SONAR_HOST_URL=http://localhost:9000"
elif [ -z "${SONAR_TOKEN:-}" ]; then
  warn "Set SONAR_TOKEN before scanning. Generate it in SonarQube and export it without printing the value."
elif [ -f package.json ] && grep -q '"sonar:scan"' package.json && has_command pnpm; then
  ok "Ready for normal local scan: pnpm sonar:scan"
  if [ -f scripts/compose/phase3-sonar-result.sh ]; then
    info "For Phase 3 evidence: scripts/compose/phase3-sonar-result.sh"
  fi
else
  ok "Ready to run scanner from the project base directory: sonar-scanner"
fi

if [ -z "${SONAR_HOST_URL:-}" ] && has_command docker; then
  info "Local SonarQube evaluation server option: docker run -d --name sonarqube -e SONAR_ES_BOOTSTRAP_CHECKS_DISABLE=true -p 9000:9000 sonarqube:latest"
fi
