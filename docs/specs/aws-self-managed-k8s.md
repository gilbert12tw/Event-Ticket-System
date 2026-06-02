# AWS Self-Managed Kubernetes Deployment

This spec defines a separate AWS deployment track under `infra/aws/self-managed-k8s/`.
It is not EKS and it does not change the Phase 1 Docker Compose boundary, the Phase 2
process-first boundary, or the Phase 3 local Compose simulation contract.

## Goals

- Provision kubeadm Kubernetes on EC2 using CloudFormation.
- Keep AWS resources separate from non-AWS Kubernetes deployment assets.
- Support `NODE_COUNT=1` for a low-cost demo and `NODE_COUNT=3` for HA.
- Reject `NODE_COUNT=2` because stacked etcd quorum is not useful with two members.
- Prefer stable 3-node On-Demand capacity at 16 GiB RAM per node for the
  two-week budget target; keep Spot as an explicit cost option for accepted
  interruption risk or larger short-lived nodes.
- Keep public app ingress cheap by default with Cloudflare Tunnel.
- Add cost guardrails for a 190 USD budget and 14-day runtime.

## Infrastructure Contract

- CloudFormation templates live in `infra/aws/self-managed-k8s/templates/`.
- `guardrails.yaml` owns Budget, SNS, Lambda cleanup, and hourly TTL cleanup.
- `cluster.yaml` owns VPC, public subnets, security groups, EC2 Launch Template,
  control-plane Auto Scaling group, optional worker Auto Scaling group, ECR
  repositories, and conditional Network Load Balancers.
- EKS, RDS, ElastiCache, NAT Gateway, and AWS-managed Kubernetes services are not used.
- The Kubernetes API Network Load Balancer is created only for `NODE_COUNT=3`.
- The application Network Load Balancer is created only when `APP_INGRESS_MODE=nlb`.
- Default `APP_INGRESS_MODE=cloudflare` deploys `cloudflared` replicas and expects
  the named Cloudflare Tunnel to route to ingress-nginx.

## Kubernetes Contract

- `40-bootstrap-k8s.sh` installs containerd, kubeadm, kubelet, kubectl, Calico,
  metrics-server, and ingress-nginx using AWS EC2 nodes.
- In 3-node mode, each control-plane node runs a node-local HAProxy endpoint on
  `API_HOST:6444` for kubeadm join discovery and kube-proxy. This avoids binding
  node agents to one bootstrap node when AWS public NLB hairpin traffic is not
  usable from EC2.
- `50-build-and-deploy-cets.sh` deploys CloudNativePG, Redis, MinIO, Mailhog,
  backend/frontend workloads, backend HPA, and kind-scoped workers.
- ECR image builds default to `CETS_IMAGE_PLATFORM=linux/amd64` because the
  budget On-Demand EC2 defaults are x86_64; Apple Silicon builders must not push
  arm64-only images to this cluster shape.
- `FORCE_PRIVATE_IMAGE_ROLLOUT=true` is an explicit repair switch for a confirmed
  image rebuild or ECR tag correction when the Deployment template is otherwise
  unchanged.
- ECR image tags are resolved to `tag@sha256:digest` in live manifests so
  corrected immutable release tags trigger a real Kubernetes rollout without
  relying on `imagePullPolicy: Always`.
- `gitops/app` is the declarative Argo CD app rollout path. It manages backend,
  frontend, worker deployments, ingress, and a PreSync migration Job from Git.
  It does not commit runtime secrets.
- `gitops/argocd/application.yaml` points Argo CD at
  `release/aws-self-managed-k8s` by default. Automated sync is not enabled in
  the committed default because first install must wait for a real release
  branch commit and matching tag.
- In 3-node mode, CloudNativePG runs three PostgreSQL instances with required
  synchronous replication to one standby.
- Backend/frontend replica count defaults to the node count; HPA can scale backend
  pods up to `BACKEND_MAX_REPLICAS`.
- The worker Auto Scaling group starts at zero and is tagged for Cluster Autoscaler;
  control-plane nodes are not delegated to Cluster Autoscaler.

## Cost Guardrails

- `10-cost-plan.sh` estimates EC2, root EBS, public IPv4, and required API/app NLB
  cost before any cluster resources are deployed.
- The deploy limit is `BUDGET_LIMIT_USD - COST_RESERVE_USD`; defaults are 190 and
  15 USD.
- `20-deploy-guardrails.sh` installs the Budget/SNS/Lambda/TTL cleanup stack.
- Guardrails deployment requires `DEDICATED_AWS_ACCOUNT_ACK=true` because the
  AWS Budget notification is account-wide; use this track in a dedicated or
  isolated AWS account.
- The cleanup Lambda scales Auto Scaling groups to zero, empties ECR repositories,
  and deletes stacks only when the prefix, `CetsCleanupGroup`, and
  `Track=aws-self-managed-k8s` scope match.
