# Bare-Metal Kubernetes HA Deployment

This track deploys Event Ticket System Phase 3 on three internal VMs without AWS.
It is designed to be reproducible from scripts and to keep public HTTPS access
behind Cloudflare Tunnel and Cloudflare Access.

## Architecture

- `work1` / `10.121.124.200`, `work2` / `10.121.124.201`, `work3` / `10.121.124.202`
- Upstream `kubeadm` HA cluster with stacked `etcd`; all three nodes are control-plane and worker nodes.
- `kube-vip` owns the Kubernetes API VIP, default `10.121.124.210:6443`.
- Calico provides pod networking.
- `ingress-nginx` is the in-cluster HTTP entrypoint.
- `cloudflared` runs as three Kubernetes replicas and exposes the app through Cloudflare Tunnel
  without requiring router changes.
- Cloudflare Access protects the hostname with identity and WARP device posture policy.
- CloudNativePG runs PostgreSQL HA with required synchronous replication to one standby. Redis,
  MinIO, and Mailhog are demo dependencies unless promoted later.

## One-Time Inputs

Copy `.env.example` to `.env.baremetal.local` and fill secrets.

```sh
cp infra/k8s/baremetal/.env.example infra/k8s/baremetal/.env.baremetal.local
```

Or generate the local file from the workspace sudo password and generated
application secrets:

```sh
infra/k8s/baremetal/scripts/05-init-local-env.sh
```

The domain must be active in Cloudflare before `40-cloudflare.sh` applies DNS,
Tunnel, and Access resources. If the domain is not active yet, add it to
Cloudflare and update the registrar nameservers first.

## Script Order

All scripts are guarded. Destructive or external changes require `APPLY=true`.

```sh
infra/k8s/baremetal/scripts/00-preflight.sh
infra/k8s/baremetal/scripts/05-init-local-env.sh
APPLY=true infra/k8s/baremetal/scripts/08-install-control-tools.sh
APPLY=true infra/k8s/baremetal/scripts/10-host-prepare.sh
APPLY=true infra/k8s/baremetal/scripts/20-bootstrap-kubeadm.sh
APPLY=true infra/k8s/baremetal/scripts/30-networking.sh
infra/k8s/baremetal/scripts/41-cloudflare-preflight.sh
APPLY=true infra/k8s/baremetal/scripts/40-cloudflare.sh
APPLY=true infra/k8s/baremetal/scripts/45-build-images.sh
APPLY=true infra/k8s/baremetal/scripts/50-deploy-cets.sh
infra/k8s/baremetal/scripts/60-verify.sh
infra/k8s/baremetal/scripts/61-verify-k8s-ha.sh
infra/k8s/baremetal/scripts/62-verify-cloudflare.sh
infra/k8s/baremetal/scripts/63-verify-app-smoke.sh
infra/k8s/baremetal/scripts/64-benchmark-capacity.sh
infra/k8s/baremetal/scripts/66-verify-observability.sh
infra/k8s/baremetal/scripts/67-verify-telemetry-redaction.sh
infra/k8s/baremetal/scripts/70-failure-drill.sh
infra/k8s/baremetal/scripts/71-verify-postgres-failover.sh
```

For normal application code updates, build images from the current Git commit
and roll the Kubernetes workloads in one guarded step:

```sh
APPLY=true infra/k8s/baremetal/scripts/46-update-images-and-rollout.sh
```

The script refuses dirty worktrees, `latest`, and non-hash image tags. By
default it imports `cets-api:<git-hash>` and `cets-frontend:<git-hash>` into
the cluster nodes. To push to a registry instead, set
`CETS_API_IMAGE_REPOSITORY`, `CETS_FRONTEND_IMAGE_REPOSITORY`, and
`CETS_PUSH_IMAGES=true` in `.env.baremetal.local`.

## Capacity Benchmark

Use `64-benchmark-capacity.sh` for the production-like maximum RPS evidence.
It runs k6 through ingress with an 80/20 read/booking traffic mix, seeds
idempotent benchmark employees, searches for the highest passing RPS, and
writes JSON plus a markdown report under `artifacts/k8s-capacity/`.

```sh
K8S_BENCH_START_RPS=100 \
K8S_BENCH_STEP_RPS=100 \
K8S_BENCH_MAX_RPS=2000 \
infra/k8s/baremetal/scripts/64-benchmark-capacity.sh
```

## Error Rate Demo

