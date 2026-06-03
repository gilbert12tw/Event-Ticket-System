check_service_graph() {
  log "checking Tempo service graph metrics"
  for _ in $(seq 1 12); do
    inbound_response=$(prom_query "$SERVICE_GRAPH_INBOUND_QUERY" 2>/dev/null || true)
    inbound_value=$(printf '%s\n' "$inbound_response" | json_scalar_value)
    backend_dependency_response=$(prom_query "$SERVICE_GRAPH_BACKEND_DEPENDENCY_QUERY" 2>/dev/null || true)
    backend_dependency_value=$(printf '%s\n' "$backend_dependency_response" | json_scalar_value)
    if [ -n "$inbound_value" ] &&
      [ -n "$backend_dependency_value" ] &&
      awk -v before="$SERVICE_GRAPH_INBOUND_BASELINE" -v after="$inbound_value" 'BEGIN { exit(after > before ? 0 : 1) }' &&
      awk -v before="$SERVICE_GRAPH_BACKEND_DEPENDENCY_BASELINE" -v after="$backend_dependency_value" 'BEGIN { exit(after > before ? 0 : 1) }'; then
      prom_query 'sum by (client,server) (increase(traces_service_graph_request_total[15m]))' >"$SERVICE_GRAPH_EVIDENCE"
      {
        printf '# cets-backend inbound service graph counter before load\n%s\n' "$SERVICE_GRAPH_INBOUND_BASELINE"
        printf '\n# cets-backend inbound service graph counter after load\n'
        printf '%s\n' "$inbound_response"
        printf '\n# cets-backend outbound dependency service graph counter before load\n%s\n' "$SERVICE_GRAPH_BACKEND_DEPENDENCY_BASELINE"
        printf '\n# cets-backend outbound dependency service graph counter after load\n'
        printf '%s\n' "$backend_dependency_response"
        printf '\n# cets-backend outbound dependency service graph edges\n'
        prom_query 'sum by (client,server) (increase(traces_service_graph_request_total{client="cets-backend"}[15m]))'
      } >"$SERVICE_GRAPH_BACKEND_DEPENDENCY_EVIDENCE"
      return
    fi
    http_get "$EDGE_URL/readyz"
    sleep 5
  done
  die "Prometheus did not return service graph metrics for cets-backend inbound and outbound dependency edges"
}

capture_service_graph_baseline() {
  log "capturing Tempo service graph baseline"
  for _ in $(seq 1 12); do
    inbound_response=$(prom_query "$SERVICE_GRAPH_INBOUND_QUERY" 2>/dev/null || true)
    backend_dependency_response=$(prom_query "$SERVICE_GRAPH_BACKEND_DEPENDENCY_QUERY" 2>/dev/null || true)
    if printf '%s\n' "$inbound_response" | grep -q '"status":"success"' &&
      printf '%s\n' "$backend_dependency_response" | grep -q '"status":"success"'; then
      inbound_value=$(printf '%s\n' "$inbound_response" | json_scalar_value)
      backend_dependency_value=$(printf '%s\n' "$backend_dependency_response" | json_scalar_value)
      if [ -n "$inbound_value" ]; then
        SERVICE_GRAPH_INBOUND_BASELINE=$inbound_value
      fi
      if [ -n "$backend_dependency_value" ]; then
        SERVICE_GRAPH_BACKEND_DEPENDENCY_BASELINE=$backend_dependency_value
      fi
      return
    fi
    sleep 5
  done
  die "Prometheus service graph baseline could not be read"
}
