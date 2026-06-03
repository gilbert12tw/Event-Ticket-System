require_running() {
  service=$1
  container=$(compose ps -q "$service")
  [ -n "$container" ] || die "$service has no container"
  state=$(docker inspect --format '{{.State.Status}}' "$container")
  [ "$state" = "running" ] || die "$service is $state"
}

check_datasource() {
  uid=$1
  curl -fsS -u "$GRAFANA_ADMIN_USER:$GRAFANA_ADMIN_PASSWORD" "$GRAFANA_URL/api/datasources/uid/$uid" |
    grep -Eq "\"uid\"[[:space:]]*:[[:space:]]*\"$uid\"" ||
    die "Grafana datasource $uid is not provisioned"
}

wait_http_grep() {
  label=$1
  url=$2
  pattern=$3
  for _ in $(seq 1 24); do
    if curl -fsS "$url" 2>/dev/null | grep -Eqi "$pattern"; then
      return
    fi
    sleep 5
  done
  die "$label did not become ready"
}

check_lgtm_health() {
  log "checking LGTM services and datasource provisioning"
  for service in grafana prometheus loki tempo pyroscope alloy; do
    require_running "$service"
  done
  wait_http_grep "Grafana" "$GRAFANA_URL/api/health" '"database"[[:space:]]*:[[:space:]]*"ok"'
  wait_http_grep "Prometheus" "$PROMETHEUS_URL/-/ready" "Prometheus Server is Ready"
  wait_http_grep "Loki" "$LOKI_URL/ready" "^ready$"
  wait_http_grep "Tempo" "$TEMPO_URL/ready" "ready"
  for _ in $(seq 1 24); do
    if curl -fsS "$PYROSCOPE_URL/ready" >/dev/null 2>&1 ||
      curl -fsS "$PYROSCOPE_URL/-/ready" >/dev/null 2>&1; then
      break
    fi
    sleep 5
  done
  curl -fsS "$PYROSCOPE_URL/ready" >/dev/null 2>&1 ||
    curl -fsS "$PYROSCOPE_URL/-/ready" >/dev/null 2>&1 ||
    die "Pyroscope did not report ready"
  for uid in Prometheus Loki Tempo Pyroscope; do
    check_datasource "$uid"
  done
}
