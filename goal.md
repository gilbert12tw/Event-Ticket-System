# Phase 3 AWS Scheduled HA Demo Goal

## Objective

Deploy the existing Phase 3 HA/LGTM branch to AWS for two scheduled demos while keeping cost under
180 USD for the two-week window. Normal non-demo operation stays in a low-cost single-AZ development
mode. Demo windows upgrade to an ALB-fronted multi-AZ application shape with complete LGTM
observability, then scale back down after each demo.

Base branch: `origin/feature/phase3-compose-ha-lgtm-pr`.

## Codex Goal Operating Rules

- Start by switching to a local branch that tracks `origin/feature/phase3-compose-ha-lgtm-pr`.
- Treat completion as unproven until every requirement below has current evidence.
- Do not mark the goal complete from intent, partial implementation, or a narrow smoke test.
- If a requirement cannot be verified, keep iterating or record the blocker with exact evidence.
- Do not use AWS root credentials except for one-time IAM and billing bootstrap. All deployment
  actions must use a least-privilege IAM principal.
- Never commit `.env.local`, `terraform.tfvars`, Terraform state, AWS credentials, real CIDRs,
  real domain secrets, or provider tokens.

## Schedule

All times are `Asia/Taipei`.

| Window | Time | Required mode |
| --- | --- | --- |
| Demo 1 deploy/test | 2026-06-03 | `demo-ha` dry run, deploy, verify, failure drill |
| Demo 1 | 2026-06-04 19:00-21:00 | `demo-ha` |
| Post Demo 1 | after 2026-06-04 21:00 | scale down to `dev-single-az` |
| Demo 2 deploy/test | 2026-06-10 | `demo-ha` dry run, deploy, verify, failure drill |
| Demo 2 | 2026-06-11 19:00-21:00 | `demo-ha` |
| Post Demo 2 | after 2026-06-11 21:00 | scale down to `dev-single-az` or destroy |

## AWS Cost and Region Gates

- Candidate regions are `us-east-1`, `us-east-2`, and `us-west-2`.
- Before apply, run a live cost check or AWS Pricing Calculator equivalent for EC2, ALB, RDS,
  S3, EBS, CloudWatch/logging, data transfer, and DNS/ACM assumptions.
- Cost-gate evidence must use the two-week forecast window from 2026-06-01 00:00 through
  2026-06-15 00:00 Asia/Taipei unless a narrower replacement window is explicitly documented.
- Pick the lowest-cost candidate region that supports the required EC2, ALB, RDS, S3, IAM, ACM,
  and Budgets resources.
- Install AWS Budgets notifications at 1, 25, 90, 150, and 180 USD before any demo infrastructure
  is applied. The AWS Budget itself must use the same two-week demo guardrail window, represented
  in AWS Budgets UTC `YYYY-MM-DD_HH:MM` format.
- Stop before apply if the two-week forecast can exceed 180 USD, the 150 USD risk threshold is
  reached, or budget alarms cannot be created.
- Stop before any action that upgrades AWS Free Plan, joins Organizations or Control Tower, enables
  unclear paid features, or relies on unmanaged paid usage.

Reference docs:

- EC2 pricing: https://aws.amazon.com/ec2/pricing/on-demand/
- RDS PostgreSQL pricing: https://aws.amazon.com/rds/postgresql/pricing/
- AWS Budgets CLI: https://docs.aws.amazon.com/code-library/latest/ug/cli_2_budgets_code_examples.html
- EC2 Free Tier usage: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-free-tier-usage.html

Repo spec: `docs/specs/phase3-aws-scheduled-ha-demo.md`.

## Deployment Modes

### `dev-single-az`

- One app EC2 node.
- One observability/control EC2 node.
- Single-AZ RDS PostgreSQL.
- S3 report export bucket.
- Redis and Mailhog may run on the observability/control node as demo dependencies.
- HTTPS may remain available through the ALB only if cost gates pass; otherwise use an SSH tunnel
  or a temporary endpoint for development checks.

### `demo-ha`

- ALB HTTPS endpoint is the only user-facing endpoint.
- At least two app EC2 nodes are healthy and spread across different AZs.
- Optional third app EC2 node is allowed only if the pre-apply cost gate remains under 180 USD.
- RDS Multi-AZ may be enabled only during demo/test windows if cost gates pass.
- ALB target health must stop routing to unhealthy app nodes.
- Complete LGTM stack is available: Grafana, Loki, Tempo, Prometheus or compatible metrics path,
  Pyroscope, and Alloy ingestion.

### `post-demo`

- Scale down app nodes and RDS to the lowest-cost `dev-single-az` shape.
- Confirm the ALB, EC2, RDS, and observability resources match the intended post-demo mode.
- Record cost and budget status after scale-down.

## Runtime Inputs

Use untracked local files only:

- `infra/aws/free-tier-compose/terraform.tfvars`
- `infra/aws/free-tier-compose/.env.local`

Required values:

- AWS profile name for least-privilege deployment.
- Selected region after the live cost gate.
- Existing domain or subdomain.
- Allowlist CIDRs for SSH, Grafana, LGTM, and any non-public admin endpoint.
- Budget notification email.
- Cloudflare token only if DNS automation is explicitly used.
- Application secrets, database password, token signing secret, provider token secret.

