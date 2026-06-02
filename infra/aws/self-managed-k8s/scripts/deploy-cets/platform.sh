# shellcheck shell=bash

postgres_sync_block() {
  if [ "$NODE_COUNT" = "3" ]; then
    cat <<'YAML'
  postgresql:
    synchronous:
      method: any
      number: 1
      dataDurability: required
  affinity:
    enablePodAntiAffinity: true
    topologyKey: kubernetes.io/hostname
YAML
  fi
}

deploy_platform() {
  log "installing storage and CloudNativePG"
  kubectl create namespace "$CETS_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.36/deploy/local-path-storage.yaml
  kubectl annotate storageclass local-path storageclass.kubernetes.io/is-default-class=true --overwrite || true
  helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null
  helm repo update >/dev/null
  helm upgrade --install cloudnative-pg cnpg/cloudnative-pg \
    --namespace cnpg-system \
    --create-namespace \
    --version 0.28.2
  kubectl rollout status deployment/cloudnative-pg -n cnpg-system --timeout=180s

  kubectl -n "$CETS_NAMESPACE" create secret generic cets-app-secrets \
    --from-literal=postgres-user="$POSTGRES_USER" \
    --from-literal=postgres-password="$POSTGRES_PASSWORD" \
    --from-literal=token-signing-secret="$TOKEN_SIGNING_SECRET" \
    --from-literal=provider-token-secret="$PROVIDER_TOKEN_SECRET" \
    --from-literal=reservation-hash-secret="$BOOKING_RESERVATION_HASH_SECRET" \
    --from-literal=object-storage-access-key="$OBJECT_STORAGE_ACCESS_KEY" \
    --from-literal=object-storage-secret-key="$OBJECT_STORAGE_SECRET_KEY" \
    --dry-run=client -o yaml | kubectl apply -f -

  kubectl -n "$CETS_NAMESPACE" create secret generic cets-pg-app \
    --type=kubernetes.io/basic-auth \
    --from-literal=username="$POSTGRES_USER" \
    --from-literal=password="$POSTGRES_PASSWORD" \
    --dry-run=client -o yaml | kubectl apply -f -

  cat >"$GENERATED_DIR/cets-platform.yaml" <<EOF
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: cets-postgres
  namespace: $CETS_NAMESPACE
spec:
  instances: $NODE_COUNT
  imageName: ghcr.io/cloudnative-pg/postgresql:16.6
$(postgres_sync_block)
  storage:
    size: 20Gi
  bootstrap:
    initdb:
      database: $POSTGRES_DB
      owner: $POSTGRES_USER
      secret:
        name: cets-pg-app
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: $CETS_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7.4.8-alpine
        args: ["redis-server", "--appendonly", "yes"]
        ports:
        - containerPort: 6379
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: $CETS_NAMESPACE
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
  namespace: $CETS_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
      - name: minio
        image: minio/minio:RELEASE.2025-09-07T16-13-09Z
        args: ["server", "/data", "--console-address", ":9001"]
        env:
        - name: MINIO_ROOT_USER
          valueFrom:
            secretKeyRef:
              name: cets-app-secrets
              key: object-storage-access-key
        - name: MINIO_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: cets-app-secrets
              key: object-storage-secret-key
        ports:
        - containerPort: 9000
        - containerPort: 9001
---
apiVersion: v1
kind: Service
metadata:
  name: minio
  namespace: $CETS_NAMESPACE
spec:
  selector:
    app: minio
  ports:
  - name: api
    port: 9000
    targetPort: 9000
  - name: console
    port: 9001
    targetPort: 9001
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mailhog
  namespace: $CETS_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mailhog
  template:
    metadata:
      labels:
        app: mailhog
    spec:
      containers:
      - name: mailhog
        image: mailhog/mailhog:v1.0.1
        ports:
        - containerPort: 1025
        - containerPort: 8025
---
apiVersion: v1
kind: Service
metadata:
  name: mailhog
  namespace: $CETS_NAMESPACE
spec:
  selector:
    app: mailhog
  ports:
  - name: smtp
    port: 1025
    targetPort: 1025
  - name: ui
    port: 8025
    targetPort: 8025
EOF

  kubectl apply -f "$GENERATED_DIR/cets-platform.yaml"
  kubectl wait --for=condition=Ready cluster/cets-postgres -n "$CETS_NAMESPACE" --timeout=600s
}
