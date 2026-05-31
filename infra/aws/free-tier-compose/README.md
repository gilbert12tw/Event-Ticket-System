# Phase 3 AWS Free-Tier Compose Scaffold

This directory contains Terraform/OpenTofu scaffold for the scheduled Phase 3 AWS demo. It is
designed to be reviewed and planned before any AWS resource is created.

## Modes

- `dev-single-az`: one app node, one observability node, single-AZ RDS.
- `demo-ha`: ALB HTTPS, two app nodes across AZs, optional third app node, optional RDS Multi-AZ.
- `post-demo`: use `dev-single-az` again after a demo window.

## Required Local Files

Copy `terraform.tfvars.example` to `terraform.tfvars` and fill in real values. The real tfvars file
is intentionally ignored by git.

```sh
cd infra/aws/free-tier-compose
cp terraform.tfvars.example terraform.tfvars
```

Or generate an untracked local file from environment variables after the live cost gate has written
`selected-region.env`:

```sh
AWS_PROFILE_NAME=cets-phase3-deployer \
PHASE3_AWS_REGION=us-east-1 \
PHASE3_DOMAIN_NAME=tickets.example.com \
PHASE3_BUDGET_EMAIL=you@example.com \
PHASE3_ALLOWLIST_CIDRS=203.0.113.10/32 \
PHASE3_REPOSITORY_URL=https://github.com/OWNER/Event-Ticket-System.git \
scripts/aws/phase3-init-local-config.sh
```

The generated config defaults the AWS Budgets window to `2026-05-31_16:00` through
`2026-06-14_16:00` UTC, which maps to 2026-06-01 00:00 through 2026-06-15 00:00 Asia/Taipei.
Override `PHASE3_BUDGET_START_UTC` and `PHASE3_BUDGET_END_UTC` only if the deployment window
changes, and keep the replacement window aligned with the cost-gate report.

Do not commit Terraform state, real secrets, allowlist CIDRs, or Cloudflare tokens.

## IAM Bootstrap

Deployment must not use root credentials. Use root only for a one-time IAM bootstrap, then switch
to the generated deployer profile:

```sh
scripts/aws/phase3-bootstrap-iam.sh
APPLY=true scripts/aws/phase3-bootstrap-iam.sh
```

The apply form writes an ignored `infra/aws/free-tier-compose/.env.local` with the new access key.
Move those values into the named AWS CLI profile before running preflight.

## Review Gates

Run these before any apply:

```sh
terraform fmt -check
terraform init -backend=false
terraform validate
terraform plan -out phase3.tfplan
```

If using the helper scripts, `TF_BIN` defaults to `terraform` when present and otherwise falls back
to `tofu`.

Before applying demo mode, run a live cost estimate for `us-east-1`, `us-east-2`, and `us-west-2`.
Apply must stop if the two-week forecast can exceed 180 USD or if AWS Budgets cannot be created.
`terraform.tfvars` must use the region selected by the cost gate.
The default cost-gate forecast covers 336 hours, from 2026-06-01 00:00 through 2026-06-15 00:00
Asia/Taipei, with 96 hours reserved as conservative `demo-ha`/deploy-test capacity.
The cost gate refuses incomplete CSV evidence: every candidate region must include the required
EC2, RDS instance, RDS storage guardrail, ALB, EBS, S3, CloudWatch/logging, data-transfer, and
DNS/ACM line items before a lowest-cost region can be selected.

Record cost evidence:

```sh
AWS_PROFILE=cets-phase3-deployer \
  scripts/aws/phase3-live-price-csv.py \
  --out infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv
scripts/aws/phase3-cost-gate.sh infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv
source infra/aws/free-tier-compose/selected-region.env
```

Generate the untracked runtime config only after that cost gate has passed. The generator refuses
stale or incomplete cost gates before writing `terraform.tfvars`: the selected region must match
`selected-region.env`, the cost report must be `pass`, the forecast window must be the exact
2026-06-01 00:00 through 2026-06-15 00:00 Asia/Taipei guardrail, and the domain, budget email,
repository URL, and CIDR inputs must parse cleanly.

Deployment scripts refuse to run with root credentials by default. Use root only for one-time IAM
or billing bootstrap outside this stack, and only with `PHASE3_ALLOW_ROOT_BOOTSTRAP=true`.

Mode planning helpers:

