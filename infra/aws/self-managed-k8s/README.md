# AWS Self-Managed Kubernetes

This track provisions a self-managed kubeadm Kubernetes cluster on EC2. It is
separate from the non-AWS Kubernetes deployment assets and does not use EKS,
RDS, ElastiCache, or NAT Gateway by default.

## Modes

- `NODE_COUNT=1`: one EC2 node, one control-plane plus workloads. This is the
  budget demo mode and is not highly available.
- `NODE_COUNT=3`: three EC2 nodes across subnets, stacked etcd, all nodes
  schedulable. This is the HA mode.
- `NODE_COUNT=2` is rejected because it does not provide useful etcd quorum.

The paid default is 3 On-Demand nodes at 16 GiB RAM each (`r6a.large`) with
`r5a.large` and `r6i.large` fallbacks. Spot capacity remains available as an
explicit cost option, but it is not the stability default.

## Cost Guardrails

AWS does not expose a hard real-time spend cap. This track therefore uses:

- a local cost preflight before CloudFormation apply;
- an hourly TTL cleanup rule;
- an AWS Budget notification routed through SNS to a cleanup Lambda.

The default budget is 190 USD with a 15 USD reserve and a 336-hour runtime.
The cleanup Lambda scales Auto Scaling groups to zero, empties matching ECR
repositories, and deletes CloudFormation stacks only when the prefix,
`CetsCleanupGroup`, and `Track=aws-self-managed-k8s` scope match.
The Budget excludes credits from the budget calculation so cleanup is based on
gross AWS usage, not only the post-credit bill.

AWS Budgets are account-wide in this guardrail stack. Use a dedicated or
isolated AWS account and set `DEDICATED_AWS_ACCOUNT_ACK=true` before running
`20-deploy-guardrails.sh`; the script refuses to deploy without that
acknowledgement.

## Active Runtime Cost Model

`MAX_RUNTIME_HOURS` is the stack TTL window. `ACTIVE_RUNTIME_HOURS` is the
planned VM running time inside that window. `10-cost-plan.sh` estimates
instance compute, root EBS, and public IPv4 with active hours, and estimates
API/app Network Load Balancer cost for the whole stack TTL:

`(instance + root EBS + public IPv4) x ACTIVE_RUNTIME_HOURS + NLB x MAX_RUNTIME_HOURS`.

For stable control-plane testing, prefer On-Demand. With 3 x 16 GiB
memory-optimized On-Demand nodes, use distinct ASG overrides such as
`r6a.large`, `r5a.large`, and `r6i.large`; this profile fits the default
two-week preflight budget with app NLB enabled. Spot is a cost option, not a
stability option.

With 3 x 32 GiB memory-optimized On-Demand nodes such as `r6a.xlarge`,
`r5a.xlarge`, and `r6i.xlarge`, a continuous two-week run exceeds the default
deploy limit. Use that shape only for short validation windows or reduce
`ACTIVE_RUNTIME_HOURS` through pause/resume.

## Pause / Resume Semantics

This track supports cost pause/resume by scaling control-plane and worker ASGs to
zero (pause) and restoring desired capacity later (resume). That operation is
**destructive**:

- ASG scale-to-zero terminates EC2 nodes and associated root EBS.
- Kubernetes node-local storage, including `local-path` PersistentVolumes on local
  disks, is not preserved.

Pause is therefore not data-preserving by default. For any workload needing
continuity after resume, run external backup or persistent storage paths before
scaling to zero.

## Disk And Logs

Each node uses a 120GB encrypted gp3 root volume by default. That volume holds
the OS, container images, kubelet/containerd state, `local-path` PV data, and
container stdout/stderr logs. Kubelet log rotation is configured through
`K8S_CONTAINER_LOG_MAX_SIZE=50Mi` and `K8S_CONTAINER_LOG_MAX_FILES=10`.

Application logs should still be written to stdout/stderr. For durable log
retention beyond node lifetime, add a centralized log sink such as CloudWatch or
Loki before relying on pause/resume.

## Image Builds

`BUILD_AND_PUSH_IMAGES=true` builds and pushes both app images to ECR. The
default `CETS_IMAGE_PLATFORM=linux/amd64` matches the x86_64 EC2 defaults such
as `r6a.large`; keep it set when building from Apple Silicon developer
machines. Override it only when the cluster instance types are changed to arm64.
Use `FORCE_PRIVATE_IMAGE_ROLLOUT=true` only after a confirmed image rebuild or
manual ECR tag repair when the Kubernetes Deployment template did not otherwise
change. The live deployment script resolves private ECR tags to
`tag@sha256:digest` before writing workload manifests, so corrected immutable
tags cause a real rollout while workloads can keep `imagePullPolicy:
IfNotPresent`.

## Kubernetes API Endpoint

