#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)
BM_DIR="$ROOT_DIR/infra/k8s/baremetal"
LOCAL_ENV="$BM_DIR/.env.baremetal.local"
GENERATED_DIR="$BM_DIR/generated"

log() {
  printf '[baremetal] %s\n' "$*" >&2
}

die() {
  printf '[baremetal] error: %s\n' "$*" >&2
  exit 1
}

load_env() {
  if [ -f "$ROOT_DIR/../.env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "$ROOT_DIR/../.env"
    set +a
  fi

  if [ -f "$LOCAL_ENV" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$LOCAL_ENV"
    set +a
  fi

  BAREMETAL_SSH_USER=${BAREMETAL_SSH_USER:-user}
  BAREMETAL_BECOME_PASSWORD=${BAREMETAL_BECOME_PASSWORD:-${SUDO_PASSWORD:-}}
  BAREMETAL_NODE_1=${BAREMETAL_NODE_1:-work1}
  BAREMETAL_NODE_1_IP=${BAREMETAL_NODE_1_IP:-10.121.124.200}
  BAREMETAL_NODE_2=${BAREMETAL_NODE_2:-work2}
  BAREMETAL_NODE_2_IP=${BAREMETAL_NODE_2_IP:-10.121.124.201}
  BAREMETAL_NODE_3=${BAREMETAL_NODE_3:-work3}
  BAREMETAL_NODE_3_IP=${BAREMETAL_NODE_3_IP:-10.121.124.202}
  K8S_VERSION=${K8S_VERSION:-1.35}
  CONTAINERD_IO_VERSION=${CONTAINERD_IO_VERSION:-1.7.29-1~ubuntu.24.04~noble}
  K8S_POD_CIDR=${K8S_POD_CIDR:-192.168.0.0/16}
  K8S_SERVICE_CIDR=${K8S_SERVICE_CIDR:-10.96.0.0/12}
  K8S_API_VIP=${K8S_API_VIP:-10.121.124.210}
  K8S_API_PORT=${K8S_API_PORT:-6443}
  K8S_INTERFACE=${K8S_INTERFACE:-enp6s18}
  METALLB_LB_POOL=${METALLB_LB_POOL:-10.121.124.211-10.121.124.220}
  METALLB_INGRESS_IP=${METALLB_INGRESS_IP:-10.121.124.211}
  METALLB_PEER_ADDRESS=${METALLB_PEER_ADDRESS:-10.121.124.254}
  METALLB_MY_ASN=${METALLB_MY_ASN:-64512}
  METALLB_PEER_ASN=${METALLB_PEER_ASN:-64513}
  METALLB_BGP_HOLD_TIME=${METALLB_BGP_HOLD_TIME:-90s}
  CETS_NAMESPACE=${CETS_NAMESPACE:-cets}
  CETS_PUBLIC_HOSTNAME=${CETS_PUBLIC_HOSTNAME:-tickets.example.com}
  CLOUDFLARE_TUNNEL_NAME=${CLOUDFLARE_TUNNEL_NAME:-cets-baremetal}

  NODES=("$BAREMETAL_NODE_1" "$BAREMETAL_NODE_2" "$BAREMETAL_NODE_3")
  NODE_IPS=("$BAREMETAL_NODE_1_IP" "$BAREMETAL_NODE_2_IP" "$BAREMETAL_NODE_3_IP")
  mkdir -p "$GENERATED_DIR"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

require_env() {
  local name=$1
  [ -n "${!name:-}" ] || die "$name is required; set it in $LOCAL_ENV"
}

require_apply() {
  [ "${APPLY:-false}" = "true" ] || die "set APPLY=true to perform this change"
}

ssh_base() {
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=8 "$@"
}

is_local_node() {
  local node=$1
  local host
  host=$(hostname)
  [ "$node" = "$host" ] || [ "$node" = "localhost" ] || [ "$node" = "127.0.0.1" ]
}

ssh_node() {
  local node=$1
  shift
  if is_local_node "$node"; then
    bash -lc "$*"
  else
    ssh_base "${BAREMETAL_SSH_USER}@${node}" "$@"
  fi
}

scp_node() {
  scp -o BatchMode=yes -o StrictHostKeyChecking=accept-new "$@"
}

copy_to_node() {
  local src=$1
  local node=$2
  local dest=$3
  if is_local_node "$node"; then
    cp "$src" "$dest"
  else
    scp_node "$src" "${BAREMETAL_SSH_USER}@${node}:$dest"
  fi
}

sudo_remote() {
  local node=$1
  shift
  require_env BAREMETAL_BECOME_PASSWORD
  if is_local_node "$node"; then
    printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | sudo -S bash -lc "$*"
  else
    printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" |
      ssh_node "$node" "sudo -S bash -lc $(printf '%q' "$*")"
  fi
}

kubectl_bm() {
  KUBECONFIG=${KUBECONFIG:-$HOME/.kube/config} kubectl "$@"
}

set_local_env_value() {
  local key=$1
  local value=$2
  [ -f "$LOCAL_ENV" ] || die "$LOCAL_ENV does not exist; run 05-init-local-env.sh first"
  if grep -q "^${key}=" "$LOCAL_ENV"; then
    awk -v key="$key" -v value="$value" 'BEGIN{updated=0} $0 ~ "^" key "=" { print key "=" value; updated=1; next } { print } END{ if (!updated) print key "=" value }' "$LOCAL_ENV" >"$LOCAL_ENV.tmp"
    mv "$LOCAL_ENV.tmp" "$LOCAL_ENV"
  else
    printf '%s=%s\n' "$key" "$value" >>"$LOCAL_ENV"
  fi
}

tf_bin() {
  if command -v terraform >/dev/null 2>&1; then
    printf terraform
  elif command -v tofu >/dev/null 2>&1; then
    printf tofu
  else
    die "terraform or tofu is required"
  fi
}
