#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

ARGOCD_VERSION=${ARGOCD_VERSION:-v3.4.2}
ARGOCD_NAMESPACE=${ARGOCD_NAMESPACE:-argocd}
ARGOCD_INSTALL_URL="https://raw.githubusercontent.com/argoproj/argo-cd/${ARGOCD_VERSION}/manifests/install.yaml"
ARGOCD_APP_MANIFEST="$BM_DIR/gitops/argocd/application.yaml"

load_env
require_apply
require_cmd kubectl

create_runtime_secrets() {
  require_env POSTGRES_PASSWORD
  require_env TOKEN_SIGNING_SECRET
  require_env PROVIDER_TOKEN_SECRET
  require_env BOOKING_RESERVATION_HASH_SECRET
  require_env OBJECT_STORAGE_SECRET_KEY

  kubectl_bm create namespace "$CETS_NAMESPACE" --dry-run=client -o yaml | kubectl_bm apply -f -

  kubectl_bm -n "$CETS_NAMESPACE" create secret generic cets-app-secrets \
    --from-literal=postgres-user="${POSTGRES_USER:-cets}" \
    --from-literal=postgres-password="$POSTGRES_PASSWORD" \
    --from-literal=token-signing-secret="$TOKEN_SIGNING_SECRET" \
    --from-literal=provider-token-secret="$PROVIDER_TOKEN_SECRET" \
    --from-literal=reservation-hash-secret="$BOOKING_RESERVATION_HASH_SECRET" \
    --from-literal=object-storage-access-key="${OBJECT_STORAGE_ACCESS_KEY:-minioadmin}" \
    --from-literal=object-storage-secret-key="$OBJECT_STORAGE_SECRET_KEY" \
    --dry-run=client -o yaml | kubectl_bm apply -f -

  kubectl_bm -n "$CETS_NAMESPACE" create secret generic cets-pg-app \
    --type=kubernetes.io/basic-auth \
    --from-literal=username="${POSTGRES_USER:-cets}" \
    --from-literal=password="$POSTGRES_PASSWORD" \
    --dry-run=client -o yaml | kubectl_bm apply -f -

  kubectl_bm -n "$CETS_NAMESPACE" create secret generic cets-runtime-env \
    --from-literal="DATABASE_URL=postgresql://${POSTGRES_USER:-cets}:$POSTGRES_PASSWORD@cets-postgres-rw:5432/${POSTGRES_DB:-cets}?pool_max_conns=${DATABASE_POOL_MAX_CONNS:-8}" \
    --from-literal="DATABASE_WRITE_URL=postgresql://${POSTGRES_USER:-cets}:$POSTGRES_PASSWORD@cets-postgres-rw:5432/${POSTGRES_DB:-cets}?pool_max_conns=${DATABASE_POOL_MAX_CONNS:-8}" \
    --from-literal="DATABASE_READ_URL=postgresql://${POSTGRES_USER:-cets}:$POSTGRES_PASSWORD@cets-postgres-ro:5432/${POSTGRES_DB:-cets}?pool_max_conns=${DATABASE_POOL_MAX_CONNS:-8}" \
    --from-literal=TOKEN_SIGNING_SECRET="$TOKEN_SIGNING_SECRET" \
    --from-literal=PROVIDER_TOKEN_SECRET="$PROVIDER_TOKEN_SECRET" \
    --from-literal=BOOKING_RESERVATION_HASH_SECRET="$BOOKING_RESERVATION_HASH_SECRET" \
    --from-literal=OBJECT_STORAGE_ACCESS_KEY="${OBJECT_STORAGE_ACCESS_KEY:-minioadmin}" \
    --from-literal=OBJECT_STORAGE_SECRET_KEY="$OBJECT_STORAGE_SECRET_KEY" \
    --dry-run=client -o yaml | kubectl_bm apply -f -

  if [ -n "${CLOUDFLARE_TUNNEL_TOKEN:-}" ]; then
    kubectl_bm -n "$CETS_NAMESPACE" create secret generic cloudflared-token \
      --from-literal=tunnel-token="$CLOUDFLARE_TUNNEL_TOKEN" \
      --dry-run=client -o yaml | kubectl_bm apply -f -
  elif ! kubectl_bm -n "$CETS_NAMESPACE" get secret cloudflared-token >/dev/null 2>&1; then
    die "cloudflared-token is missing; run 40-cloudflare.sh or set CLOUDFLARE_TUNNEL_TOKEN"
  fi

  if [ -n "${GHCR_USERNAME:-}" ] && [ -n "${GHCR_TOKEN:-}" ]; then
    kubectl_bm -n "$CETS_NAMESPACE" create secret docker-registry ghcr-pull \
      --docker-server=ghcr.io \
      --docker-username="$GHCR_USERNAME" \
      --docker-password="$GHCR_TOKEN" \
      --dry-run=client -o yaml | kubectl_bm apply -f -
  elif ! kubectl_bm -n "$CETS_NAMESPACE" get secret ghcr-pull >/dev/null 2>&1; then
    die "ghcr-pull is missing; set GHCR_USERNAME and GHCR_TOKEN in $LOCAL_ENV"
  fi
}

log "installing Argo CD $ARGOCD_VERSION"
kubectl_bm create namespace "$ARGOCD_NAMESPACE" --dry-run=client -o yaml | kubectl_bm apply -f -
kubectl_bm apply -n "$ARGOCD_NAMESPACE" -f "$ARGOCD_INSTALL_URL"
kubectl_bm -n "$ARGOCD_NAMESPACE" rollout status deployment/argocd-redis --timeout=300s
kubectl_bm -n "$ARGOCD_NAMESPACE" rollout status deployment/argocd-repo-server --timeout=300s
kubectl_bm -n "$ARGOCD_NAMESPACE" rollout status deployment/argocd-server --timeout=300s
kubectl_bm -n "$ARGOCD_NAMESPACE" rollout status statefulset/argocd-application-controller --timeout=300s

log "creating cluster-managed secrets used by GitOps manifests"
create_runtime_secrets

log "applying Argo CD Application cets-baremetal"
kubectl_bm apply -f "$ARGOCD_APP_MANIFEST"
kubectl_bm -n "$ARGOCD_NAMESPACE" get application cets-baremetal >/dev/null
log "Argo CD is watching release/baremetal for the baremetal GitOps app"
