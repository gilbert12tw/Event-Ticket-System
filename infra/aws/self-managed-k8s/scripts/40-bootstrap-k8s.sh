#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"
# shellcheck source=bootstrap-k8s/node-api-proxy.sh
. "$SCRIPT_DIR/bootstrap-k8s/node-api-proxy.sh"

load_env
require_apply
require_cmd aws
require_cmd jq
require_cmd kubectl
require_cmd helm
require_aws_identity
validate_node_count
validate_k8s_log_rotation

CONTROL_PLANE_ASG=$(stack_output "$CLUSTER_STACK_NAME" ControlPlaneAutoScalingGroupName)
KUBECONFIG_AWS="$GENERATED_DIR/kubeconfig"

instance_ids() {
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$CONTROL_PLANE_ASG" \
    --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`].InstanceId' \
    --output text | tr '\t' '\n' | sort
}

instance_ip() {
  local instance_id=$1
  local field=$2
  aws_cli ec2 describe-instances \
    --instance-ids "$instance_id" \
    --query "Reservations[0].Instances[0].$field" \
    --output text
}

wait_for_instances() {
  local ids
  ids=$(instance_ids)
  local count
  count=$(printf '%s\n' "$ids" | sed '/^$/d' | wc -l | tr -d ' ')
  [ "$count" = "$NODE_COUNT" ] || die "expected $NODE_COUNT in-service instances, got $count"
  aws_cli ec2 wait instance-status-ok --instance-ids $ids
  if [ "$AWS_USE_SSM" = "true" ]; then
    for id in $ids; do
      log "waiting for SSM online on $id"
      local deadline=$((SECONDS + 240))
      while [ "$SECONDS" -lt "$deadline" ]; do
        status=$(aws_cli ssm describe-instance-information \
          --filters "Key=InstanceIds,Values=$id" \
          --query 'InstanceInformationList[0].PingStatus' \
          --output text 2>/dev/null || true)
        [ "$status" = "Online" ] && break
        sleep 5
      done
      [ "$status" = "Online" ] || die "SSM did not come online for $id"
    done
  fi
}

run_ssm() {
  local instance_id=$1
  local command=$2
  local param_file="$GENERATED_DIR/ssm-parameters.json"
  local bash_command
  bash_command=$(cat <<EOF
bash <<'CETS_SSM_SCRIPT'
$command
CETS_SSM_SCRIPT
EOF
)
  jq -n --arg cmd "$bash_command" '{commands: [$cmd], executionTimeout: ["1800"]}' >"$param_file"
  local command_id
  command_id=$(aws_cli ssm send-command \
    --instance-ids "$instance_id" \
    --document-name AWS-RunShellScript \
    --parameters "file://$param_file" \
    --query 'Command.CommandId' \
    --output text)
  aws_cli ssm wait command-executed --command-id "$command_id" --instance-id "$instance_id" || true
  local status
  status=$(aws_cli ssm get-command-invocation \
    --command-id "$command_id" \
    --instance-id "$instance_id" \
    --query Status \
    --output text)
  aws_cli ssm get-command-invocation \
    --command-id "$command_id" \
    --instance-id "$instance_id" \
    --query StandardOutputContent \
    --output text
  if [ "$status" != "Success" ]; then
    aws_cli ssm get-command-invocation \
      --command-id "$command_id" \
      --instance-id "$instance_id" \
      --query StandardErrorContent \
      --output text >&2
    die "remote command failed on $instance_id with $status"
  fi
}

run_ssh() {
  local instance_id=$1
  local command=$2
  local ip
  ip=$(instance_ip "$instance_id" PublicIpAddress)
  local prefix
  prefix=$(ssh_prefix)
  # shellcheck disable=SC2086
  $prefix"$ip" "sudo bash -lc $(printf '%q' "$command")"
}

run_remote() {
  local instance_id=$1
  local command=$2
  if [ "$AWS_USE_SSM" = "true" ]; then
    run_ssm "$instance_id" "$command"
  else
    run_ssh "$instance_id" "$command"
  fi
}

remote_has_file() {
  local instance_id=$1
  local remote_path=$2
  local output
  if ! output=$(run_remote "$instance_id" "[ -f '$remote_path' ] && echo yes || true"); then
    die "remote file check failed on $instance_id for $remote_path"
  fi
  grep -q '^yes$' <<<"$output"
}

