# shellcheck shell=bash

append_worker_deployments() {
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
      imagePullSecrets:
      - name: cets-ecr-pull
      containers:
      - name: worker
        image: $api_deploy_image
        imagePullPolicy: IfNotPresent
        args: ["worker"]
        env:
        - name: WORKER_KINDS
          value: "$kind"
        envFrom:
        - configMapRef:
            name: cets-config
        - secretRef:
            name: cets-runtime-env
EOF
  done
}

url_encode() {
  jq -nr --arg value "$1" '$value|@uri'
}

database_url() {
  local host=$1
  printf 'postgresql://%s:%s@%s:5432/%s' \
    "$(url_encode "$POSTGRES_USER")" \
    "$(url_encode "$POSTGRES_PASSWORD")" \
    "$host" \
    "$(url_encode "$POSTGRES_DB")"
}

ecr_registry_from_image() {
  local image=$1
  local registry
  registry=${image%%/*}
  case "$registry" in
    *.dkr.ecr.*.amazonaws.com) printf '%s' "$registry" ;;
  esac
}

create_ecr_pull_secret() {
  local api_registry auth frontend_registry password registry tmpfile
  api_registry=$(ecr_registry_from_image "$CETS_API_IMAGE")
  frontend_registry=$(ecr_registry_from_image "$CETS_FRONTEND_IMAGE")
  if [ -n "$api_registry" ] && [ -n "$frontend_registry" ] && [ "$api_registry" != "$frontend_registry" ]; then
    die "CETS_API_IMAGE and CETS_FRONTEND_IMAGE must use the same ECR registry when both are private ECR images"
  fi
  registry=${api_registry:-$frontend_registry}
  if [ -z "$registry" ]; then
    log "skipping ECR pull secret because neither app image uses private ECR"
    return 0
  fi

  password=$(aws_cli ecr get-login-password)
  auth=$(printf 'AWS:%s' "$password" | base64 | tr -d '\n')
  tmpfile=$(mktemp "${TMPDIR:-/tmp}/cets-ecr-dockerconfig.XXXXXX")
  chmod 600 "$tmpfile"
  if ! jq -n \
    --arg registry "$registry" \
    --arg user AWS \
    --arg pass "$password" \
    --arg auth "$auth" \
    '{auths: {($registry): {username: $user, password: $pass, auth: $auth}}}' >"$tmpfile"; then
    rm -f "$tmpfile"
    return 1
  fi

  if ! kubectl -n "$CETS_NAMESPACE" create secret generic cets-ecr-pull \
    --type=kubernetes.io/dockerconfigjson \
    --from-file=.dockerconfigjson="$tmpfile" \
    --dry-run=client -o yaml | kubectl apply -f -; then
    rm -f "$tmpfile"
    return 1
  fi
  rm -f "$tmpfile"
}

ecr_repository_name_from_image() {
  local image=$1
  local without_registry without_digest without_tag
  without_registry=${image#*/}
  without_digest=${without_registry%@*}
  without_tag=${without_digest%:*}
  printf '%s' "$without_tag"
}

ecr_tag_from_image() {
  local image=$1
  local without_digest
  without_digest=${image%@*}
  case "$without_digest" in
    *:*) printf '%s' "${without_digest##*:}" ;;
    *) return 1 ;;
  esac
}

