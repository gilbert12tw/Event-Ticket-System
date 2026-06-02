#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

load_env
require_apply
require_env BAREMETAL_BECOME_PASSWORD

log "installing local control-plane tools on $(hostname)"
printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | sudo -S bash -lc '
set -euo pipefail
rm -f /etc/apt/sources.list.d/helm-stable-debian.list
apt-get update
apt-get install -y ca-certificates curl gpg apt-transport-https jq git make openssl

install -m 0755 -d /etc/apt/keyrings
if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
fi
. /etc/os-release
cat >/etc/apt/sources.list.d/docker.list <<EOF
deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable
EOF

rm -f /etc/apt/keyrings/opentofu.gpg /etc/apt/keyrings/opentofu-repo.gpg
curl -fsSL https://get.opentofu.org/opentofu.gpg >/etc/apt/keyrings/opentofu.gpg
curl -fsSL https://packages.opentofu.org/opentofu/tofu/gpgkey |
  gpg --no-tty --batch --yes --dearmor -o /etc/apt/keyrings/opentofu-repo.gpg
chmod a+r /etc/apt/keyrings/opentofu.gpg /etc/apt/keyrings/opentofu-repo.gpg
cat >/etc/apt/sources.list.d/opentofu.list <<EOF
deb [signed-by=/etc/apt/keyrings/opentofu.gpg,/etc/apt/keyrings/opentofu-repo.gpg] https://packages.opentofu.org/opentofu/tofu/any/ any main
EOF

apt-get update
apt-get install -y docker-ce docker-ce-cli docker-buildx-plugin tofu
systemctl enable --now docker

if ! command -v helm >/dev/null 2>&1; then
  tmpdir=$(mktemp -d)
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 -o "$tmpdir/get-helm-3"
  chmod 700 "$tmpdir/get-helm-3"
  "$tmpdir/get-helm-3"
  rm -rf "$tmpdir"
fi
'

if ! id -nG "$USER" | tr " " "\n" | grep -qx docker; then
  printf '%s\n' "$BAREMETAL_BECOME_PASSWORD" | sudo -S usermod -aG docker "$USER"
  log "added $USER to docker group; current shell may still require sudo docker until next login"
fi

log "local control tools installed"
