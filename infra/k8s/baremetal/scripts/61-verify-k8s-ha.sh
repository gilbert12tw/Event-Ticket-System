#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_cmd kubectl
require_cmd curl
require_cmd nc

count_ready_static_pods() {
  local prefix=$1
  kubectl_bm -n kube-system get pods --no-headers |
    awk -v prefix="$prefix" '$1 ~ "^" prefix "-" && $2 == "1/1" && $3 == "Running" { count++ } END { print count+0 }'
}

log "checking all Kubernetes control-plane nodes are Ready"
kubectl_bm get nodes -o wide
ready_control_planes=$(kubectl_bm get nodes --no-headers |
  awk '$2 == "Ready" && $3 ~ /control-plane/ { count++ } END { print count+0 }')
[ "$ready_control_planes" = "3" ] || die "expected 3 Ready control-plane nodes, got $ready_control_planes"

log "checking kube-apiserver, controller-manager, scheduler, etcd, and kube-vip static pods"
for component in kube-apiserver kube-controller-manager kube-scheduler etcd kube-vip; do
  count=$(count_ready_static_pods "$component")
  [ "$count" = "3" ] || die "expected 3 ready $component pods, got $count"
done

log "checking kube-vip API VIP TCP and Kubernetes readyz"
nc -z -w 3 "$K8S_API_VIP" "$K8S_API_PORT"
curl -kfsS --max-time 8 "https://$K8S_API_VIP:$K8S_API_PORT/readyz" | grep -qx 'ok'
kubectl_bm get --raw='/readyz?verbose' | grep -q '^\[+\]etcd ok$'

log "checking kube-vip leader lease"
vip_holder=$(kubectl_bm -n kube-system get lease plndr-cp-lock -o jsonpath='{.spec.holderIdentity}')
[ -n "$vip_holder" ] || die "kube-vip lease plndr-cp-lock has no holder"
case " ${NODES[*]} " in
  *" $vip_holder "*) ;;
  *) die "unexpected kube-vip lease holder $vip_holder" ;;
esac
log "kube-vip leader is $vip_holder"

log "checking etcd cluster health and members"
etcd_pod=$(kubectl_bm -n kube-system get pod -l component=etcd -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
if [ -z "$etcd_pod" ]; then
  etcd_pod=$(kubectl_bm -n kube-system get pods --no-headers | awk '$1 ~ /^etcd-/ { print $1; exit }')
fi
[ -n "$etcd_pod" ] || die "could not find an etcd pod"

etcd_args=(
  --cacert=/etc/kubernetes/pki/etcd/ca.crt
  --cert=/etc/kubernetes/pki/etcd/server.crt
  --key=/etc/kubernetes/pki/etcd/server.key
  --endpoints=https://127.0.0.1:2379
)
kubectl_bm -n kube-system exec "$etcd_pod" -- etcdctl "${etcd_args[@]}" endpoint health --cluster
member_list=$(kubectl_bm -n kube-system exec "$etcd_pod" -- etcdctl "${etcd_args[@]}" member list)
started_members=$(printf '%s\n' "$member_list" | awk -F, '$2 == " started" { count++ } END { print count+0 }')
[ "$started_members" = "3" ] || die "expected 3 started etcd members, got $started_members"

log "Kubernetes HA verification completed"
