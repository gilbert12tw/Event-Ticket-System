#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd kubectl
validate_node_count

KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"
[ -f "$KUBECONFIG_AWS" ] || die "missing kubeconfig; run 40-bootstrap-k8s.sh first"
export KUBECONFIG="$KUBECONFIG_AWS"

log "deleting one backend pod and waiting for recovery"
pod=$(kubectl -n "$CETS_NAMESPACE" get pod -l app=backend -o jsonpath='{.items[0].metadata.name}')
kubectl -n "$CETS_NAMESPACE" delete pod "$pod" --wait=false
kubectl rollout status deployment/backend -n "$CETS_NAMESPACE" --timeout=180s

log "checking app readiness after pod disruption"
kubectl -n "$CETS_NAMESPACE" run cets-drill-smoke --rm -i --restart=Never \
  --image=curlimages/curl:8.16.0 \
  --command -- curl -fsS -H "Host: $CETS_PUBLIC_HOSTNAME" \
  "http://ingress-nginx-controller.ingress-nginx.svc.cluster.local/readyz" >/dev/null

if [ "$NODE_COUNT" != "3" ]; then
  log "NODE_COUNT=1; skipping node and PostgreSQL failover drills"
  exit 0
fi

require_cmd aws
require_cmd jq
require_aws_identity
CONTROL_PLANE_ASG=$(stack_output "$CLUSTER_STACK_NAME" ControlPlaneAutoScalingGroupName)

etcd_exec_args=(
  --cacert=/etc/kubernetes/pki/etcd/ca.crt
  --cert=/etc/kubernetes/pki/etcd/server.crt
  --key=/etc/kubernetes/pki/etcd/server.key
  --endpoints=https://127.0.0.1:2379
)

remove_etcd_member() {
  local node_name=$1
  [ -n "$node_name" ] || return 0
  local etcd_pod member_id
  etcd_pod=$(kubectl -n kube-system get pods --no-headers |
    awk -v dead="etcd-$node_name" '$1 ~ /^etcd-/ && $1 != dead && $3 == "Running" { print $1; exit }')
  [ -n "$etcd_pod" ] || die "could not find a surviving etcd pod to remove $node_name"
  member_id=$(kubectl -n kube-system exec "$etcd_pod" -- \
    etcdctl "${etcd_exec_args[@]}" member list |
    awk -F, -v name=" $node_name" '$3 == name { print $1; exit }')
  if [ -n "$member_id" ]; then
    log "removing stale etcd member $node_name ($member_id)"
    kubectl -n kube-system exec "$etcd_pod" -- etcdctl "${etcd_exec_args[@]}" member remove "$member_id"
  fi
}

target_instance=$(aws_cli autoscaling describe-auto-scaling-groups \
  --auto-scaling-group-names "$CONTROL_PLANE_ASG" \
  --query 'AutoScalingGroups[0].Instances[0].InstanceId' \
  --output text)
target_private_ip=$(aws_cli ec2 describe-instances \
  --instance-ids "$target_instance" \
  --query 'Reservations[0].Instances[0].PrivateIpAddress' \
  --output text)
target_node=$(kubectl get nodes -o json |
  jq -r --arg ip "$target_private_ip" '.items[] | select(.status.addresses[]? | .type == "InternalIP" and .address == $ip) | .metadata.name' |
  head -1)
log "terminating one control-plane EC2 instance for ASG recovery drill: $target_instance"
aws_cli ec2 terminate-instances --instance-ids "$target_instance" >/dev/null
aws_cli ec2 wait instance-terminated --instance-ids "$target_instance"
remove_etcd_member "$target_node"
[ -n "$target_node" ] && kubectl delete node "$target_node" --ignore-not-found || true

log "waiting for ASG replacement EC2 instance"
deadline=$((SECONDS + 600))
while [ "$SECONDS" -lt "$deadline" ]; do
  in_service=$(aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$CONTROL_PLANE_ASG" \
    --query 'length(AutoScalingGroups[0].Instances[?LifecycleState==`InService`])' \
    --output text)
  [ "$in_service" = "3" ] && break
  sleep 15
done
[ "${in_service:-0}" = "3" ] || die "ASG did not recover to 3 in-service instances"

log "joining replacement node through idempotent bootstrap"
APPLY=true "$SCRIPT_DIR/40-bootstrap-k8s.sh"

log "waiting for ASG replacement and Kubernetes node readiness"
deadline=$((SECONDS + 900))
while [ "$SECONDS" -lt "$deadline" ]; do
  ready_nodes=$(kubectl get nodes --no-headers 2>/dev/null | awk '$2 == "Ready" { count++ } END { print count+0 }')
  [ "$ready_nodes" = "3" ] && break
  sleep 15
done
[ "${ready_nodes:-0}" = "3" ] || die "Kubernetes did not recover to 3 Ready nodes"

log "checking CloudNativePG synchronous failover"
old_primary=$(kubectl -n "$CETS_NAMESPACE" get cluster cets-postgres -o jsonpath='{.status.currentPrimary}')
[ -n "$old_primary" ] || die "could not determine PostgreSQL primary"
probe_id="aws-ha-probe-$(date +%s)"
kubectl -n "$CETS_NAMESPACE" exec "$old_primary" -c postgres -- \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc \
  "INSERT INTO audit_logs (audit_id, actor_id, role, action, entity_type, entity_id, metadata) VALUES ('$probe_id', 'aws-ha-drill', 'system_admin', 'ha.failover_probe', 'system', '$probe_id', '{\"probe\":\"aws_postgres_failover\"}'::jsonb);"
kubectl -n "$CETS_NAMESPACE" delete pod "$old_primary" --grace-period=0 --force --wait=false

deadline=$((SECONDS + 240))
new_primary=""
while [ "$SECONDS" -lt "$deadline" ]; do
  candidate=$(kubectl -n "$CETS_NAMESPACE" get cluster cets-postgres -o jsonpath='{.status.currentPrimary}' 2>/dev/null || true)
  if [ -n "$candidate" ] && [ "$candidate" != "$old_primary" ]; then
    new_primary=$candidate
    break
  fi
  sleep 5
done
[ -n "$new_primary" ] || die "PostgreSQL primary did not fail over"
kubectl wait --for=condition=Ready cluster/cets-postgres -n "$CETS_NAMESPACE" --timeout=600s
survived=$(kubectl -n "$CETS_NAMESPACE" exec "$new_primary" -c postgres -- \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc \
  "SELECT count(*) FROM audit_logs WHERE audit_id = '$probe_id';")
[ "$survived" = "1" ] || die "failover probe was not found after PostgreSQL failover"

log "failure drill completed"
