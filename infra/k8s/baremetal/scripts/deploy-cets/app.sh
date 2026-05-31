# shellcheck shell=bash

deploy_app() {
log "creating migration and seed jobs"
kubectl_bm -n "$CETS_NAMESPACE" delete job cets-migrate cets-seed --ignore-not-found
cat >"$GENERATED_DIR/cets-jobs.yaml" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: cets-migrate
  namespace: $CETS_NAMESPACE
spec:
  backoffLimit: 3
  template:
    spec:
$(image_pull_block)
      restartPolicy: Never
      containers:
      - name: migrate
        image: $CETS_API_IMAGE
        imagePullPolicy: IfNotPresent
        args: ["migrate"]
        envFrom:
        - configMapRef:
            name: cets-config
        - secretRef:
            name: cets-runtime-env
---
apiVersion: batch/v1
kind: Job
metadata:
  name: cets-seed
  namespace: $CETS_NAMESPACE
spec:
  backoffLimit: 3
  template:
    spec:
$(image_pull_block)
      restartPolicy: Never
      containers:
      - name: seed
        image: $CETS_API_IMAGE
        imagePullPolicy: IfNotPresent
        args: ["seed"]
        envFrom:
        - configMapRef:
            name: cets-config
        - secretRef:
            name: cets-runtime-env
EOF

cat >"$GENERATED_DIR/cets-app.yaml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: cets-config
  namespace: $CETS_NAMESPACE
data:
  APP_ADDR: ":8080"
  APP_ENV: "baremetal"
  AUTO_MIGRATE: "false"
  OPS_API_ENABLED: "false"
  POSTGRES_DB: "$POSTGRES_DB"
  REDIS_URL: "redis://redis:6379/0"
  QUEUE_URL: "redis://redis:6379/1"
  OBJECT_STORAGE_ENDPOINT: "http://minio:9000"
  OBJECT_STORAGE_REGION: "us-east-1"
  OBJECT_STORAGE_BUCKET: "$OBJECT_STORAGE_BUCKET"
  MAILER_HOST: "mailhog"
  MAILER_PORT: "1025"
  MAILER_FROM: "$MAILER_FROM"
  MAILER_REDIRECT_TO: "$MAILER_REDIRECT_TO"
  REQUEST_TIMEOUT_MS: "5000"
  DATABASE_TIMEOUT_MS: "5000"
  SHUTDOWN_TIMEOUT_MS: "10000"
  BOOKING_PREADMISSION: "on"
  REDIS_OUTAGE_MODE: "degrade"
  OTEL_TRACES_ENABLED: "true"
  OTEL_EXPORTER_OTLP_ENDPOINT: "http://alloy.observability.svc.cluster.local:4318"
  PYROSCOPE_ENABLED: "true"
  PYROSCOPE_SERVER_ADDRESS: "http://pyroscope.observability.svc.cluster.local:4040"
---
apiVersion: v1
kind: Secret
metadata:
  name: cets-runtime-env
  namespace: $CETS_NAMESPACE
type: Opaque
stringData:
  DATABASE_URL: "postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@cets-postgres-rw:5432/$POSTGRES_DB"
  DATABASE_WRITE_URL: "postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@cets-postgres-rw:5432/$POSTGRES_DB"
  DATABASE_READ_URL: "postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@cets-postgres-ro:5432/$POSTGRES_DB"
  TOKEN_SIGNING_SECRET: "$TOKEN_SIGNING_SECRET"
  PROVIDER_TOKEN_SECRET: "$PROVIDER_TOKEN_SECRET"
  BOOKING_RESERVATION_HASH_SECRET: "$BOOKING_RESERVATION_HASH_SECRET"
  OBJECT_STORAGE_ACCESS_KEY: "$OBJECT_STORAGE_ACCESS_KEY"
  OBJECT_STORAGE_SECRET_KEY: "$OBJECT_STORAGE_SECRET_KEY"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: $CETS_NAMESPACE
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 1
  selector:
    matchLabels:
      app: backend
  template:
    metadata:
      labels:
        app: backend
    spec:
$(image_pull_block)
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: backend
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: backend
            topologyKey: kubernetes.io/hostname
      containers:
      - name: backend
        image: $CETS_API_IMAGE
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8080
        env:
        - name: OTEL_SERVICE_NAME
          value: cets-backend
        - name: PYROSCOPE_APPLICATION_NAME
          value: cets-backend
        envFrom:
        - configMapRef:
            name: cets-config
        - secretRef:
            name: cets-runtime-env
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 20
          periodSeconds: 20
---
apiVersion: v1
kind: Service
metadata:
  name: backend
  namespace: $CETS_NAMESPACE
spec:
  selector:
    app: backend
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: frontend-nginx
  namespace: $CETS_NAMESPACE
data:
  default.conf: |
    upstream cets_backend {
      server backend.$CETS_NAMESPACE.svc.cluster.local:8080 max_fails=1 fail_timeout=5s;
    }

    server {
      listen 8080;
      server_name _;

      root /usr/share/nginx/html;
      index index.html;

      location = /healthz {
        access_log off;
        add_header Content-Type text/plain;
        return 200 "ok\n";
      }

      location /api/ {
        proxy_pass http://cets_backend;
        proxy_next_upstream error timeout http_502 http_503 http_504;
        proxy_next_upstream_tries 3;
        proxy_next_upstream_timeout 3s;
        proxy_connect_timeout 1s;
        proxy_send_timeout 10s;
        proxy_read_timeout 10s;
        proxy_set_header Host \$host;
        proxy_set_header X-Request-ID \$request_id;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header traceparent \$http_traceparent;
        proxy_set_header tracestate \$http_tracestate;
        proxy_set_header baggage \$http_baggage;
      }

      location /readyz {
        proxy_pass http://cets_backend;
        proxy_next_upstream error timeout http_502 http_503 http_504;
        proxy_next_upstream_tries 3;
        proxy_next_upstream_timeout 3s;
        proxy_connect_timeout 1s;
        proxy_send_timeout 10s;
        proxy_read_timeout 10s;
        proxy_set_header Host \$host;
        proxy_set_header X-Request-ID \$request_id;
        proxy_set_header traceparent \$http_traceparent;
        proxy_set_header tracestate \$http_tracestate;
        proxy_set_header baggage \$http_baggage;
      }

      location /metrics {
        proxy_pass http://cets_backend;
        proxy_next_upstream error timeout http_502 http_503 http_504;
        proxy_next_upstream_tries 3;
        proxy_next_upstream_timeout 3s;
        proxy_connect_timeout 1s;
        proxy_send_timeout 10s;
        proxy_read_timeout 10s;
        proxy_set_header Host \$host;
        proxy_set_header X-Request-ID \$request_id;
        proxy_set_header traceparent \$http_traceparent;
        proxy_set_header tracestate \$http_tracestate;
        proxy_set_header baggage \$http_baggage;
      }

      location / {
        try_files \$uri \$uri/ /index.html;
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: $CETS_NAMESPACE
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 1
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
    spec:
$(image_pull_block)
      volumes:
      - name: frontend-nginx
        configMap:
          name: frontend-nginx
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: frontend
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: frontend
            topologyKey: kubernetes.io/hostname
      containers:
      - name: frontend
        image: $CETS_FRONTEND_IMAGE
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8080
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: frontend-nginx
          mountPath: /etc/nginx/conf.d/default.conf
          subPath: default.conf
---
apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: $CETS_NAMESPACE
spec:
  selector:
    app: frontend
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cets
  namespace: $CETS_NAMESPACE
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "10m"
spec:
  ingressClassName: nginx
  rules:
  - host: $CETS_PUBLIC_HOSTNAME
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: backend
            port:
              number: 8080
      - path: /healthz
        pathType: Exact
        backend:
          service:
            name: backend
            port:
              number: 8080
      - path: /readyz
        pathType: Exact
        backend:
          service:
            name: backend
            port:
              number: 8080
      - path: /
        pathType: Prefix
        backend:
          service:
            name: frontend
            port:
              number: 8080
EOF

for kind in notification projection compensation export; do
  cat >>"$GENERATED_DIR/cets-app.yaml" <<EOF
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: worker-$kind
  namespace: $CETS_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: worker-$kind
  template:
    metadata:
      labels:
        app: worker-$kind
    spec:
$(image_pull_block)
      containers:
      - name: worker
        image: $CETS_API_IMAGE
        imagePullPolicy: IfNotPresent
        args: ["worker"]
        env:
        - name: WORKER_KINDS
          value: "$kind"
        - name: OTEL_SERVICE_NAME
          value: cets-worker-$kind
        - name: PYROSCOPE_APPLICATION_NAME
          value: cets-worker-$kind
        envFrom:
        - configMapRef:
            name: cets-config
        - secretRef:
            name: cets-runtime-env
EOF
done

cat >"$GENERATED_DIR/cloudflared.yaml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cloudflared
  namespace: $CETS_NAMESPACE
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 1
  selector:
    matchLabels:
      app: cloudflared
  template:
    metadata:
      labels:
        app: cloudflared
    spec:
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: cloudflared
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: cloudflared
            topologyKey: kubernetes.io/hostname
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
          failureThreshold: 1
          initialDelaySeconds: 10
          periodSeconds: 10
EOF

kubectl_bm apply -f "$GENERATED_DIR/cets-app.yaml"
kubectl_bm apply -f "$GENERATED_DIR/cets-jobs.yaml"
kubectl_bm wait --for=condition=complete job/cets-migrate -n "$CETS_NAMESPACE" --timeout=300s
kubectl_bm wait --for=condition=complete job/cets-seed -n "$CETS_NAMESPACE" --timeout=300s
if kubectl_bm -n "$CETS_NAMESPACE" get secret cloudflared-token >/dev/null 2>&1; then
  kubectl_bm apply -f "$GENERATED_DIR/cloudflared.yaml"
else
  log "cloudflared-token secret not found; skipping cloudflared until 40-cloudflare.sh is applied"
fi
}