```sh
scripts/aws/phase3-mode.sh dev-single-az
scripts/aws/phase3-mode.sh demo-ha
scripts/aws/phase3-mode.sh post-demo
```

Guarded apply helpers:

```sh
APPLY=true scripts/aws/phase3-apply.sh dev-single-az
APPLY=true scripts/aws/phase3-apply.sh demo-ha
APPLY=true scripts/aws/phase3-apply.sh post-demo
```

The apply wrapper refuses implicit AWS changes. It checks the least-privilege identity, confirms
`terraform.tfvars` uses the cost-gate selected region, validates Terraform, and for `demo-ha`
checks the scheduled Asia/Taipei deploy/test or demo window before any AWS write. It then
target-applies `aws_budgets_budget.phase3`, verifies the 1, 25, 90, 150, and 180 USD AWS Budgets
notifications, verifies the budget limit and exact 2026-05-31_16:00 through 2026-06-14_16:00 UTC
guardrail window in AWS Budgets, creates the mode plan, applies it, then runs
`phase3-verify-aws.sh` and `phase3-collect-evidence.sh` by default. This keeps the budget guardrail
in place before EC2, ALB, RDS, and S3 resources are applied, while still refusing accidental
`demo-ha` writes outside the approved schedule.

For `demo-ha`, the wrapper sets `PHASE3_REQUIRE_LGTM_DATASOURCES=true` and
`PHASE3_REQUIRE_LGTM_EVIDENCE=true` by default, so `GRAFANA_ADMIN_USER` and
`GRAFANA_ADMIN_PASSWORD` must be present. Keep `PHASE3_SKIP_VERIFY=true`,
`PHASE3_COLLECT_EVIDENCE=false`, and `PHASE3_DEMO_HA_LIGHT_VERIFY=true` for emergency/manual
recovery or explicit rehearsals only, and record the reason in the ignored evidence directory.

`demo-ha` planning is schedule-gated in `Asia/Taipei`. It is accepted only during the deploy/test
days and demo windows from `goal.md`:

- 2026-06-03 for Demo 1 deploy/test
- 2026-06-04 19:00-21:00 for Demo 1
- 2026-06-10 for Demo 2 deploy/test
- 2026-06-11 19:00-21:00 for Demo 2

For an explicit rehearsal outside those windows, set
`PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW=true`. Keep that override out of committed files and record
why it was used in the deployment evidence.

AWS verification helpers after apply:

```sh
scripts/aws/phase3-verify-aws.sh
scripts/aws/phase3-collect-evidence.sh
PHASE3_DRILL_EVIDENCE_DIR=infra/aws/free-tier-compose/cost-reports/evidence/<demo-run-id> \
  TARGET_INDEX=0 \
  scripts/aws/phase3-app-failure-drill.sh
```

`scripts/aws/phase3-apply.sh demo-ha` runs the app-node failure drill automatically by default.
When evidence collection is enabled, the apply wrapper assigns one `PHASE3_EVIDENCE_RUN_ID` and
uses it for both the drill and `phase3-collect-evidence.sh`, so completion-audit proof lands in a
single ignored demo-ha evidence directory. Set `PHASE3_RUN_FAILURE_DRILL=false` only for an
explicitly documented rehearsal where stopping an app node is unsafe.

`phase3-verify-aws.sh` defaults to the full mode invariant: `demo-ha` requires two healthy ALB
targets across at least two availability zones, while `dev-single-az`/`post-demo` require at least
one healthy target. During the failure drill, `phase3-app-failure-drill.sh` temporarily lowers the
minimum to one healthy target and requires the stopped target to be non-healthy, then still runs
HTTPS `/healthz` and `/readyz` smoke checks through the ALB before restoring the instance. If the
drill exits after stopping an instance, an exit trap attempts to start it again before the script
ends. Set `PHASE3_DRILL_EVIDENCE_DIR` to the same ignored demo evidence directory created by
`phase3-collect-evidence.sh` so the completion audit can prove the stopped-target and restored
target-health records belong to the demo-ha run. The drill suppresses deep LGTM canary checks
during the stop/restore loop; run or keep the apply wrapper's normal demo-ha verifier with
`PHASE3_REQUIRE_LGTM_EVIDENCE=true` for the separate LGTM/redaction proof.