## Implementation Scope

- Keep ticketing domain logic unchanged unless deployment verification exposes a real runtime
  incompatibility.
- Use Terraform/OpenTofu under `infra/aws/free-tier-compose`.
- Reuse existing Phase 3 Compose/LGTM assets where possible.
- RDS PostgreSQL and S3 are shared backing services.
- Redis and Mailhog are explicitly non-HA demo dependencies. Their outage may degrade reservation
  pre-admission or notification demo behavior, but booking, ticket, check-in, and audit truth must
  remain in PostgreSQL.
- Do not claim production multi-region HA, full disaster recovery, or complete production
  operations maturity.

## Required Verification Evidence

Before marking this goal complete, collect evidence for:

- Branch and diff are based on `origin/feature/phase3-compose-ha-lgtm-pr`.
- Terraform/OpenTofu `fmt`, `validate`, and `plan` pass for the selected mode.
- `scripts/aws/phase3-cost-gate.sh` output shows selected US region and two-week forecast below
  180 USD, including the forecast window start/end.
- AWS Budgets exist with thresholds 1, 25, 90, 150, and 180 USD.
- Dev mode serves `/healthz` and `/readyz` and connects to RDS.
- S3 report export path works or the remaining gap is documented.
- Demo HA ALB HTTPS endpoint serves the app and APIs.
- At least two app targets are healthy across AZs during demo mode.
- Stopping one app node makes ALB avoid that target and smoke tests continue.
- Grafana datasources are healthy for metrics, logs, traces, and profiles.
- A backend request is visible in Prometheus, Loki, Tempo, and Pyroscope.
- Redaction canaries show no PII, signed QR tokens, provider secrets, raw idempotency keys,
  email bodies, or raw recipient email in telemetry.
- Post-demo scale-down returns the environment to `dev-single-az`, or resources are destroyed.

Helper scripts:

- `scripts/aws/phase3-preflight.sh`: static Terraform and plan gate.
- `scripts/aws/phase3-bootstrap-iam.sh`: root-only bootstrap path for the least-privilege deployer
  identity used by all later AWS actions.
- `scripts/aws/phase3-identity-guard.sh`: refuses root credentials for deployment unless explicit
  bootstrap mode is set.
- `scripts/aws/phase3-init-local-config.sh`: creates untracked `terraform.tfvars` from required
  env vars and generated secrets.
- `scripts/aws/phase3-cost-gate.sh`: records pricing/calculator evidence and enforces 150/180 USD thresholds.
  It refuses incomplete candidate-region or required line-item coverage before selecting the
  lowest-cost candidate, then writes the selected region to ignored `selected-region.env`; do not
  default deployment to a region before this gate.
- `scripts/aws/phase3-live-price-csv.py`: queries the AWS Pricing API for the candidate regions
  and writes the CSV consumed by `phase3-cost-gate.sh`.
- `scripts/aws/phase3-mode.sh`: plans `dev-single-az`, `demo-ha`, or `post-demo`.
- `scripts/aws/phase3-apply.sh`: guarded apply wrapper for `dev-single-az`, `demo-ha`, or
  `post-demo`; it requires `APPLY=true`, verifies least-privilege identity, selected-region cost
  gate alignment, Terraform format/validate, applies and verifies AWS Budgets before the full
  infrastructure plan, then runs mode plan, post-apply verification, and evidence collection unless
  explicitly skipped. In `demo-ha`, it requires deep LGTM datasource/evidence checks by default.
- `scripts/aws/phase3-verify-aws.sh`: checks budget, ALB target health, HTTPS smoke, Grafana health,
  S3 export bucket read/write/delete path, optional datasource health, and optional deep LGTM
  evidence via Grafana datasource proxy plus an SSM redaction canary on an app node.
- `scripts/aws/phase3-s3-smoke.sh`: writes, heads, verifies AES256 server-side encryption for, and
  deletes a non-sensitive canary object in the Terraform-managed report export bucket. It can write
  JSON evidence files when `PHASE3_S3_SMOKE_EVIDENCE_DIR` is set.
- `scripts/aws/phase3-app-failure-drill.sh`: stops one app node and proves ALB failover before restoring it.
- `scripts/aws/phase3-collect-evidence.sh`: collects deployment evidence into ignored
  `infra/aws/free-tier-compose/cost-reports/evidence/` after apply, including raw Grafana proxy
  query responses when Grafana credentials are provided.
- `scripts/aws/phase3-completion-audit.py`: local read-only completion audit for `goal.md`
  evidence. It intentionally fails until the required cost-gate, Budget, dev, demo-ha, LGTM,
  redaction, and post-demo evidence exists.
- `scripts/aws/phase3-destroy.sh`: guarded cleanup wrapper that refuses root deployment credentials,
  requires `PHASE3_CONFIRM_DESTROY=destroy-cets-phase3` and `PHASE3_DESTROY_REASON`, verifies the
  selected region alignment, then applies a Terraform/OpenTofu destroy plan.