Use `69-demo-error-rate.sh` when you intentionally want RED/error panels to
show elevated 4xx traffic. It starts at 350 RPS for 120 seconds by default and
marks the synthetic 4xx requests as expected in k6 so the demo can complete
while application metrics still show the error-rate spike.

```sh
infra/k8s/baremetal/scripts/69-demo-error-rate.sh
```

Tune the default blast with `K8S_ERROR_DEMO_TARGET_RPS`,
`K8S_ERROR_DEMO_DURATION`, and `K8S_ERROR_DEMO_ERROR_RATIO`.

Run `66-verify-observability.sh` after a benchmark window to prove the
Grafana/LGTM investigation path: RED metrics, Tempo trace, Loki logs for the
same trace ID, Pyroscope CPU profile samples, and service graph data involving
`cets-backend`.

To expose both the ticket app and Grafana through Cloudflare Access, set
`CETS_PUBLIC_HOSTNAME` and `GRAFANA_PUBLIC_HOSTNAME` in `.env.baremetal.local`,
then run `41-cloudflare-preflight.sh`, `APPLY=true 40-cloudflare.sh`, and
`APPLY=true 50-deploy-cets.sh`. The single Tunnel routes the app hostname to
ingress-nginx and the Grafana hostname to
`kube-prometheus-stack-grafana.observability.svc.cluster.local:80`.
Set `CLOUDFLARE_ACCESS_ENABLED=false` only when both hostnames should be public
without Cloudflare Access login; Grafana's own login page will then be exposed
to the internet.

For a demo-only public ticket app with mock profile selection, set
`CETS_APP_ENV=demo` in `.env.baremetal.local` and rerun
`APPLY=true infra/k8s/baremetal/scripts/50-deploy-cets.sh`. Leave the default
`CETS_APP_ENV=baremetal` for deployments that must require real provider
claims.

The deployment also provisions the `Cets` Grafana folder through the
kube-prometheus-stack dashboard sidecar. The folder contains separate
dashboards for `CETS Metrics RED`, `CETS Metrics USE`, `CETS Logs`,
`CETS Traces`, and `CETS Profiles`. To inspect them locally:

```sh
kubectl -n observability port-forward svc/kube-prometheus-stack-grafana 3000:80
kubectl -n observability get secret kube-prometheus-stack-grafana \
  -o jsonpath='{.data.admin-password}' | base64 -d; echo
```

Open `http://127.0.0.1:3000`, sign in as `admin`, then open the `Cets` folder.
Treat `66-verify-observability.sh` as the data-level acceptance check when
dashboard panels are empty after a quiet traffic window. The `CETS Traces`
dashboard separates Tempo-derived service graph data from K8s deployment
topology. Current routing sends UI/static routes (`/`) from ingress-nginx to
frontend, while API/health/ready routes (`/api`, `/healthz`, `/readyz`) go
directly from ingress-nginx to backend. ingress-nginx emits spans, but its
OpenTelemetry module may not emit backend proxy client spans, so Tempo may show
`user -> ingress-nginx` and `user -> cets-backend` instead of an
`ingress-nginx -> cets-backend` edge. The frontend node remains topology-only
until a future instrumented frontend or gateway emits spans.

For a full automated audit, use:

```sh
infra/k8s/baremetal/scripts/99-verify-all.sh
```

`99-verify-all.sh` defaults to the Cloudflare-only public HA path, so router BGP is not required.
The approved browser check is still required unless explicitly disabled for an automated audit:

```sh
REQUIRE_APPROVED_BROWSER=false infra/k8s/baremetal/scripts/99-verify-all.sh
```

Disruptive drills are skipped by default in `99-verify-all.sh`. Include them with:

```sh
RUN_FAILURE_DRILLS=true infra/k8s/baremetal/scripts/99-verify-all.sh
```

## Optional Router Diagnostics

Router BGP/ECMP is not required for the accepted public endpoint. Cloudflare Tunnel traffic is
balanced by available `cloudflared` connectors and Kubernetes services instead of router ECMP.

This environment's gateway identifies as MikroTik RouterOS. Generate a reviewable
RouterOS v7 BGP script with:

```sh
infra/k8s/baremetal/scripts/35-render-routeros-bgp.sh
```

Review and apply `infra/k8s/baremetal/generated/routeros-metallb-bgp.rsc` on
the router only if router access becomes available, then run `65-verify-bgp.sh`.
