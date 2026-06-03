# shellcheck shell=bash

git_image_tag() {
  require_cmd git
  if [ "${ALLOW_DIRTY_IMAGE_TAG:-false}" != "true" ]; then
    git -C "$ROOT_DIR" diff --quiet --exit-code ||
      die "worktree has uncommitted changes; commit first or set ALLOW_DIRTY_IMAGE_TAG=true"
    git -C "$ROOT_DIR" diff --cached --quiet --exit-code ||
      die "index has staged changes; commit first or set ALLOW_DIRTY_IMAGE_TAG=true"
  fi
  git -C "$ROOT_DIR" rev-parse --short=12 HEAD
}

prepare_images() {
  local image_tag api_repo frontend_repo registry
  image_tag=${CETS_IMAGE_TAG:-$(git_image_tag)}
  api_repo=$(stack_output "$CLUSTER_STACK_NAME" ApiRepositoryUri)
  frontend_repo=$(stack_output "$CLUSTER_STACK_NAME" FrontendRepositoryUri)

  if [ "$BUILD_AND_PUSH_IMAGES" = "true" ]; then
    require_cmd docker
    registry=${api_repo%%/*}
    aws_cli ecr get-login-password | docker login --username AWS --password-stdin "$registry"
    CETS_API_IMAGE="$api_repo:$image_tag"
    CETS_FRONTEND_IMAGE="$frontend_repo:$image_tag"
    log "building and pushing $CETS_API_IMAGE for $CETS_IMAGE_PLATFORM"
    docker build --platform "$CETS_IMAGE_PLATFORM" \
      -t "$CETS_API_IMAGE" \
      -f "$ROOT_DIR/services/api/Dockerfile" \
      "$ROOT_DIR"
    docker push "$CETS_API_IMAGE"
    log "building and pushing $CETS_FRONTEND_IMAGE for $CETS_IMAGE_PLATFORM"
    docker build --platform "$CETS_IMAGE_PLATFORM" \
      -t "$CETS_FRONTEND_IMAGE" \
      -f "$ROOT_DIR/apps/web/Dockerfile" \
      "$ROOT_DIR"
    docker push "$CETS_FRONTEND_IMAGE"
  else
    require_env CETS_API_IMAGE
    require_env CETS_FRONTEND_IMAGE
  fi
}
