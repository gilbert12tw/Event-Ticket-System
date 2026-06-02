# shellcheck shell=bash

deploy_observability() {
log "installing observability stack"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
helm repo add grafana https://grafana.github.io/helm-charts >/dev/null
helm repo update >/dev/null
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace observability \
  --version 86.1.0 \
  --set grafana.enabled=true \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
  --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false
cat >"$GENERATED_DIR/alloy-values.yaml" <<EOF
alloy:
  extraEnv:
  - name: NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
  configMap:
    content: |-
      logging {
        level  = "info"
        format = "logfmt"
      }

      discovery.kubernetes "cets_pods" {
        role = "pod"
        selectors {
          role  = "pod"
          field = "spec.nodeName=" + sys.env("NODE_NAME")
        }
      }

      discovery.relabel "cets_pod_logs" {
        targets = discovery.kubernetes.cets_pods.targets

        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          regex         = "$CETS_NAMESPACE"
          action        = "keep"
        }
        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          target_label  = "namespace"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_name"]
          target_label  = "pod"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_container_name"]
          target_label  = "container"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_label_app"]
          target_label  = "app"
        }
        rule {
          source_labels = ["__meta_kubernetes_namespace", "__meta_kubernetes_pod_container_name"]
          separator     = "/"
          target_label  = "job"
        }
      }

      loki.source.kubernetes "cets_pod_logs" {
        targets    = discovery.relabel.cets_pod_logs.output
        forward_to = [loki.process.cets_pod_logs.receiver]
      }

      loki.process "cets_pod_logs" {
        stage.static_labels {
          values = {
            cluster = "cets-baremetal",
          }
        }
        forward_to = [loki.write.local.receiver]
      }

      loki.write "local" {
        endpoint {
          url = "http://loki-gateway.observability.svc.cluster.local/loki/api/v1/push"
        }
      }
EOF
helm upgrade --install alloy grafana/alloy \
  --namespace observability \
  --version 1.8.2 \
  -f "$GENERATED_DIR/alloy-values.yaml"
helm upgrade --install tempo grafana/tempo --namespace observability --version 1.24.4
helm upgrade --install pyroscope grafana/pyroscope --namespace observability --version 2.0.2

cat >"$GENERATED_DIR/loki-values.yaml" <<EOF
deploymentMode: SingleBinary
loki:
  auth_enabled: false
  commonConfig:
    replication_factor: 1
  storage:
    type: filesystem
  schemaConfig:
    configs:
    - from: "2024-04-01"
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: loki_index_
        period: 24h
singleBinary:
  replicas: 1
  persistence:
    enabled: true
    size: 10Gi
backend:
  replicas: 0
read:
  replicas: 0
write:
  replicas: 0
ingester:
  replicas: 0
querier:
  replicas: 0
queryFrontend:
  replicas: 0
queryScheduler:
  replicas: 0
distributor:
  replicas: 0
compactor:
  replicas: 0
indexGateway:
  replicas: 0
bloomPlanner:
  replicas: 0
bloomBuilder:
  replicas: 0
bloomGateway:
  replicas: 0
chunksCache:
  enabled: false
resultsCache:
  enabled: false
minio:
  enabled: false
EOF

helm upgrade --install loki grafana/loki \
  --namespace observability \
  --version 7.0.0 \
  -f "$GENERATED_DIR/loki-values.yaml"

cat >"$GENERATED_DIR/cets-observability.yaml" <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: cets-backend
  namespace: $CETS_NAMESPACE
  labels:
    release: kube-prometheus-stack
spec:
  selector:
    matchLabels:
      app: backend
  endpoints:
  - port: http
    path: /metrics
    interval: 15s
---
apiVersion: v1
kind: Service
metadata:
  name: backend-metrics
  namespace: $CETS_NAMESPACE
  labels:
    app: backend
spec:
  selector:
    app: backend
  ports:
  - name: http
    port: 8080
    targetPort: 8080
EOF
kubectl_bm apply -f "$GENERATED_DIR/cets-observability.yaml" || true
}
