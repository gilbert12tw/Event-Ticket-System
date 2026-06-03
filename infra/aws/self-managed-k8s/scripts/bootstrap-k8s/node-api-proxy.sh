# shellcheck shell=bash

node_api_proxy_command() {
  local backends=""
  local index ip
  for index in "${!PRIVATE_IPS[@]}"; do
    ip=${PRIVATE_IPS[$index]}
    [ "$ip" = "None" ] && continue
    backends="$backends
  server cp$((index + 1)) $ip:6443 check"
  done

cat <<EOF
set -euo pipefail
tmp_hosts=\$(mktemp)
awk -v host="$API_HOST" '
  {
    keep = 1
    for (i = 2; i <= NF; i++) {
      if (\$i == host) {
        keep = 0
      }
    }
    if (keep) {
      print
    }
  }
' /etc/hosts >"\$tmp_hosts"
cat "\$tmp_hosts" >/etc/hosts
rm -f "\$tmp_hosts"
printf '127.0.0.1 %s\n' "$API_HOST" >>/etc/hosts
cat >/etc/haproxy/haproxy.cfg <<'HAPROXY'
global
  log /dev/log local0
  user haproxy
  group haproxy
  daemon

defaults
  log global
  mode tcp
  timeout connect 5s
  timeout client 60s
  timeout server 60s

frontend kube_apiserver
  bind 127.0.0.1:6444
  default_backend kube_apiserver

backend kube_apiserver
  balance roundrobin
$backends
HAPROXY
systemctl enable haproxy
systemctl restart haproxy
EOF
}