In `NODE_COUNT=3`, the public API Network Load Balancer remains the admin
kubeconfig endpoint. Nodes use a local HAProxy listener on `API_HOST:6444` for
kubeadm join discovery and kube-proxy, with each node resolving `API_HOST` to
`127.0.0.1`. This avoids pinning node agents to a single bootstrap node when EC2
cannot hairpin through the public API NLB.

## Command Order

Copy and fill the local environment file:

```sh
cp infra/aws/self-managed-k8s/.env.aws.example infra/aws/self-managed-k8s/.env.aws.local
```

Run the guarded flow:

```sh
infra/aws/self-managed-k8s/scripts/00-preflight.sh
infra/aws/self-managed-k8s/scripts/10-cost-plan.sh
APPLY=true infra/aws/self-managed-k8s/scripts/20-deploy-guardrails.sh
APPLY=true infra/aws/self-managed-k8s/scripts/30-deploy-infra.sh
APPLY=true infra/aws/self-managed-k8s/scripts/40-bootstrap-k8s.sh
APPLY=true infra/aws/self-managed-k8s/scripts/50-build-and-deploy-cets.sh
infra/aws/self-managed-k8s/scripts/60-verify.sh
```

Pause, resume, failure drills, and cleanup are explicit:

```sh
APPLY=true PAUSE_DESTROYS_NODE_LOCAL_DATA_ACK=true infra/aws/self-managed-k8s/scripts/90-pause-cluster.sh
APPLY=true infra/aws/self-managed-k8s/scripts/91-resume-cluster.sh
APPLY=true infra/aws/self-managed-k8s/scripts/70-failure-drill.sh
APPLY=true infra/aws/self-managed-k8s/scripts/99-destroy-all.sh
```

## Public Ingress

`APP_INGRESS_MODE=cloudflare` is the default. It deploys `cloudflared` replicas
that make outbound connections to Cloudflare Tunnel, avoiding AWS application
load balancer cost. `APP_INGRESS_MODE=nlb` creates an optional app Network Load
Balancer that targets ingress-nginx NodePorts.

For a temporary trusted HTTPS URL without a custom domain, start a Cloudflare
Quick Tunnel. This is a preview-only URL under `trycloudflare.com`; it can change
after the tunnel pod restarts and should not be used as the production URL.
Anyone with the URL can reach the demo app, so stop it when the review is done.

```sh
APPLY=true QUICK_HTTPS_PUBLIC_EXPOSURE_ACK=true infra/aws/self-managed-k8s/scripts/92-start-quick-https-tunnel.sh
APPLY=true infra/aws/self-managed-k8s/scripts/93-stop-quick-https-tunnel.sh
```

The Kubernetes API HA endpoint uses an AWS Network Load Balancer only in
`NODE_COUNT=3` mode. This is for kubeadm control-plane availability, not for
application traffic.

## Argo CD Release Flow

The default CD path is GitOps after the cluster and platform are bootstrapped.
Argo CD watches `RELEASE_BRANCH` (`release/aws-self-managed-k8s` by default) and
detects drift in `infra/aws/self-managed-k8s/gitops/app`. A release is
controlled by a manual branch push plus a semver-like tag such as `v0.0.1`; no
workflow tracks `latest`.

Prepare a real release commit on the release branch before applying the Argo CD
`Application`. The committed `gitops/app` defaults are examples; update
non-secret runtime config such as hostname, object bucket, and replica counts in
Git before enabling sync.

```sh
git switch release/aws-self-managed-k8s
CETS_RELEASE_VERSION=v0.0.1 \
CETS_API_IMAGE=111111111111.dkr.ecr.us-east-2.amazonaws.com/cets-aws-k8s/cets-api:v0.0.1 \
CETS_FRONTEND_IMAGE=111111111111.dkr.ecr.us-east-2.amazonaws.com/cets-aws-k8s/cets-frontend:v0.0.1 \
infra/aws/self-managed-k8s/scripts/80-prepare-release-gitops.sh
git add infra/aws/self-managed-k8s/gitops/app/kustomization.yaml
git commit -m "release(aws-k8s): v0.0.1"
git tag -a v0.0.1 -m v0.0.1
infra/aws/self-managed-k8s/scripts/82-verify-release-gitops.sh
git push origin release/aws-self-managed-k8s v0.0.1
```

Apply the Argo CD `Application` only after the release branch contains real ECR
image references and the matching tag points at the release branch HEAD:

```sh
kubectl -n argocd apply -f infra/aws/self-managed-k8s/gitops/argocd/application.yaml
```

`82-verify-release-gitops.sh` checks that both app images use the same release
tag, that the release branch HEAD has the matching Git tag for branch pushes,
and that GitOps manifests do not contain runtime secrets. The default
`Application` intentionally omits automated sync; enable automated sync only
after branch protection and the release workflow are in place. The GitOps path
expects cluster-local secrets such as `cets-runtime-env`, `cets-app-secrets`,
and `cets-pg-app` to be created by the bootstrap scripts or another secret
manager before Argo CD syncs the app.