resolve_ecr_image_digest_ref() {
  local image=$1
  local registry repository tag digest
  registry=${image%%/*}
  case "$registry" in
    *.dkr.ecr.*.amazonaws.com) ;;
    *)
      printf '%s' "$image"
      return 0
      ;;
  esac
  case "$image" in
    *@sha256:*)
      printf '%s' "$image"
      return 0
      ;;
  esac

  repository=$(ecr_repository_name_from_image "$image")
  tag=$(ecr_tag_from_image "$image") || die "ECR image $image must include a tag"
  digest=$(aws_cli ecr describe-images \
    --repository-name "$repository" \
    --image-ids "imageTag=$tag" \
    --query 'imageDetails[0].imageDigest' \
    --output text)
  [ -n "$digest" ] && [ "$digest" != "None" ] || die "could not resolve ECR digest for $image"
  printf '%s@%s' "$image" "$digest"
}

private_image_deployments() {
  printf '%s\n' \
    backend \
    frontend \
    worker-notification \
    worker-projection \
    worker-compensation \
    worker-export
}

restart_private_image_deployments_if_requested() {
  local deployment
  [ "$BUILD_AND_PUSH_IMAGES" = "true" ] || [ "$FORCE_PRIVATE_IMAGE_ROLLOUT" = "true" ] || return 0
  while IFS= read -r deployment; do
    kubectl -n "$CETS_NAMESPACE" rollout restart "deployment/$deployment"
  done <<EOF
$(private_image_deployments)
EOF
}

wait_private_image_deployments() {
  local deployment
  while IFS= read -r deployment; do
    kubectl rollout status "deployment/$deployment" -n "$CETS_NAMESPACE" --timeout=300s
  done <<EOF
$(private_image_deployments)
EOF
}

deploy_app() {
  local app_replicas=$NODE_COUNT
  local db_rw_url db_ro_url api_deploy_image frontend_deploy_image
  db_rw_url=$(database_url cets-postgres-rw)
  db_ro_url=$(database_url cets-postgres-ro)
  api_deploy_image=$(resolve_ecr_image_digest_ref "$CETS_API_IMAGE")
  frontend_deploy_image=$(resolve_ecr_image_digest_ref "$CETS_FRONTEND_IMAGE")
  create_ecr_pull_secret
  kubectl -n "$CETS_NAMESPACE" delete job minio-init cets-migrate cets-seed --ignore-not-found
  kubectl -n "$CETS_NAMESPACE" create secret generic cets-runtime-env \
    --from-literal=DATABASE_URL="$db_rw_url" \
    --from-literal=DATABASE_WRITE_URL="$db_rw_url" \
    --from-literal=DATABASE_READ_URL="$db_ro_url" \
    --from-literal=TOKEN_SIGNING_SECRET="$TOKEN_SIGNING_SECRET" \
    --from-literal=PROVIDER_TOKEN_SECRET="$PROVIDER_TOKEN_SECRET" \
    --from-literal=BOOKING_RESERVATION_HASH_SECRET="$BOOKING_RESERVATION_HASH_SECRET" \
    --from-literal=OBJECT_STORAGE_ACCESS_KEY="$OBJECT_STORAGE_ACCESS_KEY" \
    --from-literal=OBJECT_STORAGE_SECRET_KEY="$OBJECT_STORAGE_SECRET_KEY" \
    --dry-run=client -o yaml | kubectl apply -f -

  cat >"$GENERATED_DIR/cets-app.yaml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: cets-config
  namespace: $CETS_NAMESPACE
data:
  APP_ADDR: ":8080"
  APP_ENV: "$CETS_APP_ENV"
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
  BOOKING_PREADMISSION: "on"
  REDIS_OUTAGE_MODE: "degrade"
---
apiVersion: batch/v1
kind: Job
metadata:
  name: minio-init
  namespace: $CETS_NAMESPACE
spec:
  backoffLimit: 5
  template:
    spec:
      restartPolicy: OnFailure
      containers:
      - name: mc
        image: minio/mc:RELEASE.2025-08-13T08-35-41Z
        env:
        - name: OBJECT_STORAGE_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: cets-app-secrets
              key: object-storage-access-key
        - name: OBJECT_STORAGE_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: cets-app-secrets
              key: object-storage-secret-key
        command: ["/bin/sh", "-c"]
        args:
        - |
          until mc alias set local http://minio:9000 "\$OBJECT_STORAGE_ACCESS_KEY" "\$OBJECT_STORAGE_SECRET_KEY"; do sleep 2; done
          mc mb --ignore-existing "local/$OBJECT_STORAGE_BUCKET"
---
apiVersion: batch/v1
kind: Job
metadata:
  name: cets-migrate
  namespace: $CETS_NAMESPACE
spec:
  backoffLimit: 3
  template:
    spec:
      restartPolicy: Never
      imagePullSecrets:
      - name: cets-ecr-pull
      containers:
      - name: migrate
        image: $api_deploy_image
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
      restartPolicy: Never
      imagePullSecrets:
      - name: cets-ecr-pull
      containers:
      - name: seed
        image: $api_deploy_image
        imagePullPolicy: IfNotPresent
        args: ["seed"]
        envFrom:
        - configMapRef:
            name: cets-config
        - secretRef:
            name: cets-runtime-env
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: $CETS_NAMESPACE
spec:
  replicas: $app_replicas
  selector:
    matchLabels:
      app: backend
  template:
    metadata:
      labels:
        app: backend
    spec:
      imagePullSecrets:
      - name: cets-ecr-pull
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app: backend
      containers:
      - name: backend
        image: $api_deploy_image
        imagePullPolicy: IfNotPresent
        ports:
        - name: http
          containerPort: 8080
        resources:
          requests:
            cpu: 250m
            memory: 512Mi
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
  - name: http
    port: 8080
    targetPort: 8080
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: backend
  namespace: $CETS_NAMESPACE
spec:
  minReplicas: 1
  maxReplicas: $BACKEND_MAX_REPLICAS
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: backend
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: $CETS_NAMESPACE
spec:
  replicas: $app_replicas
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
    spec:
      imagePullSecrets:
      - name: cets-ecr-pull
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app: frontend
      containers:
      - name: frontend
        image: $frontend_deploy_image
        imagePullPolicy: IfNotPresent
        ports:
        - name: http
          containerPort: 8080
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
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
  - name: http
    port: 8080
    targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cets
  namespace: $CETS_NAMESPACE
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
      - path: /readyz
        pathType: Exact
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
      - path: /
        pathType: Prefix
        backend:
          service:
            name: frontend
            port:
              number: 8080
EOF

  append_worker_deployments
  kubectl apply -f "$GENERATED_DIR/cets-app.yaml"
  restart_private_image_deployments_if_requested
  kubectl wait --for=condition=complete job/minio-init -n "$CETS_NAMESPACE" --timeout=300s
  kubectl wait --for=condition=complete job/cets-migrate -n "$CETS_NAMESPACE" --timeout=300s
  kubectl wait --for=condition=complete job/cets-seed -n "$CETS_NAMESPACE" --timeout=300s
  wait_private_image_deployments
}
