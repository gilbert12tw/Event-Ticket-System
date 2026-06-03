write_correctness_summary() {
  best_rps=$1
  output=$2
  event_title="k8s capacity $RUN_ID-rps-$best_rps"
  psql_with_event_title "$event_title" "$CORRECTNESS_SUMMARY_SQL" >"$output"
  [ -s "$output" ] || die "could not write post-load correctness summary for '$event_title'"
}

require_post_load_correctness() {
  summary=$1
  awk -f "$CORRECTNESS_CHECK" "$summary" || die "post-load correctness invariants failed"
}

print_correctness_summary() {
  summary=$1
  awk -F'|' '{ label = $1; gsub(/_/, " ", label); printf "- %s: `%s`\n", label, $2 }' "$summary"
}
