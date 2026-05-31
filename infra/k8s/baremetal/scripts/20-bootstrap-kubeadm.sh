#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd ssh
require_cmd scp

FIRST_NODE=${NODES[0]}
FIRST_IP=${NODE_IPS[0]}

log "writing kube-vip manifest and kubeadm config for $FIRST_NODE"
cat >"$GENERATED_DIR/kube-vip.yaml" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: kube-vip
  namespace: kube-system
spec:
  containers:
  - args:
    - manager
    env:
    - name: vip_arp
      value: "true"
    - name: address
      value: "$K8S_API_VIP"
    - name: port
      value: "$K8S_API_PORT"
    - name: vip_interface
      value: "$K8S_INTERFACE"
    - name: cp_enable
      value: "true"
    - name: cp_namespace
      value: kube-system
    - name: vip_leaderelection
      value: "true"
    - name: vip_nodename
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    image: ghcr.io/kube-vip/kube-vip:v0.9.2
    imagePullPolicy: IfNotPresent
    name: kube-vip
    securityContext:
      capabilities:
        add:
        - NET_ADMIN
        - NET_RAW
    volumeMounts:
    - mountPath: /.kube/config
      name: kubeconfig
      readOnly: true
  hostNetwork: true
  volumes:
  - hostPath:
      path: /etc/kubernetes/kube-vip.conf
      type: FileOrCreate
    name: kubeconfig
EOF

cat >"$GENERATED_DIR/kubeadm-config.yaml" <<EOF
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: stable-$K8S_VERSION
controlPlaneEndpoint: "$K8S_API_VIP:$K8S_API_PORT"
networking:
  podSubnet: "$K8S_POD_CIDR"
  serviceSubnet: "$K8S_SERVICE_CIDR"
apiServer:
  certSANs:
  - "$K8S_API_VIP"
  - "$FIRST_IP"
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
EOF

copy_to_node "$GENERATED_DIR/kube-vip.yaml" "$FIRST_NODE" /tmp/kube-vip.yaml
copy_to_node "$GENERATED_DIR/kubeadm-config.yaml" "$FIRST_NODE" /tmp/kubeadm-config.yaml

log "initializing first control-plane node"
sudo_remote "$FIRST_NODE" "
set -euo pipefail
mkdir -p /etc/kubernetes/manifests
cp /tmp/kube-vip.yaml /etc/kubernetes/manifests/kube-vip.yaml
if [ ! -f /etc/kubernetes/admin.conf ]; then
  kubeadm init --config /tmp/kubeadm-config.yaml --upload-certs
fi
node_ip=\$(hostname -I | awk '{print \$1}')
kubevip_source=/etc/kubernetes/super-admin.conf
[ -f \"\$kubevip_source\" ] || kubevip_source=/etc/kubernetes/admin.conf
sed \"s#https://$K8S_API_VIP:$K8S_API_PORT#https://\${node_ip}:$K8S_API_PORT#\" \"\$kubevip_source\" >/etc/kubernetes/kube-vip.conf
"

mkdir -p "$HOME/.kube"
sudo_remote "$FIRST_NODE" "cat /etc/kubernetes/admin.conf" >"$HOME/.kube/config"
chmod 0600 "$HOME/.kube/config"

log "creating join commands"
CERT_KEY=$(sudo_remote "$FIRST_NODE" "kubeadm init phase upload-certs --upload-certs 2>/dev/null | tail -1")
CONTROL_JOIN=$(sudo_remote "$FIRST_NODE" "kubeadm token create --print-join-command --kubeconfig /etc/kubernetes/kube-vip.conf")

for node in "${NODES[@]:1}"; do
  log "joining $node as control-plane"
  copy_to_node "$GENERATED_DIR/kube-vip.yaml" "$node" /tmp/kube-vip.yaml
  sudo_remote "$node" "
set -euo pipefail
mkdir -p /etc/kubernetes/manifests
cp /tmp/kube-vip.yaml /etc/kubernetes/manifests/kube-vip.yaml
if [ ! -f /etc/kubernetes/kubelet.conf ]; then
  $CONTROL_JOIN --control-plane --certificate-key $CERT_KEY
fi
node_ip=\$(hostname -I | awk '{print \$1}')
kubevip_source=/etc/kubernetes/super-admin.conf
[ -f \"\$kubevip_source\" ] || kubevip_source=/etc/kubernetes/admin.conf
sed \"s#https://$K8S_API_VIP:$K8S_API_PORT#https://\${node_ip}:$K8S_API_PORT#\" \"\$kubevip_source\" >/etc/kubernetes/kube-vip.conf
"
done

log "allowing workloads on control-plane nodes"
kubectl_bm taint nodes --all node-role.kubernetes.io/control-plane- >/dev/null 2>&1 || true
kubectl_bm label nodes "${NODES[@]}" node.cets.io/baremetal=true --overwrite

log "kubeadm HA bootstrap completed"
