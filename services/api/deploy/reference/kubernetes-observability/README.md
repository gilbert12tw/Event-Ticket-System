# Kubernetes Observability Reference

These manifests are reference-only examples for a future container-platform decision gate. They are
not loaded by local Docker Compose, not mounted into the application, and not required for current
release gates.

The reference covers the orchestration-state signals from the observability deck without changing
product runtime behavior:

- `kube-state-metrics.yaml` deploys kube-state-metrics with read-only RBAC for workload, service,
  autoscaling, node, namespace, and persistent-volume state.
- `node-exporter-daemonset.yaml` shows the one-exporter-per-node DaemonSet shape for OS metrics.
- `app-metrics-sidecar-example.yaml` shows the sidecar pattern for an app-specific exporter in the
  same pod as an application container.
- `metrics-stack.yaml` shows a containerized Prometheus + Grafana metrics stack: Prometheus is a
  StatefulSet with a persistent volume claim for TSDB data, while Grafana stays stateless and uses
  Prometheus as its data source.
- `prometheus-scrape-example.yaml` shows the Prometheus scrape job shape for
  `kube-state-metrics.cets-observability.svc:8080`.
- `log-sidecar-example.yaml` shows the container log forwarding sidecar pattern: the app still
  writes stdout, while a Fluent Bit sidecar tails a shared stdout spool and forwards structured logs.
- `plg-log-stack.yaml` shows a Loki + Grafana log analysis/viewing stack for the PLG pattern.
- `efk-log-stack.yaml` shows Elasticsearch + Kibana analysis/viewing resources for the EFK pattern.

Together, these files keep the logging and metrics stack as code for review without making it the
active deployment path.

Keep application behavior independent from these files. Booking, ticket, check-in, worker,
reporting, and audit correctness still rely on PostgreSQL-backed application contracts, not
observability data.