The verifier also runs `phase3-s3-smoke.sh` against the Terraform-managed report export bucket.
That helper writes a non-sensitive object under `exports/phase3-smoke/`, confirms it with
`head-object`, verifies `AES256` server-side encryption, and deletes it by default. Set
`PHASE3_S3_SMOKE_EVIDENCE_DIR` only when you want the JSON `put-object`, `head-object`, and
`delete-object` responses preserved in an ignored evidence directory.

For the deeper LGTM gate, pass the untracked Grafana credentials when running the verifier:

```sh
GRAFANA_ADMIN_USER=admin \
GRAFANA_ADMIN_PASSWORD='from-terraform-tfvars' \
PHASE3_REQUIRE_LGTM_DATASOURCES=true \
PHASE3_REQUIRE_LGTM_EVIDENCE=true \
scripts/aws/phase3-verify-aws.sh
```

That checks provisioned Grafana datasource health for Prometheus, Loki, Tempo, and Pyroscope,
then uses Grafana datasource proxy queries to prove backend evidence exists in Prometheus, Loki,
Tempo, and Pyroscope. It also sends a short SSM redaction-canary command to a healthy app node and
fails if Loki receives raw PII, signed QR/token values, provider secrets, email bodies, raw
recipient email, or raw idempotency keys. The deployer IAM policy must include the SSM permissions
from `scripts/aws/phase3-bootstrap-iam.sh`; rerun that bootstrap with `ROTATE_ACCESS_KEY=false`
after pulling policy changes.

`phase3-collect-evidence.sh` writes an ignored run directory under
`infra/aws/free-tier-compose/cost-reports/evidence/`. It collects safe Terraform outputs, AWS
identity, budget window/limit and notification proof, selected cost-gate report, ALB target health, app instance
AZ/state evidence, `/healthz`, `/readyz`, S3 export bucket smoke proof, Grafana health, optional
Grafana datasource health, and the full `phase3-verify-aws.sh` output. When Grafana credentials are
present, it also stores raw Grafana datasource proxy responses for Prometheus, Tempo, Pyroscope,
Loki backend logs, and the redaction canary. Do not move those evidence files into tracked docs if
they contain real account metadata, IP addresses, allowlist-related values, or budget subscriber
email.

Before claiming `goal.md` is complete, run the local read-only completion audit:

```sh
scripts/aws/phase3-completion-audit.py
```

It does not call AWS or print secrets. It checks the selected-region cost report, local tfvars
alignment, the latest ignored `demo-ha` evidence directory, Budget API evidence, ALB target health,
failure-drill evidence, health checks, S3 smoke proof, Grafana datasource proof, LGTM proxy
responses, redaction canary output, and the latest ignored `dev-single-az` post-demo scale-down
evidence. If the final post-demo path is full cleanup instead of scale-down, it accepts the ignored
destroy evidence written by `phase3-destroy.sh` when the destroy summary is marked destroyed and
the post-destroy Terraform state list is empty. During interim work before the demos are complete,
it should fail and list the missing evidence instead of letting a partial deployment look finished.

Post-demo cleanup can scale down or destroy. Prefer `post-demo` first when you still need the
single-AZ development environment:

```sh
APPLY=true scripts/aws/phase3-apply.sh post-demo
```

For full cleanup after the final demo or when cost gates require stopping all AWS spend:

```sh
PHASE3_CONFIRM_DESTROY=destroy-cets-phase3 \
PHASE3_DESTROY_REASON='post-demo cleanup' \
scripts/aws/phase3-destroy.sh
```

The destroy helper still refuses root deployment credentials and verifies the selected-region
alignment before applying a destroy plan. By default it writes ignored evidence under
`infra/aws/free-tier-compose/cost-reports/evidence/destroy-<timestamp>/`, including identity-guard
proof, the destroy plan/apply logs, a destroy summary, and the post-destroy Terraform state list.
Set `PHASE3_COLLECT_DESTROY_EVIDENCE=false` only for manual recovery where evidence has already
been captured elsewhere.

## DNS and HTTPS

This scaffold creates an ACM DNS-validated certificate and outputs validation records. Add the DNS
records in the existing DNS provider, then re-run plan/apply. Cloudflare automation is intentionally
not enabled by default because it requires an untracked token and a separate provider.

## HA Boundary

Demo HA means ALB target health can route around unhealthy app nodes during the scheduled demo
windows. This scaffold does not claim multi-region HA or complete disaster recovery.
