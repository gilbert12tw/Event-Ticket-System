#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd awk
require_cmd grep

should_require_git_tag() {
  [ "${REQUIRE_RELEASE_TAG_AT_HEAD:-true}" = "true" ] || return 1
  [ "${ALLOW_MISSING_RELEASE_TAG:-false}" = "true" ] && return 1
  if [ "${GITHUB_EVENT_NAME:-}" = "pull_request" ] || [ "${GITHUB_REF_TYPE:-}" = "tag" ]; then
    return 1
  fi
  [[ "${GITHUB_REF:-}" == refs/tags/* ]] && return 1
  git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1
}

infer_release_version() {
  if [ -n "$CETS_RELEASE_VERSION" ]; then
    printf '%s\n' "$CETS_RELEASE_VERSION"
    return 0
  fi
  if [ "${GITHUB_REF_TYPE:-}" = "tag" ] && [ -n "${GITHUB_REF_NAME:-}" ]; then
    printf '%s\n' "$GITHUB_REF_NAME"
    return 0
  fi
  if [[ "${GITHUB_REF:-}" == refs/tags/v* ]]; then
    printf '%s\n' "${GITHUB_REF#refs/tags/}"
    return 0
  fi
  awk '
    $1 == "newTag:" {
      if (tag == "") {
        tag = $2
      } else if (tag != $2) {
        mismatch = 1
      }
    }
    END {
      if (mismatch == 1 || tag == "") {
        exit 1
      }
      print tag
    }
  ' "$GITOPS_APP_DIR/kustomization.yaml"
}

validate_semver_tag() {
  local version=$1
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]] ||
    die "release version must look like v0.0.1, got $version"
}

assert_kustomize_tag() {
  local version=$1
  local matches
  matches=$(grep -c "newTag: $version" "$GITOPS_APP_DIR/kustomization.yaml" || true)
  [ "$matches" = "2" ] || die "expected both app images to use newTag: $version"
}

assert_release_tag_at_head() {
  local version=$1
  should_require_git_tag || return 0
  git -C "$ROOT_DIR" fetch --tags --force origin "$version" >/dev/null 2>&1 || true
  if ! git -C "$ROOT_DIR" rev-parse -q --verify "refs/tags/$version" >/dev/null; then
    die "release branch HEAD must be tagged with $version before Argo CD rollout"
  fi
  local head tagged
  head=$(git -C "$ROOT_DIR" rev-parse HEAD)
  tagged=$(git -C "$ROOT_DIR" rev-list -n 1 "$version")
  [ "$head" = "$tagged" ] || die "tag $version points to $tagged, not release branch HEAD $head"
}

assert_argocd_application() {
  grep -q "name: $ARGOCD_APP_NAME" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application name must be $ARGOCD_APP_NAME"
  grep -q "namespace: $ARGOCD_NAMESPACE" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application namespace must be $ARGOCD_NAMESPACE"
  grep -q "project: $ARGOCD_PROJECT" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application project must be $ARGOCD_PROJECT"
  grep -q "repoURL: $ARGOCD_REPO_URL" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD repoURL must be $ARGOCD_REPO_URL"
  grep -q "targetRevision: $ARGOCD_TARGET_REVISION" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD targetRevision must be $ARGOCD_TARGET_REVISION"
  grep -q "path: infra/aws/self-managed-k8s/gitops/app" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application must point at the AWS GitOps app path"
}

assert_gitops_app_env() {
  grep -q 'APP_ENV: "aws-self-managed-k8s"' "$GITOPS_APP_DIR/configmap.yaml" ||
    die "GitOps app config must keep APP_ENV aws-self-managed-k8s; demo/local/test belongs only in temporary script overrides"
}

assert_no_gitops_secrets() {
  if grep -REin '(^[[:space:]]*kind:[[:space:]]*Secret[[:space:]]*$|^[[:space:]]*stringData:)' \
    "$AWS_K8S_DIR/gitops"; then
    die "GitOps manifests must not contain unsealed Kubernetes Secrets"
  fi
  if grep -REin '^[[:space:]]*(database[-_]?url|postgres[-_]?password|token[-_]?signing[-_]?secret|provider[-_]?token[-_]?secret|booking[-_]?reservation[-_]?hash[-_]?secret|object[-_]?storage[-_]?secret[-_]?key|password|private[-_]?key|secret[-_]?key):' \
    "$AWS_K8S_DIR/gitops"; then
    die "GitOps manifests must not contain runtime secrets"
  fi
}

release_version=$(infer_release_version)
validate_semver_tag "$release_version"
assert_kustomize_tag "$release_version"
assert_release_tag_at_head "$release_version"
assert_argocd_application
assert_gitops_app_env
assert_no_gitops_secrets

log "GitOps release manifests are valid for $release_version"