api_target_group_arn() {
  aws_cli cloudformation describe-stack-resources \
    --stack-name "$CLUSTER_STACK_NAME" \
    --logical-resource-id ApiTargetGroup \
    --query 'StackResources[0].PhysicalResourceId' \
    --output text
}

wait_for_api_target_healthy() {
  local instance_id=$1
  local target_group_arn state
  target_group_arn=$(api_target_group_arn)
  local deadline=$((SECONDS + 240))
  while [ "$SECONDS" -lt "$deadline" ]; do
    state=$(aws_cli elbv2 describe-target-health \
      --target-group-arn "$target_group_arn" \
      --targets "Id=$instance_id" \
      --query 'TargetHealthDescriptions[0].TargetHealth.State' \
      --output text 2>/dev/null || true)
    [ "$state" = "healthy" ] && return 0
    log "waiting for API target $instance_id to become healthy, current=$state"
    sleep 10
  done
  die "API target $instance_id did not become healthy in target group $target_group_arn"
}

set_kubeadm_endpoint_command() {
  local kubectl_endpoint=$1
  local advertised_endpoint=$2
  cat <<EOF
set -euo pipefail
JOIN_KUBECONFIG=/tmp/kubeadm-bootstrap.conf
cp /etc/kubernetes/admin.conf "\$JOIN_KUBECONFIG"
sed -i 's#server: https://.*#server: https://$kubectl_endpoint#' "\$JOIN_KUBECONFIG"
kubectl --kubeconfig "\$JOIN_KUBECONFIG" -n kube-public get configmap cluster-info -o jsonpath='{.data.kubeconfig}' >/tmp/cluster-info.kubeconfig
sed -i 's#server: https://.*#server: https://$advertised_endpoint#' /tmp/cluster-info.kubeconfig
kubectl --kubeconfig "\$JOIN_KUBECONFIG" -n kube-public patch configmap cluster-info --type merge --patch "\$(python3 -c 'import json; print(json.dumps({"data":{"kubeconfig":open("/tmp/cluster-info.kubeconfig").read()}}))')"
kubectl --kubeconfig "\$JOIN_KUBECONFIG" -n kube-system get configmap kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' >/tmp/kubeadm-cluster-config.yaml
sed -i 's#^controlPlaneEndpoint: .*#controlPlaneEndpoint: "$advertised_endpoint"#' /tmp/kubeadm-cluster-config.yaml
kubectl --kubeconfig "\$JOIN_KUBECONFIG" -n kube-system patch configmap kubeadm-config --type merge --patch "\$(python3 -c 'import json; print(json.dumps({"data":{"ClusterConfiguration":open("/tmp/kubeadm-cluster-config.yaml").read()}}))')"
EOF
}

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
  etcd_pod=$(KUBECONFIG="$KUBECONFIG_AWS" kubectl -n kube-system get pods --no-headers |
    awk -v dead="etcd-$node_name" '$1 ~ /^etcd-/ && $1 != dead && $3 == "Running" { print $1; exit }')
  [ -n "$etcd_pod" ] || die "could not find a surviving etcd pod to remove $node_name"
  member_id=$(KUBECONFIG="$KUBECONFIG_AWS" kubectl -n kube-system exec "$etcd_pod" -- \
    etcdctl "${etcd_exec_args[@]}" member list |
    awk -F, -v name=" $node_name" '$3 == name { print $1; exit }')
  if [ -n "$member_id" ]; then
    log "removing stale etcd member $node_name ($member_id)"
    KUBECONFIG="$KUBECONFIG_AWS" kubectl -n kube-system exec "$etcd_pod" -- \
      etcdctl "${etcd_exec_args[@]}" member remove "$member_id"
  fi
}

