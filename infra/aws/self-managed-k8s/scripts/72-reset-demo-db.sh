#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd kubectl

KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"
[ -f "$KUBECONFIG_AWS" ] || die "missing kubeconfig; run 40-bootstrap-k8s.sh first"
export KUBECONFIG="$KUBECONFIG_AWS"

backend_image=$(kubectl -n "$CETS_NAMESPACE" get deployment backend -o jsonpath='{.spec.template.spec.containers[?(@.name=="backend")].image}')
[ -n "$backend_image" ] || die "could not resolve backend image"

cat >"$GENERATED_DIR/cets-reset-demo-db.yaml" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: cets-reset-demo-db
  namespace: $CETS_NAMESPACE
spec:
  ttlSecondsAfterFinished: 600
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      imagePullSecrets:
      - name: cets-ecr-pull
      containers:
      - name: reset-demo-db
        image: $backend_image
        imagePullPolicy: IfNotPresent
        args: ["reset-demo-db"]
        envFrom:
        - configMapRef:
            name: cets-config
        - secretRef:
            name: cets-runtime-env
EOF

kubectl -n "$CETS_NAMESPACE" delete job cets-reset-demo-db --ignore-not-found
kubectl apply -f "$GENERATED_DIR/cets-reset-demo-db.yaml"
kubectl wait --for=condition=complete job/cets-reset-demo-db -n "$CETS_NAMESPACE" --timeout=300s
log "demo database reset completed"
