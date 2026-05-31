# Phase 3 Bare-Metal Kubernetes Cloudflare HA Goal

## Objective

Deploy Event-Ticket-System Phase 3 on the internal three-VM Kubernetes cluster and keep iterating
until one Cloudflare HTTPS hostname is usable, protected, highly available without router/BGP
changes, observable, and verified by failure drills.

Primary deployment path: `infra/k8s/baremetal`.

AWS and router/BGP work are not active goals. Do not use AWS credentials or AWS infrastructure unless
the project owner explicitly abandons this bare-metal Cloudflare path later.

## Target Infrastructure

| Node | IP | Role |
| --- | --- | --- |
| `work1` | `10.121.124.200` | Kubernetes control-plane + worker |
| `work2` | `10.121.124.201` | Kubernetes control-plane + worker |
| `work3` | `10.121.124.202` | Kubernetes control-plane + worker |

Core platform:

- Upstream `kubeadm` HA cluster with stacked `etcd`.
- `containerd` runtime.
- `kube-vip` Kubernetes API VIP: `10.121.124.210:6443`.
- Calico CNI.
- `ingress-nginx` as the in-cluster HTTP entrypoint.
- Cloudflare Tunnel as the public HA path, with three `cloudflared` replicas spread one per VM.
- Cloudflare Access + WARP device posture so only approved identities and enrolled computers can
  use the browser endpoint.
- CloudNativePG for PostgreSQL HA.
- Redis, MinIO, and Mailhog as demo dependencies unless separately promoted to HA services.
- Grafana, Prometheus, Loki, Tempo, Pyroscope, and Alloy-compatible collection for LGTM evidence.

Accepted public traffic path:

```text
Browser
  -> Cloudflare Access
  -> Cloudflare Tunnel
  -> 3 cloudflared replicas
  -> ingress-nginx service
  -> frontend/backend pods
```

This path intentionally does not require router BGP, ECMP, inbound firewall exposure, paid
Cloudflare Load Balancing, or private LAN routing from user devices. Cloudflare Tunnel replicas give
connector availability and retry across available connectors; they do not provide Cloudflare Load
Balancer health-check steering. If active origin health-check steering is later required, add paid
Cloudflare Load Balancing as a separate decision and cost gate.

## Operating Rules

- Treat completion as unproven until every required evidence item below has current evidence.
- Keep iterating through install, fix, verify, and drill until the goal is complete or a concrete
  blocker is recorded.
- Do not mark the goal complete from intent, partial installation, a single healthy pod, or a narrow
  smoke test.
- Use untracked local files only for secrets and environment-specific values:
  - `infra/k8s/baremetal/.env.baremetal.local`
  - `infra/k8s/baremetal/cloudflare/terraform.tfvars.json`
  - generated manifests under `infra/k8s/baremetal/generated/`
- Never commit sudo passwords, Cloudflare tokens, Tunnel tokens, Terraform/OpenTofu state, real
  application secrets, or provider credentials.
- Treat the previously pasted Cloudflare API token as exposed. Rotate or revoke it after
  verification and update only the untracked local env file.
- Prefer scripted, reproducible changes. If a manual Cloudflare or approved-browser step is
  required, record the exact step and evidence.
- Delegate small, independent review or mapping tasks to `gpt-5.3-codex-spark` when useful, but keep
  PostgreSQL durability and routing decisions in the main implementation path.

## Required Apply Order