reconcile_stale_nodes() {
  [ "$cluster_exists" = "true" ] || return 0
  local live_private_ips_json node_json
  live_private_ips_json=$(printf '%s\n' "${PRIVATE_IPS[@]}" |
    jq -Rsc 'split("\n") | map(select(length > 0 and . != "None"))')
  node_json=$(KUBECONFIG="$KUBECONFIG_AWS" kubectl get nodes -o json)
  local node_name
  while IFS= read -r node_name; do
    [ -n "$node_name" ] || continue
    log "removing stale node $node_name before replacement bootstrap"
    [ "$NODE_COUNT" = "3" ] && remove_etcd_member "$node_name"
    KUBECONFIG="$KUBECONFIG_AWS" kubectl delete node "$node_name" --ignore-not-found
  done < <(jq -r --argjson live "$live_private_ips_json" '
    .items[]
    | select(.metadata.name | test("^cets-cp-[0-9]+$"))
    | . as $node
    | ([ $node.status.addresses[]? | select(.type == "InternalIP") | .address ]
        | any(. as $ip | ($live | index($ip)) != null)) as $has_live_ip
    | select($has_live_ip | not)
    | .metadata.name
  ' <<<"$node_json")
}

host_prepare_command() {
  local hostname=$1
  cat <<EOF
set -euo pipefail
if [ ! -f /etc/kubernetes/kubelet.conf ]; then
  hostnamectl set-hostname "$hostname"
fi
swapoff -a || true
sed -i.bak '/ swap / s/^/#/' /etc/fstab
cat >/etc/modules-load.d/k8s.conf <<'MODS'
overlay
br_netfilter
MODS
modprobe overlay
modprobe br_netfilter
cat >/etc/sysctl.d/99-kubernetes.conf <<'SYSCTL'
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
SYSCTL
sysctl --system
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl gnupg apt-transport-https containerd haproxy
mkdir -p /etc/containerd
containerd config default >/etc/containerd/config.toml
sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
systemctl restart containerd
install -m 0755 -d /etc/apt/keyrings
curl -fsSL "https://pkgs.k8s.io/core:/stable:/v$K8S_VERSION/deb/Release.key" |
  gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v$K8S_VERSION/deb/ /" >/etc/apt/sources.list.d/kubernetes.list
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y kubelet kubeadm kubectl
apt-mark hold kubelet kubeadm kubectl
systemctl enable --now kubelet
EOF
}

wait_for_instances
declare -a IDS=()
while IFS= read -r id; do
  [ -n "$id" ] && IDS+=("$id")
done < <(instance_ids)
declare -a PRIVATE_IPS=()
declare -a PUBLIC_IPS=()
for id in "${IDS[@]}"; do
  PRIVATE_IPS+=("$(instance_ip "$id" PrivateIpAddress)")
  PUBLIC_IPS+=("$(instance_ip "$id" PublicIpAddress)")
done

cluster_exists=false
bootstrap_id="${IDS[0]}"
if [ -f "$KUBECONFIG_AWS" ] && KUBECONFIG="$KUBECONFIG_AWS" kubectl --request-timeout=10s get nodes >/dev/null 2>&1; then
  cluster_exists=true
  bootstrap_id=""
  for id in "${IDS[@]}"; do
    if remote_has_file "$id" /etc/kubernetes/admin.conf; then
      bootstrap_id=$id
      break
    fi
  done
  [ -n "$bootstrap_id" ] || die "cluster exists but no current EC2 node has /etc/kubernetes/admin.conf"
fi

reconcile_stale_nodes

declare -a CURRENT_NODE_NAMES=()
declare -a MISSING_NODE_NAMES=()

node_name_present() {
  local candidate=$1
  local node_name
  for node_name in "${CURRENT_NODE_NAMES[@]}"; do
    [ "$node_name" = "$candidate" ] && return 0
  done
  return 1
}

if [ "$cluster_exists" = "true" ]; then
  while IFS= read -r node_name; do
    [ -n "$node_name" ] && CURRENT_NODE_NAMES+=("$node_name")
  done < <(KUBECONFIG="$KUBECONFIG_AWS" kubectl get nodes -o json |
    jq -r '.items[].metadata.name' | sort)
  for ((node_number=1; node_number<=NODE_COUNT; node_number++)); do
    node_name="cets-cp-$node_number"
    node_name_present "$node_name" || MISSING_NODE_NAMES+=("$node_name")
  done
fi

missing_index=0
for index in "${!IDS[@]}"; do
  node_name="cets-cp-$((index + 1))"
  if [ "$cluster_exists" = "true" ] && ! remote_has_file "${IDS[$index]}" /etc/kubernetes/kubelet.conf; then
    [ "$missing_index" -lt "${#MISSING_NODE_NAMES[@]}" ] ||
      die "no missing Kubernetes node name is available for replacement instance ${IDS[$index]}"
    node_name="${MISSING_NODE_NAMES[$missing_index]}"
    missing_index=$((missing_index + 1))
  fi
  log "preparing ${IDS[$index]} as $node_name"
  run_remote "${IDS[$index]}" "$(host_prepare_command "$node_name")"
done

if [ "$NODE_COUNT" = "3" ]; then
  API_ENDPOINT=$(stack_output "$CLUSTER_STACK_NAME" ApiEndpoint)
else
  [ "${PUBLIC_IPS[0]}" != "None" ] || die "single-node mode requires a public IP for local kubeconfig access"
  API_ENDPOINT="${PUBLIC_IPS[0]}:6443"
fi
API_HOST=${API_ENDPOINT%:*}

if [ "$NODE_COUNT" = "3" ]; then
  NODE_LOCAL_API_ENDPOINT="$API_HOST:6444"
  for id in "${IDS[@]}"; do
    proxy_command=$(node_api_proxy_command) ||
      die "failed to render node-local API proxy command"
    log "configuring node-local API proxy on $id"
    run_remote "$id" "$proxy_command"
  done
fi

cert_sans="    - $API_HOST"
for ip in "${PRIVATE_IPS[@]}"; do
  cert_sans="$cert_sans
    - $ip"
done
for ip in "${PUBLIC_IPS[@]}"; do
  [ "$ip" = "None" ] && continue
  cert_sans="$cert_sans
    - $ip"
done

init_command=$(cat <<EOF
set -euo pipefail
mkdir -p /etc/kubernetes
cat >/tmp/kubeadm-config.yaml <<'KUBEADM'
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: stable-$K8S_VERSION
controlPlaneEndpoint: "$API_ENDPOINT"
networking:
  podSubnet: "$K8S_POD_CIDR"
  serviceSubnet: "$K8S_SERVICE_CIDR"
apiServer:
  certSANs:
$cert_sans
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
containerLogMaxSize: "$K8S_CONTAINER_LOG_MAX_SIZE"
containerLogMaxFiles: $K8S_CONTAINER_LOG_MAX_FILES
KUBEADM
if [ ! -f /etc/kubernetes/admin.conf ]; then
  kubeadm init --config /tmp/kubeadm-config.yaml --upload-certs
fi
mkdir -p /root/.kube
cp /etc/kubernetes/admin.conf /root/.kube/config
EOF
)

if [ "$cluster_exists" = "true" ]; then
  log "cluster already exists; using $bootstrap_id for join commands"
else
  log "initializing first control-plane node"
  run_remote "$bootstrap_id" "$init_command"
fi

log "collecting kubeconfig"
run_remote "$bootstrap_id" "cat /etc/kubernetes/admin.conf" >"$KUBECONFIG_AWS"
chmod 0600 "$KUBECONFIG_AWS"

if [ "$NODE_COUNT" = "3" ]; then
  wait_for_api_target_healthy "$bootstrap_id"
  log "creating control-plane join command"
  log "using node-local API proxy for kubeadm join discovery"
  run_remote "$bootstrap_id" "$(set_kubeadm_endpoint_command "$NODE_LOCAL_API_ENDPOINT" "$NODE_LOCAL_API_ENDPOINT")" >/dev/null
  JOIN_KUBECONFIG=/tmp/kubeadm-bootstrap.conf
  JOIN_KUBECONFIG_COMMAND="set -euo pipefail
cp /etc/kubernetes/admin.conf $JOIN_KUBECONFIG
sed -i 's#server: https://.*#server: https://$NODE_LOCAL_API_ENDPOINT#' $JOIN_KUBECONFIG"
  run_remote "$bootstrap_id" "$JOIN_KUBECONFIG_COMMAND" >/dev/null
  CERT_KEY=$(run_remote "$bootstrap_id" "kubeadm init phase upload-certs --upload-certs --kubeconfig $JOIN_KUBECONFIG 2>/dev/null | tail -1")
  JOIN_COMMAND=$(run_remote "$bootstrap_id" "kubeadm token create --kubeconfig $JOIN_KUBECONFIG --print-join-command")
  JOIN_COMMAND=${JOIN_COMMAND/"$API_ENDPOINT"/"$NODE_LOCAL_API_ENDPOINT"}
  for id in "${IDS[@]}"; do
    [ "$id" = "$bootstrap_id" ] && continue
    log "joining $id as control-plane"
    run_remote "$id" "set -euo pipefail
if [ ! -f /etc/kubernetes/kubelet.conf ]; then
      $JOIN_COMMAND --control-plane --certificate-key $CERT_KEY
fi"
  done
  log "restoring kubeadm discovery endpoint to API load balancer"
  run_remote "$bootstrap_id" "$(set_kubeadm_endpoint_command "$NODE_LOCAL_API_ENDPOINT" "$API_ENDPOINT")" >/dev/null
fi

export KUBECONFIG="$KUBECONFIG_AWS"
if [ "$NODE_COUNT" = "3" ]; then
  log "pointing kube-proxy at node-local API proxy"
  KUBE_PROXY_CONFIG="$GENERATED_DIR/kube-proxy.kubeconfig"
  kubectl -n kube-system get configmap kube-proxy -o jsonpath='{.data.kubeconfig\.conf}' |
    sed "s#server: https://.*#server: https://$NODE_LOCAL_API_ENDPOINT#" >"$KUBE_PROXY_CONFIG"
  KUBE_PROXY_PATCH=$(jq -n --arg data "$(cat "$KUBE_PROXY_CONFIG")" '{"data":{"kubeconfig.conf":$data}}')
  kubectl -n kube-system patch configmap kube-proxy --type merge --patch "$KUBE_PROXY_PATCH"
  KUBE_PROXY_HOST_ALIAS_PATCH=$(jq -n --arg host "$API_HOST" \
    '{"spec":{"template":{"spec":{"hostAliases":[{"ip":"127.0.0.1","hostnames":[$host]}]}}}}')
  kubectl -n kube-system patch daemonset kube-proxy --type merge --patch "$KUBE_PROXY_HOST_ALIAS_PATCH"
  kubectl -n kube-system rollout restart daemonset/kube-proxy
  kubectl -n kube-system rollout status daemonset/kube-proxy --timeout=180s
fi
log "allowing workloads on control-plane nodes"
kubectl taint nodes --all node-role.kubernetes.io/control-plane- >/dev/null 2>&1 || true
kubectl label nodes --all node.cets.io/aws=true --overwrite

log "installing Calico CNI"
helm repo add projectcalico https://docs.tigera.io/calico/charts >/dev/null
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx >/dev/null
helm repo update >/dev/null
if kubectl get installation.operator.tigera.io default >/dev/null 2>&1; then
  log "Calico installation already exists; skipping Tigera operator Helm upgrade"
else
  helm upgrade --install calico projectcalico/tigera-operator \
    --namespace tigera-operator \
    --create-namespace \
    --version v3.31.0 \
    --set installation.cni.type=Calico \
    --set installation.calicoNetwork.bgp=Disabled \
    --set "installation.calicoNetwork.ipPools[0].cidr=$K8S_POD_CIDR" \
    --set installation.calicoNetwork.ipPools[0].encapsulation=VXLAN \
    --set installation.calicoNetwork.ipPools[0].natOutgoing=Enabled
fi
kubectl rollout status deployment/tigera-operator -n tigera-operator --timeout=240s

log "installing metrics-server"
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system patch deployment metrics-server --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]' || true
kubectl rollout status deployment/metrics-server -n kube-system --timeout=180s

log "installing ingress-nginx NodePort entrypoint"
cat >"$GENERATED_DIR/ingress-nginx-values.yaml" <<EOF
controller:
  kind: DaemonSet
  service:
    type: NodePort
    nodePorts:
      http: 30080
    externalTrafficPolicy: Local
  metrics:
    enabled: true
  config:
    use-forwarded-headers: "true"
    compute-full-forwarded-for: "true"
EOF
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace \
  --version 4.15.1 \
  -f "$GENERATED_DIR/ingress-nginx-values.yaml"
kubectl rollout status daemonset/ingress-nginx-controller -n ingress-nginx --timeout=240s

log "kubeadm bootstrap completed; kubeconfig at $KUBECONFIG_AWS"
