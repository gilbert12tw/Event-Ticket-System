#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

GITOPS_APP_DIR="$BM_DIR/gitops/app"
GITOPS_ARGOCD_APP="$BM_DIR/gitops/argocd/application.yaml"
ARGOCD_APP_NAME=${ARGOCD_APP_NAME:-cets-baremetal}
ARGOCD_NAMESPACE=${ARGOCD_NAMESPACE:-argocd}
ARGOCD_REPO_URL=${ARGOCD_REPO_URL:-https://github.com/gilbert12tw/Event-Ticket-System.git}
ARGOCD_TARGET_REVISION=${ARGOCD_TARGET_REVISION:-release/baremetal}

load_env
require_cmd awk
require_cmd grep

assert_kustomization_images() {
  local file="$GITOPS_APP_DIR/kustomization.yaml"
  grep -q 'newName: ghcr.io/gilbert12tw/event-ticket-system/cets-api' "$file" ||
    die "cets-api image must use GHCR"
  grep -q 'newName: ghcr.io/gilbert12tw/event-ticket-system/cets-frontend' "$file" ||
    die "cets-frontend image must use GHCR"

  awk '
    $1 == "newTag:" {
      if ($2 !~ /^[0-9a-f]+$/ || length($2) < 7 || length($2) > 40) {
        printf "image tag must be a Git hash, got %s\n", $2 > "/dev/stderr"
        exit 1
      }
      count++
    }
    END {
      if (count != 2) {
        printf "expected exactly 2 image tags, got %d\n", count > "/dev/stderr"
        exit 1
      }
    }
  ' "$file" || die "GitOps images must use explicit Git hash tags"
}

assert_argocd_application() {
  grep -q "name: $ARGOCD_APP_NAME" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application name must be $ARGOCD_APP_NAME"
  grep -q "namespace: $ARGOCD_NAMESPACE" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application namespace must be $ARGOCD_NAMESPACE"
  grep -q "repoURL: $ARGOCD_REPO_URL" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD repoURL must be $ARGOCD_REPO_URL"
  grep -q "targetRevision: $ARGOCD_TARGET_REVISION" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD targetRevision must be $ARGOCD_TARGET_REVISION"
  grep -q "path: infra/k8s/baremetal/gitops/app" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application must point at the baremetal GitOps app path"
  grep -q "automated:" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application must enable automated sync"
  grep -q "prune: true" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application must enable automated prune"
  grep -q "selfHeal: true" "$GITOPS_ARGOCD_APP" ||
    die "Argo CD Application must enable automated self-heal"
}

assert_gitops_config() {
  grep -q 'APP_ENV: "demo"' "$GITOPS_APP_DIR/configmap.yaml" ||
    die "GitOps app config must preserve the current baremetal APP_ENV demo"
  grep -R "name: ghcr-pull" "$GITOPS_APP_DIR" >/dev/null ||
    die "GitOps workloads must reference the cluster-managed ghcr-pull secret"
  grep -R "name: cets-runtime-env" "$GITOPS_APP_DIR" >/dev/null ||
    die "GitOps workloads must reference the cluster-managed runtime secret"
  grep -R "name: cloudflared-token" "$GITOPS_APP_DIR" >/dev/null ||
    die "GitOps cloudflared deployment must reference the cluster-managed tunnel token"
}

assert_no_gitops_secrets() {
  if grep -REin '(^[[:space:]]*kind:[[:space:]]*Secret[[:space:]]*$|^[[:space:]]*stringData:)' \
    "$BM_DIR/gitops"; then
    die "GitOps manifests must not contain unsealed Kubernetes Secrets"
  fi
  if grep -REin '^[[:space:]]*(database[-_]?url|postgres[-_]?password|token[-_]?signing[-_]?secret|provider[-_]?token[-_]?secret|booking[-_]?reservation[-_]?hash[-_]?secret|object[-_]?storage[-_]?secret[-_]?key|tunnel[-_]?token|password|private[-_]?key|secret[-_]?key):' \
    "$BM_DIR/gitops"; then
    die "GitOps manifests must not contain runtime secrets"
  fi
}

assert_kustomize_builds() {
  if command -v kustomize >/dev/null 2>&1; then
    kustomize build "$GITOPS_APP_DIR" >/dev/null
  elif command -v kubectl >/dev/null 2>&1; then
    kubectl kustomize "$GITOPS_APP_DIR" >/dev/null
  else
    log "kustomize and kubectl are unavailable; skipping render check"
  fi
}

assert_kustomization_images
assert_argocd_application
assert_gitops_config
assert_no_gitops_secrets
assert_kustomize_builds

log "baremetal GitOps release manifests are valid"