Run the deployment through the reproducible script track:

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
REQUIRE_BGP=false infra/k8s/baremetal/scripts/99-verify-all.sh
```

Before any apply/install step, confirm the needed local inputs exist in
`infra/k8s/baremetal/.env.baremetal.local`. If an input is missing, stop and record the exact
missing variable instead of guessing a secret or production value.

`35-render-routeros-bgp.sh` and `65-verify-bgp.sh` are optional router-owned diagnostics only. They
must not block this Cloudflare-only public HA goal because the project owner cannot change router
configuration.

## Required Verification Evidence

### Host And Kubernetes HA

- SSH and sudo work from the current workspace to `work1`, `work2`, and `work3`.
- Host prerequisites, kernel modules, sysctls, disabled swap, and `containerd` are in the expected
  Kubernetes-ready state.
- `kubectl get nodes -o wide` shows all three nodes `Ready`.
- all three nodes are control-plane nodes and schedulable for workloads.
- Kubernetes API is reachable through `10.121.124.210:6443`.
- stacked `etcd` quorum remains healthy with one control-plane node unavailable.
- kube-vip is running and can move the API VIP after a node failure.

### Cloudflare HTTPS And Access

- the domain zone is active in Cloudflare.
- Terraform/OpenTofu for `infra/k8s/baremetal/cloudflare` initializes and validates.
- Cloudflare Tunnel, DNS record, Tunnel config, Access application, and Access policy are applied
  from code.
- three `cloudflared` replicas are healthy and spread one per VM.
- the public hostname serves HTTPS through Cloudflare.
- unauthenticated or unapproved devices are denied by Cloudflare Access.
- approved identity plus WARP-enrolled approved computer can open the app in a browser.
- deleting one `cloudflared` pod and losing one VM does not break the public endpoint after normal
  Kubernetes recovery and Cloudflare connector retry.

### Event-Ticket-System Runtime

- backend deployment has three ready replicas spread one per VM.
- frontend deployment has three ready replicas spread one per VM.
- worker deployments for notification, projection, compensation, and export are healthy.
- Redis, MinIO, and Mailhog demo dependencies are reachable.
- `/healthz` and `/readyz` pass through the internal ingress and the Cloudflare HTTPS domain.
- booking, ticket, check-in, and audit smoke paths pass.
- PostgreSQL remains the source of truth for booking, ticket, check-in, and audit decisions.

### PostgreSQL HA And Durability

- CloudNativePG reports a healthy three-instance PostgreSQL cluster with one writable primary.
- The cluster uses synchronous replication with `method: any`, `number: 1`, and
  `dataDurability: required`.
- A committed booking/check-in/audit transaction survives abrupt primary pod deletion and failover.
- `pg_stat_replication` or equivalent CloudNativePG evidence shows one synchronous standby before
  failover and healthy replication after recovery.
- The documented tradeoff is accepted: writes may pause if no synchronous standby is available,
  because RPO=0 durability is preferred over continuing writes with possible data loss.

### Database Read/Write Split

The current implementation uses `DATABASE_URL` against the CloudNativePG `rw` service. That is not a
read/write split.

This goal requires a full read/write split plan before claiming database scaling maturity:

- introduce explicit `DATABASE_WRITE_URL` and `DATABASE_READ_URL` runtime configuration;
- point write traffic, migrations, seeds, workers, idempotency, booking, ticket redemption,
  check-in, eligibility finalization, and audit-sensitive reads/writes to the primary/rw path;
- point read traffic to read-only replicas only after each query path is classified safe under
  replica lag;
- start with reporting/dashboard/read-model queries as the first allowed read-replica candidates;
- add tests that fail if core consistency paths use `DATABASE_READ_URL`;
- add freshness/lag handling for any user-visible read-replica path.

Until those code and test changes exist, keep all runtime traffic on the write connection and record
"no read/write split yet" as the truthful state.

### Observability And Redaction

- Grafana is reachable through the intended protected/admin path.
- Prometheus, Loki, Tempo, Pyroscope, and Alloy-compatible collectors are deployed and healthy.
- a backend request is visible in metrics, logs, traces, and profiles.
- redaction canaries show no raw PII, signed QR tokens, provider secrets, raw idempotency keys,
  email bodies, or raw recipient email in telemetry.

### Failure Drills

- deleting one backend pod does not cause sustained endpoint failure.
- deleting one frontend pod does not cause sustained endpoint failure.
- deleting one `cloudflared` pod keeps the Tunnel available.
- draining one Kubernetes node does not break the Cloudflare HTTPS endpoint.
- abruptly stopping or rebooting one VM does not route users to a dead application endpoint after
  Kubernetes and Tunnel recovery.
- CloudNativePG failover recovers application readiness and preserves a previously committed
  transaction.
- after each drill, all three nodes return healthy and all required replicas are restored.

## Optional Router/BGP Diagnostics

MetalLB/FRR and RouterOS BGP files may remain in the repo as diagnostics or future router-owned work,
but they are not required for this Cloudflare-only public HA goal.

Current router fact pattern:

- Router `10.121.124.254` identifies as MikroTik RouterOS.
- TCP ports 22 and 80 respond.
- TCP port 179 refuses connections.
- FRR neighbors remain `Active`, proving BGP is blocked on router-side configuration.

Do not claim router ECMP, MetalLB BGP success, or internal LAN LoadBalancer HA unless
`65-verify-bgp.sh` passes after router-side work by someone with router access.

## Stop Conditions

- Stop if Cloudflare zone ownership, Tunnel creation, Access policy, or WARP approved-browser access
  cannot be verified.
- Stop if Kubernetes control-plane quorum is not healthy across the three VMs.
- Stop if PostgreSQL HA, synchronous durability, or committed-write survival cannot be proven.
- Stop if read/write split implementation would route core consistency paths to stale replicas.
- Stop if telemetry leaks any protected sensitive value.
- Stop before committing or printing any real secret.

Router BGP/ECMP failure is not a stop condition for this goal.

## Current Evidence Snapshot - 2026-05-31

Completed evidence already recorded from local runs:

- `00-preflight.sh`: work1/work2/work3 reachable, sudo works from local secret source.
- `08-install-control-tools.sh`, `10-host-prepare.sh`, and `20-bootstrap-kubeadm.sh`: control tools,
  Kubernetes host prerequisites, three stacked control-plane nodes, and kube-vip API VIP are working.
- `30-networking.sh`: Calico, MetalLB FRR-K8s, and ingress-nginx are healthy enough for current
  internal ingress checks.
- `40-cloudflare.sh`: Cloudflare Tunnel, DNS record, Access application, WARP-required posture rule,
  Access policy, and Kubernetes `cloudflared-token` secret were applied.
- `50-deploy-cets.sh`: CloudNativePG, app runtime, three `cloudflared` replicas, and observability
  stack were deployed.
- `61-verify-k8s-ha.sh`, `60-verify.sh` with `VERIFY_CLOUDFLARE=false`,
  `62-verify-cloudflare.sh`, `63-verify-app-smoke.sh`, `66-verify-observability.sh`, and
  `67-verify-telemetry-redaction.sh` passed in prior runs.
- `70-failure-drill.sh` passed backend pod deletion, one `cloudflared` pod deletion, and `work3`
  drain/restore.
- `71-verify-postgres-failover.sh` passed a primary pod deletion failover drill, but it still needs
  committed-write survival and synchronous replication evidence.
- `99-verify-all.sh` passed in fallback audit mode with
  `REQUIRE_BGP=false REQUIRE_APPROVED_BROWSER=false`; approved-browser evidence remains manual.
- Backend, frontend, and `cloudflared` have previously been verified as three ready replicas spread
  one per node across work1/work2/work3.
- Cloudflare public hostname is `tickets.sky-lab.uk`; unauthenticated curl is denied/challenged by
  Cloudflare Access.
- Direct `65-verify-bgp.sh` still fails because router `10.121.124.254:179` refuses TCP
  connections. This is out of scope for the active public HA goal.

## Completion Criteria

This goal is complete only when:

- every required apply step has completed successfully;
- every required evidence category has current evidence;
- the user-facing endpoint is one Cloudflare HTTPS domain;
- approved browser access works from the allowed WARP-enrolled computer;
- unapproved access is denied;
- application, VM, and PostgreSQL HA failure drills pass;
- PostgreSQL synchronous durability and committed-write survival are proven;
- database read/write split is either implemented with tests or explicitly recorded as not yet
  implemented;
- observability and redaction evidence exists;
- no unresolved blocker remains.
