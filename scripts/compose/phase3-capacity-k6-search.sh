run_k6_candidate() {
  rps=$1
  out="$ARTIFACT_DIR/k6-$RUN_ID-rps-$rps.json"
  log "running k6 candidate rps=$rps duration=$DURATION base=$BASE_URL"
  docker run --rm --network host \
    -v "$ROOT_DIR/k6:/k6:ro" \
    -v "$ARTIFACT_DIR:/artifacts" \
    -e BASE_URL="$BASE_URL" \
    -e K6_PROVIDER_TOKEN_SECRET="$PROVIDER_TOKEN_SECRET" \
    -e K6_TARGET_RPS="$rps" \
    -e K6_CAPACITY_DURATION="$DURATION" \
    -e K6_READ_RATIO="$READ_RATIO" \
    -e K6_EMPLOYEE_PREFIX="$EMPLOYEE_PREFIX" \
    -e K6_EMPLOYEE_COUNT="$EMPLOYEE_COUNT" \
    -e K6_HOT_EVENT_CAPACITY="$HOT_EVENT_CAPACITY" \
    -e K6_RUN_ID="$RUN_ID-rps-$rps" \
    "$K6_IMAGE" run --summary-export "/artifacts/$(basename "$out")" "$SCRIPT" >&2
}

candidate_passes() {
  rps=$1
  if run_k6_candidate "$rps"; then
    write_replica_spread "$rps"
    printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-pass-rps.txt"
    printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-pass-rps-$RUN_ID.txt"
    return 0
  fi
  printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-fail-rps.txt"
  printf '%s\n' "$rps" >"$ARTIFACT_DIR/last-fail-rps-$RUN_ID.txt"
  return 1
}

find_max_rps() {
  low=0
  high=0
  current=$START_RPS

  while [ "$current" -le "$MAX_RPS" ]; do
    if candidate_passes "$current"; then
      low=$current
      current=$((current + STEP_RPS))
    else
      high=$current
      break
    fi
  done

  if [ "$high" -eq 0 ]; then
    printf '%s\n' "$low"
    return
  fi

  while [ $((high - low)) -gt "$RESOLUTION_RPS" ]; do
    mid=$(((low + high) / 2))
    if candidate_passes "$mid"; then
      low=$mid
    else
      high=$mid
    fi
  done
  printf '%s\n' "$low"
}
