# Kubernetes Observability Reference

These manifests are reference-only examples for a future container-platform decision gate. They are
not loaded by local Docker Compose, not mounted into the application, and not required for current
release gates.

The reference covers the orchestration-state signals from the observability deck without changing
product runtime behavior:

- `kube-state-metrics.yaml` deploys kube-state-metrics with read-only RBAC for workload, service,
  autoscaling, node, namespace, and persistent-volume state.
- `prometheus-scrape-example.yaml` shows the Prometheus scrape job shape for
  `kube-state-metrics.cets-observability.svc:8080`.

Keep application behavior independent from these files. Booking, ticket, check-in, worker,
reporting, and audit correctness still rely on PostgreSQL-backed application contracts, not
observability data.
