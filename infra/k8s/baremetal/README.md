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
