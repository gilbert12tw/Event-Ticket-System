# shellcheck shell=bash

deploy_cloudflare() {
  [ "$APP_INGRESS_MODE" = "cloudflare" ] || return 0

  local app_replicas=$NODE_COUNT
  kubectl -n "$CETS_NAMESPACE" create secret generic cloudflared-token \
    --from-literal=tunnel-token="$CLOUDFLARE_TUNNEL_TOKEN" \
    --dry-run=client -o yaml | kubectl apply -f -

  cat >"$GENERATED_DIR/cloudflared.yaml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cloudflared
  namespace: $CETS_NAMESPACE
spec:
  replicas: $app_replicas
  selector:
    matchLabels:
      app: cloudflared
  template:
    metadata:
      labels:
        app: cloudflared
    spec:
      containers:
      - name: cloudflared
        image: cloudflare/cloudflared:2026.5.0
        args: ["tunnel", "--no-autoupdate", "--loglevel", "info", "--metrics", "0.0.0.0:2000", "run"]
        ports:
        - name: metrics
          containerPort: 2000
        env:
        - name: TUNNEL_TOKEN
          valueFrom:
            secretKeyRef:
              name: cloudflared-token
              key: tunnel-token
        livenessProbe:
          httpGet:
            path: /ready
            port: 2000
          initialDelaySeconds: 10
          periodSeconds: 10
EOF

  kubectl apply -f "$GENERATED_DIR/cloudflared.yaml"
  kubectl rollout status deployment/cloudflared -n "$CETS_NAMESPACE" --timeout=300s
}
