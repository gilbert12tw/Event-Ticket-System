check_profile_data() {
  log "checking Pyroscope profile data"
  for _ in $(seq 1 12); do
    profile=$(curl -fsS --get \
      --data-urlencode 'query=process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="cets-backend"}' \
      --data-urlencode 'from=now-1h' \
      --data-urlencode 'until=now' \
      --data-urlencode 'maxNodes=64' \
      "$PYROSCOPE_URL/pyroscope/render" 2>/dev/null || true)
    if printf '%s\n' "$profile" | grep -Eq '"numTicks":[0-9]*[1-9][0-9]*'; then
      printf '%s\n' "$profile" >"$PYROSCOPE_PROFILE_EVIDENCE"
      return
    fi
    sleep 5
  done
  die "Pyroscope did not return cets-backend profile samples"
}
