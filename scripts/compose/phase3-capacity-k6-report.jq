def metric_value($name; $field):
  (.metrics[$name][$field] // "n/a");

def metric_rate($name):
  (.metrics[$name].rate // .metrics[$name].value);

[
  ["target RPS", $target_rps],
  ["HTTP throughput req/s", (metric_rate("http_reqs") // "n/a")],
  ["HTTP request count", metric_value("http_reqs"; "count")],
  ["unexpected error rate", (metric_rate("http_req_failed{expected_error:false}") // metric_rate("http_req_failed") // "n/a")],
  ["check pass rate", (metric_rate("checks") // "n/a")],
  ["dropped iterations", metric_value("dropped_iterations"; "count")],
  ["http_req_duration p50", metric_value("http_req_duration"; "med")],
  ["http_req_duration p95", metric_value("http_req_duration"; "p(95)")],
  ["http_req_duration p99", metric_value("http_req_duration"; "p(99)")],
  ["read flow p50", metric_value("http_req_duration{flow:read}"; "med")],
  ["read flow p95", metric_value("http_req_duration{flow:read}"; "p(95)")],
  ["read flow p99", metric_value("http_req_duration{flow:read}"; "p(99)")],
  ["booking p50", metric_value("k8s_booking_duration"; "med")],
  ["booking p95", metric_value("k8s_booking_duration"; "p(95)")],
  ["booking p99", metric_value("k8s_booking_duration"; "p(99)")],
  ["booking attempts", (.metrics.k8s_booking_attempts.count // "n/a")],
  ["booking confirmed", (.metrics.k8s_booking_confirmed.count // "n/a")],
  ["booking waitlisted", (.metrics.k8s_booking_waitlisted.count // "n/a")],
  ["booking conflicts", (.metrics.k8s_booking_conflicts.count // "n/a")],
  ["gateway replica hit samples", (.metrics.k8s_gateway_replica_hits.count // "n/a")],
  ["frontend replica hit samples", (.metrics.k8s_frontend_replica_hits.count // "n/a")],
  ["backend replica hit samples", (.metrics.k8s_backend_replica_hits.count // "n/a")]
] | .[] | "- \(.[0]): `\(.[1])`"
