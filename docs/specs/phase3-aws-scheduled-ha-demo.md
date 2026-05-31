# Phase 3 AWS Scheduled HA Demo

This spec extends the local Phase 3 HA/LGTM simulation into a scheduled AWS demo deployment. It is
cost-gated and time-boxed for two demo windows; it does not claim complete production HA,
multi-region failover, or disaster recovery.

## Goals

- Run normal development testing in `dev-single-az`: one app EC2 node, one observability/control
  EC2 node, single-AZ RDS PostgreSQL, and S3 report export storage.
- Upgrade to `demo-ha` for the scheduled deploy/test and demo windows: ALB HTTPS entrypoint, at
  least two app EC2 nodes across AZs, optional third app node only after cost approval, and complete
  LGTM observability.
- Keep one user-facing endpoint during demo windows. ALB target health must avoid unhealthy app
  nodes.
- Keep PostgreSQL as the final truth for booking, tickets, check-in, and audit.
- Keep Redis and Mailhog as demo dependencies with explicit degradation behavior; they are not
  HA data-truth services.

## Schedule

| Window | Time | Required mode |
| --- | --- | --- |
| Demo 1 deploy/test | 2026-06-03 Asia/Taipei | `demo-ha` dry run, deploy, verify, failure drill |
| Demo 1 | 2026-06-04 19:00-21:00 Asia/Taipei | `demo-ha` |
| Demo 2 deploy/test | 2026-06-10 Asia/Taipei | `demo-ha` dry run, deploy, verify, failure drill |
| Demo 2 | 2026-06-11 19:00-21:00 Asia/Taipei | `demo-ha` |

After each demo, the environment must scale back to `dev-single-az` unless the cost gate requires a
full destroy.

## Cost and Safety Gates

- Candidate regions are `us-east-1`, `us-east-2`, and `us-west-2`; choose the lowest-cost candidate
  that supports EC2, ALB, RDS PostgreSQL, S3, ACM, IAM, and Budgets.
- Two-week forecast must stay below 180 USD.
- Budgets must notify at 1, 25, 90, 150, and 180 USD before demo resources are applied.
- Stop before any account action that upgrades AWS Free Plan, joins Organizations or Control Tower,
  enables unclear paid features, or cannot be cost-estimated.
- Root credentials may only bootstrap IAM and billing guardrails. Deployment must use a
  least-privilege IAM principal.

## Acceptance Criteria

| AC | Requirement |
| --- | --- |
| AWS-PH3-AC-1 | Terraform/OpenTofu `fmt`, `validate`, and `plan` pass before apply. |
| AWS-PH3-AC-2 | `scripts/aws/phase3-cost-gate.sh` records selected region and two-week forecast below 180 USD. |
| AWS-PH3-AC-3 | AWS Budgets exist for 1, 25, 90, 150, and 180 USD notifications. |
| AWS-PH3-AC-4 | `dev-single-az` serves `/healthz` and `/readyz` and connects to RDS. |
| AWS-PH3-AC-5 | `demo-ha` has one ALB HTTPS endpoint and at least two healthy app targets across AZs. |
| AWS-PH3-AC-6 | Stopping one app node causes ALB to avoid the unhealthy target while smoke tests continue. |
| AWS-PH3-AC-7 | Grafana datasources for metrics, logs, traces, and profiles are healthy. |
| AWS-PH3-AC-8 | Backend request evidence exists in Prometheus, Loki, Tempo, and Pyroscope. |
| AWS-PH3-AC-9 | Telemetry redaction canaries cover PII, signed QR tokens, provider secrets, raw idempotency keys, email bodies, and raw recipient email. |
| AWS-PH3-AC-10 | Post-demo mode is scaled back to `dev-single-az` or destroyed with evidence. |

## Non-Goals

- No production multi-region failover.
- No claim of complete disaster recovery.
- No managed Redis HA requirement for this demo.
- No real SSO, HR, or email provider integration unless a separate security and cost spec is added.
