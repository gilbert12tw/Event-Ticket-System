#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_cmd kubectl
require_cmd helm

log "installing Calico CNI"
helm repo add projectcalico https://docs.tigera.io/calico/charts >/dev/null
helm repo update >/dev/null
helm upgrade --install calico projectcalico/tigera-operator \
  --namespace tigera-operator \
  --create-namespace \
  --version v3.31.0 \
  --set installation.cni.type=Calico \
  --set installation.calicoNetwork.bgp=Enabled \
  --set "installation.calicoNetwork.ipPools[0].cidr=$K8S_POD_CIDR" \
  --set installation.calicoNetwork.ipPools[0].encapsulation=VXLAN \
  --set installation.calicoNetwork.ipPools[0].natOutgoing=Enabled

kubectl_bm rollout status deployment/tigera-operator -n tigera-operator --timeout=180s

log "installing MetalLB with FRR-K8s"
helm repo add metallb https://metallb.github.io/metallb >/dev/null
helm repo update >/dev/null
helm upgrade --install metallb metallb/metallb \
  --namespace metallb-system \
  --create-namespace \
  --version 0.16.1

kubectl_bm rollout status deployment/metallb-controller -n metallb-system --timeout=180s

cat >"$GENERATED_DIR/metallb-bgp.yaml" <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: cets-lb-pool
  namespace: metallb-system
spec:
  addresses:
  - $METALLB_LB_POOL
---
apiVersion: metallb.io/v1beta1
kind: BGPAdvertisement
metadata:
  name: cets-lb-bgp
  namespace: metallb-system
spec:
  ipAddressPools:
  - cets-lb-pool
---
apiVersion: metallb.io/v1beta2
kind: BGPPeer
metadata:
  name: lan-router
  namespace: metallb-system
spec:
  myASN: $METALLB_MY_ASN
  peerASN: $METALLB_PEER_ASN
  peerAddress: $METALLB_PEER_ADDRESS
  holdTime: $METALLB_BGP_HOLD_TIME
EOF

kubectl_bm apply -f "$GENERATED_DIR/metallb-bgp.yaml"

log "allowing MetalLB to advertise from kubeadm control-plane nodes"
kubectl_bm label nodes "${NODES[@]}" \
  node.kubernetes.io/exclude-from-external-load-balancers- --overwrite

log "installing ingress-nginx as the internal LoadBalancer entrypoint"
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx >/dev/null
helm repo update >/dev/null
cat >"$GENERATED_DIR/ingress-nginx-values.yaml" <<EOF
controller:
  kind: DaemonSet
  hostNetwork: false
  service:
    type: LoadBalancer
    loadBalancerIP: "$METALLB_INGRESS_IP"
    externalTrafficPolicy: Local
  admissionWebhooks:
    enabled: true
  metrics:
    enabled: true
  config:
    use-forwarded-headers: "true"
    compute-full-forwarded-for: "true"
    server-snippet: |
      add_header X-CETS-Gateway-Replica \$hostname always;
EOF

helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace \
  --version 4.15.1 \
  -f "$GENERATED_DIR/ingress-nginx-values.yaml"

kubectl_bm rollout status daemonset/ingress-nginx-controller -n ingress-nginx --timeout=240s
log "networking installation completed"
