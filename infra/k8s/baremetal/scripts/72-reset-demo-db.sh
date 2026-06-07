#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd kubectl

backend_image=$(kubectl_bm -n "$CETS_NAMESPACE" get deployment backend -o jsonpath='{.spec.template.spec.containers[?(@.name=="backend")].image}')
[ -n "$backend_image" ] || die "could not resolve backend image"

image_pull_block() {
  if [ -n "${CETS_IMAGE_PULL_SECRET:-}" ]; then
    cat <<PULL_SECRET
      imagePullSecrets:
      - name: $CETS_IMAGE_PULL_SECRET
PULL_SECRET
  fi
}

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
$(image_pull_block)
      restartPolicy: Never
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

kubectl_bm -n "$CETS_NAMESPACE" delete job cets-reset-demo-db --ignore-not-found
kubectl_bm apply -f "$GENERATED_DIR/cets-reset-demo-db.yaml"
kubectl_bm wait --for=condition=complete job/cets-reset-demo-db -n "$CETS_NAMESPACE" --timeout=300s
log "demo database reset completed"
