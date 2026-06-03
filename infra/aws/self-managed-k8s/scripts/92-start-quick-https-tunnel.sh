#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd kubectl
require_cmd curl
require_cmd grep

[ "${QUICK_HTTPS_PUBLIC_EXPOSURE_ACK:-false}" = "true" ] ||
  die "set QUICK_HTTPS_PUBLIC_EXPOSURE_ACK=true to acknowledge this creates a public temporary HTTPS URL"

KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"
[ -f "$KUBECONFIG_AWS" ] || die "missing kubeconfig; run 40-bootstrap-k8s.sh first"
export KUBECONFIG="$KUBECONFIG_AWS"

PROXY_NAME=${QUICK_HTTPS_PROXY_NAME:-cets-https-preview-proxy}
TUNNEL_NAME=${QUICK_HTTPS_TUNNEL_NAME:-cets-quick-https-tunnel}
PROXY_IMAGE=${QUICK_HTTPS_PROXY_IMAGE:-nginxinc/nginx-unprivileged:1.29-alpine}
CLOUDFLARED_IMAGE=${QUICK_HTTPS_CLOUDFLARED_IMAGE:-cloudflare/cloudflared:2026.5.0}
ORIGIN_URL="http://$PROXY_NAME.$CETS_NAMESPACE.svc.cluster.local:8080"
STARTED_AT=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

cleanup_quick_https_resources() {
  kubectl -n "$CETS_NAMESPACE" delete deployment "$TUNNEL_NAME" "$PROXY_NAME" --ignore-not-found >/dev/null 2>&1 || true
  kubectl -n "$CETS_NAMESPACE" delete service "$PROXY_NAME" --ignore-not-found >/dev/null 2>&1 || true
  kubectl -n "$CETS_NAMESPACE" delete configmap "$PROXY_NAME" --ignore-not-found >/dev/null 2>&1 || true
  rm -f "$GENERATED_DIR/quick-https-url.txt"
}

cleanup_on_error() {
  local status=$?
  if [ "$status" -ne 0 ] && [ "${KEEP_FAILED_QUICK_HTTPS_RESOURCES:-false}" != "true" ]; then
    log "cleaning up temporary HTTPS resources after failure"
    cleanup_quick_https_resources
  fi
}

trap cleanup_on_error EXIT

reset_previous_tunnel() {
  kubectl -n "$CETS_NAMESPACE" delete deployment "$TUNNEL_NAME" --ignore-not-found --wait=true
}

extract_quick_tunnel_url() {
  local deadline logs pod url
  pod=$1
  deadline=$((SECONDS + 120))
  while [ "$SECONDS" -lt "$deadline" ]; do
    logs=$(kubectl -n "$CETS_NAMESPACE" logs "$pod" --since-time="$STARTED_AT" 2>/dev/null || true)
    url=$(printf '%s\n' "$logs" | grep -Eo 'https://[-A-Za-z0-9]+\.trycloudflare\.com' | tail -n 1 || true)
    if [ -n "$url" ]; then
      printf '%s\n' "$url"
      return 0
    fi
    sleep 2
  done
  return 1
}

current_tunnel_pod() {
  kubectl -n "$CETS_NAMESPACE" get pods -l "app=$TUNNEL_NAME" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | tail -n 1
}

verify_quick_tunnel_url() {
  local url=$1
  local deadline
  deadline=$((SECONDS + 90))
  while [ "$SECONDS" -lt "$deadline" ]; do
    if curl -fsS "$url/healthz" >/dev/null &&
      curl -fsS "$url/api/v1/auth/bootstrap" | grep -q '"success":true'; then
      return 0
    fi
    sleep 3
  done
  return 1
}

log "deploying temporary HTTPS preview proxy in namespace $CETS_NAMESPACE"
reset_previous_tunnel
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $PROXY_NAME
  namespace: $CETS_NAMESPACE
data:
  default.conf: |
    server {
      listen 8080;
      server_name _;

      proxy_http_version 1.1;
      proxy_set_header Host \$host;
      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Host \$host;
      proxy_set_header X-Forwarded-Proto https;
      proxy_set_header X-Real-IP \$remote_addr;

      location = /healthz {
        proxy_pass http://backend.$CETS_NAMESPACE.svc.cluster.local:8080/healthz;
      }

      location = /readyz {
        proxy_pass http://backend.$CETS_NAMESPACE.svc.cluster.local:8080/readyz;
      }

      location ^~ /api {
        proxy_pass http://backend.$CETS_NAMESPACE.svc.cluster.local:8080;
      }

      location / {
        proxy_pass http://frontend.$CETS_NAMESPACE.svc.cluster.local:8080;
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: $PROXY_NAME
  namespace: $CETS_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: $PROXY_NAME
  template:
    metadata:
      labels:
        app: $PROXY_NAME
    spec:
      containers:
      - name: proxy
        image: $PROXY_IMAGE
        ports:
        - name: http
          containerPort: 8080
        resources:
          requests:
            cpu: 25m
            memory: 64Mi
          limits:
            memory: 128Mi
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/conf.d/default.conf
          subPath: default.conf
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: $PROXY_NAME
---
apiVersion: v1
kind: Service
metadata:
  name: $PROXY_NAME
  namespace: $CETS_NAMESPACE
spec:
  selector:
    app: $PROXY_NAME
  ports:
  - name: http
    port: 8080
    targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: $TUNNEL_NAME
  namespace: $CETS_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: $TUNNEL_NAME
  template:
    metadata:
      labels:
        app: $TUNNEL_NAME
    spec:
      containers:
      - name: cloudflared
        image: $CLOUDFLARED_IMAGE
        args:
        - tunnel
        - --no-autoupdate
        - --loglevel
        - info
        - --metrics
        - 0.0.0.0:2000
        - --url
        - $ORIGIN_URL
        ports:
        - name: metrics
          containerPort: 2000
        resources:
          requests:
            cpu: 25m
            memory: 64Mi
          limits:
            memory: 128Mi
EOF

kubectl rollout status "deployment/$PROXY_NAME" -n "$CETS_NAMESPACE" --timeout=180s
kubectl rollout status "deployment/$TUNNEL_NAME" -n "$CETS_NAMESPACE" --timeout=180s
tunnel_pod=$(current_tunnel_pod)
[ -n "$tunnel_pod" ] || die "could not find current $TUNNEL_NAME pod"

log "waiting for Cloudflare quick tunnel URL"
quick_url=$(extract_quick_tunnel_url "$tunnel_pod") ||
  die "could not find trycloudflare.com URL in cloudflared logs"

log "verifying $quick_url"
verify_quick_tunnel_url "$quick_url" ||
  die "quick tunnel URL was created but app health/auth verification did not pass"

printf '%s\n' "$quick_url" >"$GENERATED_DIR/quick-https-url.txt"
log "temporary HTTPS URL: $quick_url"
printf '%s\n' "$quick_url"
