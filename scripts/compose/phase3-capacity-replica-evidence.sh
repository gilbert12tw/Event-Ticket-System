write_replica_spread() {
  rps=$1
  sample_file="$ARTIFACT_DIR/capacity-replica-headers-$RUN_ID-rps-$rps.txt"
  summary_file="$ARTIFACT_DIR/capacity-replica-spread-$RUN_ID-rps-$rps.txt"
  : >"$sample_file"
  for _ in $(seq 1 "$REPLICA_SAMPLES"); do
    curl -fsSI "$BASE_URL/readyz" >>"$sample_file"
    printf '\n' >>"$sample_file"
  done
  gateway=$(awk -v header=X-CETS-Gateway-Replica -f "$HEADER_REPLICAS_AWK" "$sample_file")
  frontend=$(awk -v header=X-CETS-Frontend-Replica -f "$HEADER_REPLICAS_AWK" "$sample_file")
  backend=$(awk -v header=X-CETS-Backend-Replica -f "$HEADER_REPLICAS_AWK" "$sample_file")
  {
    printf 'gateway|%s\n' "$gateway"
    printf 'frontend|%s\n' "$frontend"
    printf 'backend|%s\n' "$backend"
    printf 'headers|%s\n' "$sample_file"
  } >"$summary_file"
  log "observed capacity replicas: gateway=$gateway frontend=$frontend backend=$backend"
  require_replica_spread_summary "$summary_file"
}

print_replica_spread() {
  summary=$1
  awk -F'|' '$1 != "headers" { printf "- %s distinct replicas: `%s`\n", $1, $2 }' "$summary"
}

require_replica_spread_summary() {
  summary=$1
  awk -f "$REPLICA_SPREAD_CHECK" "$summary" || die "capacity replica spread invariants failed"
}