- Budget spend is calculated before applying Free Tier credits (`IncludeCredit:
  false`) so cleanup protects credit consumption, not only post-credit invoices.
- `99-destroy-all.sh` is the explicit local cleanup path and must be safe to run
  after partial deployment.

The pre-deployment cost model separates stack TTL from VM running time:
`(compute + root EBS + public IPv4) x ACTIVE_RUNTIME_HOURS + NLB x MAX_RUNTIME_HOURS`.
`MAX_RUNTIME_HOURS` remains the two-week TTL window; `ACTIVE_RUNTIME_HOURS`
must be less than or equal to it.

The paid default is 3 x 16 GiB On-Demand nodes using distinct ASG overrides such
as `r6a.large`, `r5a.large`, and `r6i.large`. With the app NLB enabled, this
shape fits the default two-week `190 USD - 15 USD reserve` preflight estimate in
the supported regions. 3 x 32 GiB On-Demand nodes, such as `r6a.xlarge`,
`r5a.xlarge`, and `r6i.xlarge`, do not fit a continuous 14-day budget window and
must be treated as short validation capacity or paired with lower
`ACTIVE_RUNTIME_HOURS` through pause/resume. Spot remains available only when
interruption risk is acceptable.

Pause and resume for cost control are destructive: scaling ASGs to zero destroys
EC2 instances and root EBS, including Kubernetes node-local `local-path` PV data.
Data continuity requires external backup or non-local persistent storage before any
pause/resume cycle.

Each node has a 120GB encrypted gp3 root volume by default. Kubelet container
log rotation is configured by `K8S_CONTAINER_LOG_MAX_SIZE` and
`K8S_CONTAINER_LOG_MAX_FILES`; app logs must use stdout/stderr and should be
shipped to centralized logging for durable retention.

## Release Branch CD

- Application images are versioned by semver-like tags such as `v0.0.1`; the
  GitOps path does not use `latest`.
- `80-prepare-release-gitops.sh` updates the GitOps image references for a
  release branch commit and validates that both app images use the same tag.
- Operators manually push `release/aws-self-managed-k8s` and the matching tag.
  The verifier requires the matching tag to point at release branch HEAD before
  branch rollout. Argo CD detects the branch change; automatic sync should be
  enabled only after branch protection/release checks are in place.
- Non-secret deploy config in GitOps manifests is the app source of truth. Edit
  hostname, object bucket, replica count, and HPA limits in Git before enabling
  Argo CD sync for a customized environment.
- GitHub release workflow checks are non-destructive; they validate release
  manifests and do not call AWS or Argo CD deployment APIs.

## Acceptance Criteria

| ID | Scenario |
| --- | --- |
| AWS-K8S-AC-1 | Static tests prove AWS scripts do not source or call non-AWS deployment scripts. |
| AWS-K8S-AC-2 | CloudFormation templates do not include EKS, RDS, ElastiCache, or NAT Gateway resources. |
| AWS-K8S-AC-3 | Default config is Cloudflare app ingress; app NLB exists only behind an explicit condition. |
| AWS-K8S-AC-4 | Guardrails include Budget, SNS, Lambda cleanup, EventBridge TTL, and stack cleanup tags. |
| AWS-K8S-AC-5 | ASGs use Launch Templates, MixedInstancesPolicy with On-Demand defaults and optional Spot support, and explicit 1/3 node sizing. |
| AWS-K8S-AC-6 | `bash -n` passes for all AWS scripts. |
| AWS-K8S-AC-7 | Live 1-node verification passes `/healthz` and `/readyz`. |
| AWS-K8S-AC-8 | Live 3-node verification shows three Ready control-plane nodes and healthy etcd. |
| AWS-K8S-AC-9 | Failure drill deletes a backend pod, repairs one EC2 replacement through idempotent bootstrap, and verifies PostgreSQL failover. |
| AWS-K8S-AC-10 | Argo CD release manifests track `release/aws-self-managed-k8s`, use explicit `v*` image tags, require matching tags at branch HEAD, and contain no runtime secrets. |
| AWS-K8S-AC-11 | Pause scales only in-scope AWS self-managed ASGs to zero, requires destructive-data acknowledgement, and never touches non-AWS Kubernetes assets. |
| AWS-K8S-AC-12 | Resume reruns cost preflight, checks guardrails, reconciles CloudFormation parameters before ASG restore, and bootstraps kubeadm before optional app deploy. |
| AWS-K8S-AC-13 | Budget cleanup tracks gross usage before credits and kubelet log rotation bounds node-local container logs. |

## Non-Goals

- No cross-region or multi-region HA.
- No managed Kubernetes service.
- No AWS-managed database/cache promotion.
- No service mesh.
- No claim that Kubernetes is a Phase 1 or Phase 2 deliverable.
