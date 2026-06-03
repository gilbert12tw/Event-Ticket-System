#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"
# shellcheck source=deploy-cets/images.sh
. "$SCRIPT_DIR/deploy-cets/images.sh"

load_env
require_cmd git

KUSTOMIZE_BIN=${KUSTOMIZE_BIN:-kustomize}

validate_release_version() {
  [[ "$CETS_RELEASE_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]] ||
    die "CETS_RELEASE_VERSION must look like v0.0.1, got ${CETS_RELEASE_VERSION:-<empty>}"
}

require_clean_index() {
  git -C "$ROOT_DIR" diff --quiet --exit-code ||
    die "worktree has uncommitted changes; commit or stash before preparing a release"
  git -C "$ROOT_DIR" diff --cached --quiet --exit-code ||
    die "index has staged changes; commit or unstage before preparing a release"
}

require_release_branch() {
  local branch
  branch=$(git -C "$ROOT_DIR" branch --show-current)
  if [ "$branch" != "$RELEASE_BRANCH" ] && [ "${ALLOW_NON_RELEASE_BRANCH:-false}" != "true" ]; then
    die "switch to $RELEASE_BRANCH or set ALLOW_NON_RELEASE_BRANCH=true for a dry run"
  fi
}

prepare_release_images() {
  CETS_IMAGE_TAG="$CETS_RELEASE_VERSION"
  if [ "$BUILD_AND_PUSH_IMAGES" = "true" ]; then
    require_cmd aws
    require_aws_identity
    prepare_images
  else
    require_env CETS_API_IMAGE
    require_env CETS_FRONTEND_IMAGE
  fi

  case "$CETS_API_IMAGE" in
    *":$CETS_RELEASE_VERSION") ;;
    *) die "CETS_API_IMAGE must end with :$CETS_RELEASE_VERSION" ;;
  esac
  case "$CETS_FRONTEND_IMAGE" in
    *":$CETS_RELEASE_VERSION") ;;
    *) die "CETS_FRONTEND_IMAGE must end with :$CETS_RELEASE_VERSION" ;;
  esac
}

validate_release_version
require_clean_index
require_release_branch
require_cmd "$KUSTOMIZE_BIN"
prepare_release_images

(
  cd "$GITOPS_APP_DIR"
  "$KUSTOMIZE_BIN" edit set image \
    "cets-api=$CETS_API_IMAGE" \
    "cets-frontend=$CETS_FRONTEND_IMAGE"
)

ALLOW_MISSING_RELEASE_TAG=true "$SCRIPT_DIR/82-verify-release-gitops.sh"

log "prepared GitOps release $CETS_RELEASE_VERSION on $RELEASE_BRANCH"
log "next: git add infra/aws/self-managed-k8s/gitops/app/kustomization.yaml"
log "next: git commit -m 'release(aws-k8s): $CETS_RELEASE_VERSION'"
log "next: git tag -a '$CETS_RELEASE_VERSION' -m '$CETS_RELEASE_VERSION'"
log "next: infra/aws/self-managed-k8s/scripts/82-verify-release-gitops.sh"
log "next: git push origin '$RELEASE_BRANCH' '$CETS_RELEASE_VERSION'"
