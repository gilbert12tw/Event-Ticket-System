#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl
validate_node_count
validate_ingress_mode

KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"
[ -f "$KUBECONFIG_AWS" ] || die "missing kubeconfig; run 40-bootstrap-k8s.sh first"
export KUBECONFIG="$KUBECONFIG_AWS"

log "checking node readiness"
kubectl get nodes -o wide
ready_nodes=$(kubectl get nodes --no-headers | awk '$2 == "Ready" { count++ } END { print count+0 }')
[ "$ready_nodes" = "$NODE_COUNT" ] || die "expected $NODE_COUNT Ready nodes, got $ready_nodes"

log "checking control-plane static pods"
for component in kube-apiserver kube-controller-manager kube-scheduler etcd; do
  count=$(kubectl -n kube-system get pods --no-headers |
    awk -v prefix="$component" '$1 ~ "^" prefix "-" && $2 == "1/1" && $3 == "Running" { count++ } END { print count+0 }')
  [ "$count" = "$NODE_COUNT" ] || die "expected $NODE_COUNT ready $component pods, got $count"
done
kubectl get --raw='/readyz?verbose' | grep -q '^\[+\]etcd ok$'
etcd_pod=$(kubectl -n kube-system get pods --no-headers |
  awk '$1 ~ /^etcd-/ && $3 == "Running" { print $1; exit }')
[ -n "$etcd_pod" ] || die "could not find a running etcd pod"
started_members=$(kubectl -n kube-system exec "$etcd_pod" -- \
  etcdctl \
    --cacert=/etc/kubernetes/pki/etcd/ca.crt \
    --cert=/etc/kubernetes/pki/etcd/server.crt \
    --key=/etc/kubernetes/pki/etcd/server.key \
    --endpoints=https://127.0.0.1:2379 \
    member list |
  awk -F, '$2 == " started" { count++ } END { print count+0 }')
[ "$started_members" = "$NODE_COUNT" ] || die "expected $NODE_COUNT started etcd members, got $started_members"

log "checking platform and app rollouts"
kubectl wait --for=condition=Ready cluster/cets-postgres -n "$CETS_NAMESPACE" --timeout=60s
for deployment in redis minio mailhog backend frontend worker-notification worker-projection worker-compensation worker-export; do
  kubectl rollout status "deployment/$deployment" -n "$CETS_NAMESPACE" --timeout=120s
done
kubectl -n "$CETS_NAMESPACE" get hpa backend >/dev/null
kubectl -n "$CETS_NAMESPACE" get ingress cets >/dev/null

log "checking in-cluster HTTP readiness"
kubectl -n "$CETS_NAMESPACE" run cets-smoke --rm -i --restart=Never \
  --image=curlimages/curl:8.16.0 \
  --command -- curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" \
  "http://ingress-nginx-controller.ingress-nginx.svc.cluster.local/readyz" >/dev/null

if [ "$APP_INGRESS_MODE" = "nlb" ]; then
  require_cmd aws
  require_aws_identity
  endpoint=$(stack_output "$CLUSTER_STACK_NAME" AppEndpoint)
  log "checking app NLB endpoint $endpoint"
  curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" "http://$endpoint/readyz" >/dev/null
elif [ -n "${PUBLIC_URL:-}" ]; then
  log "checking public Cloudflare endpoint $PUBLIC_URL"
  curl -fsS "$PUBLIC_URL/readyz" >/dev/null
else
  log "PUBLIC_URL not set; skipped external Cloudflare smoke"
fi

log "verification completed"
