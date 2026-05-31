# Phase 3 AWS Deployment Progress

This report records non-secret evidence gathered while working toward `goal.md`.

## 2026-05-31 Asia/Taipei

Completed IAM bootstrap for the Phase 3 AWS deployer identity.

Evidence:

- Current branch: `feature/phase3-compose-ha-lgtm-pr`.
- Root identity was used only for IAM bootstrap.
- `scripts/aws/phase3-bootstrap-iam.sh` created or reused IAM user `cets-phase3-deployer`.
- Deployer identity verified with AWS STS:
  - Account: `992382443806`
  - ARN: `arn:aws:iam::992382443806:user/cets-phase3-deployer`
- `scripts/aws/phase3-identity-guard.sh` accepts the deployer profile as non-root.
- Generated access-key helper is stored in ignored local file:
  `infra/aws/free-tier-compose/.env.local`.
- The generated AWS CLI profile is `cets-phase3-deployer`.

Remaining required evidence before `goal.md` can be marked complete:

- Real untracked `infra/aws/free-tier-compose/terraform.tfvars` with domain, allowlist CIDRs,
  budget email, and secrets.
- Terraform/OpenTofu plan and apply using the deployer profile.
- AWS Budgets proof for 1, 25, 90, 150, and 180 USD thresholds.
- Dev mode `/healthz` and `/readyz` evidence with RDS connectivity.
- S3 report export evidence or a documented gap.
- Demo HA ALB HTTPS smoke evidence.
- At least two healthy app targets across AZs during demo mode.
- App-node failure drill evidence showing ALB avoids an unhealthy target.
- Grafana, Loki, Tempo, Prometheus, Pyroscope, and Alloy runtime evidence.
- Telemetry redaction canary evidence.
- Post-demo scale-down or destroy evidence.

## 2026-05-31 Pre-Push Gate Attempt

Attempted required pre-push gate with `act push` before publishing local commits needed by EC2
bootstrap.

Evidence:

- `act push` started the workflow and completed the `changes` and `openapi` jobs.
- The local act runner failed before backend/frontend tests could run because the default
  `catthehacker/ubuntu:act-latest` runner image did not have `node` available for JavaScript
  actions:
  - frontend: `pnpm/action-setup` failed with `exec: "node": executable file not found in $PATH`.
  - backend: `actions/setup-go` failed with `exec: "node": executable file not found in $PATH`.
- Because the required pre-push gate did not pass, the branch was not pushed.

Follow-up:

- Configure a working act runner image or another project-approved local CI path before pushing.
- The branch still needs to be published before EC2 bootstrap can clone the latest AWS deployment
  files.

## 2026-05-31 Region Gate Hardening

Updated the AWS config flow so deployment cannot silently default to `us-east-1`.

Evidence:

- Terraform variable `aws_region` now requires an explicit value.
- `scripts/aws/phase3-init-local-config.sh` fails if `PHASE3_AWS_REGION` is missing.
- `scripts/aws/phase3-cost-gate.sh` writes ignored `infra/aws/free-tier-compose/selected-region.env`
  with the selected lowest-cost candidate region after the cost gate passes.
- Ignored local artifacts confirmed:
  - `infra/aws/free-tier-compose/selected-region.env`
  - `infra/aws/free-tier-compose/cost-reports/`

Required behavior:

- Deployment must not choose a region from preference or convenience. The selected region must come
  from `scripts/aws/phase3-cost-gate.sh` after comparing `us-east-1`, `us-east-2`, and `us-west-2`.
- `terraform.tfvars` must use that selected region, or the apply must stop before touching AWS
  infrastructure.

Verification:

- `PHASE3_AWS_REGION=us-east-1 scripts/aws/phase3-init-local-config.sh` is no longer accepted as
  an implicit default path; the caller must provide the chosen region explicitly.
- `scripts/aws/phase3-cost-gate.sh` refuses the example cost CSV unless
  `ALLOW_EXAMPLE_COSTS=true`, so example data cannot be mistaken for live pricing evidence.

## 2026-05-31 Live AWS Pricing Gate

Generated the Phase 3 two-week forecast from the AWS Pricing API and passed the cost gate before
any Terraform apply.

Evidence:

- `scripts/aws/phase3-bootstrap-iam.sh` now includes `pricing:GetProducts` in the deployer policy
  and supports `ROTATE_ACCESS_KEY=false` for policy-only bootstrap updates.
- Root profile was used only for this IAM bootstrap policy update:
  `AWS_PROFILE=default ROTATE_ACCESS_KEY=false APPLY=true scripts/aws/phase3-bootstrap-iam.sh`.
- Existing deployer credentials were not rotated.
- `scripts/aws/phase3-live-price-csv.py` generated the ignored live pricing input:
  `infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv`.
- `scripts/aws/phase3-cost-gate.sh` selected the lowest-cost region and wrote the ignored
  selection file:
  `infra/aws/free-tier-compose/selected-region.env`.

Cost-gate result:

- Selected region: `us-east-1`.
- Selected two-week forecast: `38.4957 USD`.
- Candidate totals from the generated report:
  - `us-east-1`: `38.4957 USD`
  - `us-east-2`: `38.4957 USD`
  - `us-west-2`: `38.4957 USD`
- Status: `pass`; below both the `150 USD` risk stop and `180 USD` hard cap.
- Tie behavior: when regions have equal totals, `scripts/aws/phase3-cost-gate.sh` now picks the
  deterministic lexical lowest region instead of relying on awk map iteration order.

Pricing scope:

- EC2 app and observability nodes, RDS PostgreSQL single-AZ and multi-AZ windows, ALB hours, ALB
  LCU minimum, gp3 EBS, and S3 Standard prices come from the AWS Pricing API.
- CloudWatch/logging/alarms and low demo traffic data transfer remain explicit conservative
  guardrail estimates in the generated CSV.

Verification:

- `bash -n scripts/aws/phase3-bootstrap-iam.sh scripts/aws/phase3-cost-gate.sh` passed.
- `python3 -m py_compile scripts/aws/phase3-live-price-csv.py` passed.
- `AWS_PROFILE=cets-phase3-deployer scripts/aws/phase3-live-price-csv.py --out infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv`
  passed.
- `scripts/aws/phase3-cost-gate.sh infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv`
  passed.

## 2026-05-31 Act Push Gate Retry

Retried the required pre-push gate after adding a local act Node path override.

Evidence:

- `.actrc` adds `/opt/acttoolcache/node/24.16.0/x64/bin` to `PATH` for the local act runner.
- `act push` no longer fails at JavaScript actions:
  - `changes` completed.
  - `openapi` completed.
  - frontend lint, type-check, format check, unit tests, and build completed.
  - backend golangci-lint completed with `0 issues`.
  - backend `go test ./... -count=1` completed.
- The frontend job did not complete because the local act runner stalled during
  `pnpm --filter cets-web exec playwright install --with-deps chromium`.
  The Chromium download subprocess remained running for more than eight minutes after reporting
  progress, and `/root/.cache/ms-playwright` contained only a partial `chromium-1217` install.
- The act run was terminated and the leftover act frontend container was removed.
- Because the full required `act push` gate did not pass, the branch was not pushed.

Follow-up:

- Fix or bypass the local act Playwright browser-install stall with a project-approved approach,
  then rerun `act push`.
- Do not push the branch until `act push` passes or the project owner explicitly accepts this local
  runner limitation.

## 2026-05-31 CI Playwright Sharding Hardening

Refactored the CI workflow to reduce duplicated web setup and make mocked Playwright checks use
controlled parallelism.

Evidence:

- Added `.github/actions/setup-web/action.yml` so frontend and live-gate jobs share the same
  pnpm, Node.js, and dependency setup.
- Added `scripts/ci/install-playwright.sh` so local act runs can use the Playwright image's
  preinstalled `/ms-playwright` browsers instead of downloading Chromium during every run.
- Updated `.actrc` to use `mcr.microsoft.com/playwright:v1.59.1-noble`, matching the locked
  Playwright version.
- Split mocked Playwright checks out of the frontend job into `frontend-e2e`, with two matrix
  shards and isolated ports/output directories for local act compatibility.
- Kept controlled in-shard parallelism with `PLAYWRIGHT_WORKERS=2`, avoiding the previous local
  eight-browser overload while still running multiple shards concurrently.

Verification:

- `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/ci.yml"); YAML.load_file(".github/actions/setup-web/action.yml")'`
  passed.
- `bash -n scripts/ci/install-playwright.sh` passed.
- `git diff --check` passed.
- `pnpm --filter cets-web type-check` passed.
- `pnpm exec prettier --check apps/web/playwright.config.ts .github/workflows/ci.yml .github/actions/setup-web/action.yml`
  passed.
- `act push -j frontend-e2e` passed with both shards:
  - shard 1: 62 passed in about 4.3 minutes.
  - shard 2: 62 passed in about 4.3 minutes.
- `act push -j frontend` passed:
  - frontend lint passed.
  - frontend type-check passed.
  - formatting passed.
  - Vitest passed with 23 files and 118 tests.
  - frontend build passed.

## 2026-05-31 Full Act Push Gate Passed

Ran and passed the complete local push workflow before publishing the latest AWS cost-gate commit.

Evidence:

- Commit pushed: `bead2f3 feat(deploy): add live aws pricing gate`.
- Branch pushed: `feature/phase3-compose-ha-lgtm-pr`.
- `act push` completed successfully.
- Frontend static checks passed: lint, type-check, and Prettier formatting.
- Frontend unit/build passed:
  - Vitest: 23 files, 118 tests.
  - Production web build completed.
- Mocked Playwright shards passed:
  - shard 1: 62 passed.
  - shard 2: 62 passed.
- OpenAPI contract check passed.
- Backend checks passed:
  - `golangci-lint` found `0 issues`.
  - `go test ./...` passed, including deployment contract tests.
- Compose contract passed:
  - Compose config rendered successfully.
  - `go test ./deploy` passed.
- Live gates passed:
  - Release Docker image built.
  - Backing services started.
  - Explicit database migration completed.
  - `/readyz` returned ready.
  - Live Playwright workflow checks passed: 4 tests.
  - k6 smoke gate passed with `684/684` checks and `0` failed HTTP requests.
  - WS4 worker-isolation lag gate passed with `63/63` checks.
  - Sensitive log leakage scan passed.
  - Local live-gate containers, network, and volumes were stopped and removed.

## 2026-05-31 Pre-Apply Guard Hardening

Hardened the local AWS deployment scripts so `terraform.tfvars` cannot drift from the live cost-gate
region selection.

Evidence:

- `scripts/aws/phase3-init-local-config.sh` now requires
  `infra/aws/free-tier-compose/selected-region.env` before generating untracked `terraform.tfvars`.
- `scripts/aws/phase3-init-local-config.sh` refuses regions outside `us-east-1`, `us-east-2`, and
  `us-west-2`.
- `scripts/aws/phase3-init-local-config.sh` refuses to generate config if the requested
  `PHASE3_AWS_REGION` does not match the cost-gate selected `PHASE3_AWS_REGION`.
- `scripts/aws/phase3-preflight.sh` now checks that:
  - `selected-region.env` exists.
  - the cost-gate report exists.
  - the cost-gate report status is `pass`.
  - `terraform.tfvars` `aws_region` matches the selected region.
- `scripts/aws/phase3-preflight.sh` and `scripts/aws/phase3-mode.sh` now auto-detect
  `terraform` or `tofu` when `TF_BIN` is not set.
- `infra/aws/free-tier-compose/README.md` now documents the live Pricing API flow instead of the
  old manual example CSV replacement flow.

Verification:

- `bash -n scripts/aws/phase3-init-local-config.sh scripts/aws/phase3-preflight.sh` passed.
- `scripts/aws/phase3-init-local-config.sh` generated a temporary tfvars file when the requested
  region matched the selected region.
- `scripts/aws/phase3-init-local-config.sh` rejected a temporary config attempt with
  `PHASE3_AWS_REGION=us-west-2` because the cost gate selected `us-east-1`.
- `git diff --check` passed.
- `tofu -chdir=infra/aws/free-tier-compose fmt -check` passed.
- `tofu -chdir=infra/aws/free-tier-compose init -backend=false` passed.
- `tofu -chdir=infra/aws/free-tier-compose validate` passed.

## 2026-05-31 Demo HA Verification Hardening

Strengthened the AWS verifier so `demo-ha` evidence proves both target health and availability-zone
spread.

Evidence:

- `scripts/aws/phase3-verify-aws.sh` already required two healthy ALB app targets in `demo-ha`.
- It now resolves the healthy target instance IDs through EC2 and fails unless those healthy
  instances span at least two availability zones.
- The failure drill still temporarily lowers the healthy-target minimum to one, so it can prove ALB
  avoids the stopped target without incorrectly requiring two healthy targets during the outage.
- The verifier output now includes `healthy_target_azs` for the completion audit.

Required runtime proof still missing:

- Run `scripts/aws/phase3-verify-aws.sh` against applied `demo-ha` infrastructure and archive the
  output with target health and AZ evidence.

Verification:

- `bash -n scripts/aws/phase3-verify-aws.sh scripts/aws/phase3-app-failure-drill.sh` passed.
- Offline verifier regression with fake `aws`/`terraform`/`curl` passed for healthy targets in
  `us-east-1a` and `us-east-1b`.
- Offline verifier regression failed as expected when both healthy targets were in `us-east-1a`.
- `terraform -chdir=infra/aws/free-tier-compose fmt -check` passed.
- `terraform -chdir=infra/aws/free-tier-compose init -backend=false` passed.
- `terraform -chdir=infra/aws/free-tier-compose validate` passed.

## 2026-05-31 Failure Drill Verification Hardening

Fixed the AWS failure-drill helper so it verifies the intended ALB failover behavior instead of
reusing the full steady-state `demo-ha` invariant after intentionally stopping one app node.

Evidence:

- `scripts/aws/phase3-verify-aws.sh` now auto-detects `terraform` or `tofu` when `TF_BIN` is not
  set, matching the preflight and mode helpers.
- `scripts/aws/phase3-verify-aws.sh` can now run a deeper LGTM datasource health gate when
  untracked `GRAFANA_ADMIN_USER` and `GRAFANA_ADMIN_PASSWORD` are provided.
- Normal `scripts/aws/phase3-verify-aws.sh` behavior still requires:
  - at least two healthy ALB targets in `demo-ha`;
  - at least one healthy ALB target in `dev-single-az` or `post-demo`.
- With `PHASE3_REQUIRE_LGTM_DATASOURCES=true`, the verifier fails unless Grafana datasource health
  checks pass for Prometheus, Loki, Tempo, and Pyroscope.
- `scripts/aws/phase3-app-failure-drill.sh` now calls the verifier with:
  - `PHASE3_MIN_HEALTHY_TARGETS=1`;
  - `PHASE3_EXPECT_UNHEALTHY_TARGET` set to the stopped app instance ID.
- During the drill, the verifier checks that the stopped target is registered but not healthy, and
  still requires ALB HTTPS `/healthz` and `/readyz` smoke checks to pass.
- The verifier retries HTTPS smoke checks for short ALB/target-health convergence windows.
- `scripts/aws/phase3-app-failure-drill.sh` now installs an exit trap after confirming `demo-ha`,
  so an interrupted or failed drill attempts to restart the stopped app instance before exiting.

Verification:

- `bash -n scripts/aws/phase3-verify-aws.sh scripts/aws/phase3-app-failure-drill.sh` passed.
- `jq` parsing for mixed healthy/non-healthy ALB target-health JSON returned the expected healthy
  count.

## 2026-05-31 Demo HA Schedule Guard

Added a local planning guard so `demo-ha` cannot be planned accidentally outside the scheduled
Asia/Taipei deploy/test and demo windows.

Evidence:

- `scripts/aws/phase3-mode.sh demo-ha` now evaluates the current `Asia/Taipei` time before writing
  a Terraform/OpenTofu plan.
- The allowed `demo-ha` windows are:
  - 2026-06-03 for Demo 1 deploy/test.
  - 2026-06-04 19:00-21:00 for Demo 1.
  - 2026-06-10 for Demo 2 deploy/test.
  - 2026-06-11 19:00-21:00 for Demo 2.
- Outside those windows, `demo-ha` planning fails unless
  `PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW=true` is set for an explicit rehearsal.
- `dev-single-az` and `post-demo` planning remain available outside demo windows so the
  environment can be kept low-cost or scaled down.

Required runtime proof still missing:

- Run the guarded `demo-ha` plan during an allowed deploy/test or demo window, or record the reason
  for any explicit rehearsal override in deployment evidence.

Verification:

- `bash -n scripts/aws/phase3-mode.sh` passed.
- With a temporary example `terraform.tfvars` and `TF_BIN=true`,
  `PHASE3_NOW_TAIPEI=2026-05-31T12:00:00 scripts/aws/phase3-mode.sh demo-ha` failed as expected.
- With a temporary example `terraform.tfvars` and `TF_BIN=true`,
  `PHASE3_NOW_TAIPEI=2026-06-03T10:00:00 scripts/aws/phase3-mode.sh demo-ha` passed.
- With a temporary example `terraform.tfvars` and `TF_BIN=true`,
  `PHASE3_ALLOW_DEMO_HA_OUTSIDE_WINDOW=true PHASE3_NOW_TAIPEI=2026-05-31T12:00:00 scripts/aws/phase3-mode.sh demo-ha`
  passed as an explicit override path.
- With a temporary example `terraform.tfvars` and `TF_BIN=true`,
  `PHASE3_NOW_TAIPEI=2026-05-31T12:00:00 scripts/aws/phase3-mode.sh dev-single-az` passed.

## 2026-05-31 Deployment Evidence Collector

Added a post-apply evidence collector so demo-day proof can be gathered consistently without
copying secrets into tracked docs.

Evidence:

- `scripts/aws/phase3-collect-evidence.sh` writes ignored run directories under
  `infra/aws/free-tier-compose/cost-reports/evidence/`.
- The collector records AWS identity, safe Terraform outputs, selected cost-gate report, budget and
  budget-notification evidence, ALB target health, app instance state/AZ evidence, `/healthz`,
  `/readyz`, Grafana health, optional Grafana datasource health, and the full
  `phase3-verify-aws.sh` output.
- Sensitive Terraform outputs such as `rds_endpoint` and object-store access key material are not
  written by the collector.

Required runtime proof still missing:

- Run `scripts/aws/phase3-collect-evidence.sh` after a real AWS apply and keep the ignored evidence
  directory for the completion audit.

Verification:

- `bash -n scripts/aws/phase3-collect-evidence.sh` passed.
- `git diff --check` passed.

## 2026-05-31 AWS LGTM Evidence Wiring

Hardened the AWS deployment shape so the required LGTM evidence is backed by AWS runtime data,
not only local Compose assumptions.

Evidence:

- The observability EC2 bootstrap now writes an AWS-specific Prometheus config that scrapes the
  ALB HTTPS `/metrics` endpoint for `job="cets-backend"` instead of local-only
  `backend-1..3` Docker DNS names.
- App EC2 nodes now start an `alloy-agent` container that reads the local Docker socket, applies
  the Phase 3 telemetry redaction rules, and forwards app-node logs to Loki on the observability
  node.
- The ALB security group now also allows HTTP/HTTPS from the observability security group so
  Prometheus can scrape the demo endpoint even when public app CIDRs are restricted.
- `scripts/aws/phase3-verify-aws.sh` supports `PHASE3_REQUIRE_LGTM_EVIDENCE=true`:
  - Prometheus backend scrape evidence through Grafana datasource proxy.
  - Tempo backend trace search through Grafana datasource proxy.
  - Pyroscope backend profile render through Grafana datasource proxy.
  - Loki backend trace-correlated log search through Grafana datasource proxy.
  - SSM-triggered app-node redaction canary checked in Loki.
- `scripts/aws/phase3-collect-evidence.sh` stores raw Grafana proxy responses for Prometheus,
  Tempo, Pyroscope, Loki backend logs, and Loki redaction canary when Grafana credentials are
  provided.
- The deployer IAM bootstrap policy now includes the SSM command permissions needed for the
  app-node redaction canary.

Required runtime proof still missing:

- Rerun `scripts/aws/phase3-bootstrap-iam.sh` with `ROTATE_ACCESS_KEY=false APPLY=true` so the
  deployer policy includes the new SSM permissions.
- After apply, run the verifier with `PHASE3_REQUIRE_LGTM_DATASOURCES=true` and
  `PHASE3_REQUIRE_LGTM_EVIDENCE=true`.
- Keep the ignored collector output for the completion audit.

Verification:

- `bash -n scripts/aws/phase3-verify-aws.sh scripts/aws/phase3-bootstrap-iam.sh scripts/compose/phase3-verify.sh`
  passed.

## 2026-05-31 Guarded AWS Apply And Destroy Wrappers

Added explicit wrappers for the AWS steps that would otherwise be easy to run out of order on a
demo day.

Evidence:

- `scripts/aws/phase3-apply.sh` requires `APPLY=true`, refuses root deployment credentials through
  `phase3-identity-guard.sh`, verifies the selected region in `terraform.tfvars` matches
  `selected-region.env`, verifies the cost-gate report status is `pass`, runs Terraform
  format/init/validate, delegates mode planning to `phase3-mode.sh`, applies the generated plan,
  then runs `phase3-verify-aws.sh` and `phase3-collect-evidence.sh` by default.
- `scripts/aws/phase3-destroy.sh` requires
  `PHASE3_CONFIRM_DESTROY=destroy-cets-phase3` and `PHASE3_DESTROY_REASON`, refuses root deployment
  credentials, verifies selected-region alignment, creates a destroy plan, and applies it.
- `goal.md` and `infra/aws/free-tier-compose/README.md` now include the guarded apply and destroy
  workflows.

Required runtime proof still missing:

- Run `APPLY=true scripts/aws/phase3-apply.sh dev-single-az` after untracked `terraform.tfvars`
  is prepared with real demo values.
- During scheduled windows, run `APPLY=true scripts/aws/phase3-apply.sh demo-ha`, then run the
  failure drill and deep LGTM verifier with Grafana credentials.
- After each demo, run `APPLY=true scripts/aws/phase3-apply.sh post-demo` or the guarded destroy
  helper, then keep the ignored evidence output.

## 2026-05-31 Budget-First Apply And Demo-HA Deep Verification

Strengthened the guarded apply wrapper so it better matches `goal.md` instead of relying on manual
operator sequencing.

Evidence:

- `scripts/aws/phase3-apply.sh` now target-applies `aws_budgets_budget.phase3` before the full
  infrastructure plan.
- The wrapper verifies the 1, 25, 90, 150, and 180 USD `ACTUAL` notifications through the AWS
  Budgets API before applying EC2, ALB, RDS, S3, and observability resources.
- For `demo-ha`, the wrapper now defaults `PHASE3_REQUIRE_LGTM_DATASOURCES=true` and
  `PHASE3_REQUIRE_LGTM_EVIDENCE=true`, and requires untracked Grafana credentials unless
  `PHASE3_DEMO_HA_LIGHT_VERIFY=true` is set for an explicit rehearsal.
- `goal.md` and `infra/aws/free-tier-compose/README.md` now document the budget-first apply and
  default deep LGTM verification behavior.

Required runtime proof still missing:

- Run the updated wrapper against real AWS credentials and preserve the budget verification output
  in the ignored evidence directory.

## 2026-05-31 S3 Export Path Verification Hardening

Added an explicit S3 smoke helper so the required report export backing path is not left as an
implicit assumption during AWS verification.

Evidence:

- `scripts/aws/phase3-s3-smoke.sh` uses Terraform outputs for the selected region and export
  bucket, writes a non-sensitive object under `exports/phase3-smoke/`, confirms the object with
  `head-object`, verifies `AES256` server-side encryption, and deletes the object by default.
- `scripts/aws/phase3-verify-aws.sh` now runs the S3 smoke after `/healthz` and `/readyz`, so
  post-apply verification fails if the export bucket cannot be written, read, or cleaned up.
- `scripts/aws/phase3-collect-evidence.sh` now runs the same smoke with
  `PHASE3_S3_SMOKE_EVIDENCE_DIR`, preserving `put-object`, `head-object`, `delete-object`, and text
  summary evidence in the ignored evidence run directory.
- `goal.md` now lists the S3 smoke helper and clarifies that the AWS verifier covers the export
  bucket path.

Required runtime proof still missing:

- Run the updated verifier/collector after real AWS apply and preserve the ignored S3 smoke evidence
  files for the completion audit.

## 2026-05-31 Cost-Gate Candidate Coverage Hardening

Closed a gap in the lowest-cost region decision. The cost gate now fails when the pricing CSV does
not cover all required candidate regions or omits required cost line items for any candidate.

Evidence:

- `scripts/aws/phase3-cost-gate.sh` requires `us-east-1`, `us-east-2`, and `us-west-2` by default.
- Each candidate region must include EC2 app baseline, EC2 observability, extra demo app nodes, RDS
  single-AZ, RDS Multi-AZ, RDS storage guardrail, ALB hours, ALB LCU minimum, app and observability
  gp3 EBS, S3 export storage, CloudWatch/logging guardrail, data-transfer guardrail, and DNS/ACM
  guardrail line items.
- `infra/aws/free-tier-compose/cost-input.example.csv` now mirrors the required live-pricing line
  item names, so example input still fails because it is marked `replace-before-use`, not because it
  lacks coverage.
- `scripts/aws/phase3-live-price-csv.py` now emits explicit RDS storage and DNS/ACM guardrail rows
  in addition to the live API-backed compute, RDS instance, ALB, EBS, and S3 prices.
- `infra/aws/free-tier-compose/README.md` documents that incomplete CSV evidence cannot select a
  lowest-cost region.

Required runtime proof still missing:

- Generate fresh live pricing CSV with `scripts/aws/phase3-live-price-csv.py`, run the hardened
  cost gate, and preserve the selected-region report before AWS apply.

## 2026-05-31 Budget Window And Cost-Gate Report Hardening

Aligned the AWS Budget scaffold and cost-gate evidence with the explicit two-week demo guardrail
instead of leaving the budget as an open-ended monthly artifact.

Evidence:

- Terraform now sets `aws_budgets_budget.phase3.time_period_start` and `time_period_end`.
- The default budget window is `2026-05-31_16:00` through `2026-06-14_16:00` UTC, which maps to
  2026-06-01 00:00 through 2026-06-15 00:00 Asia/Taipei.
- `scripts/aws/phase3-init-local-config.sh` writes the same default window into untracked
  `terraform.tfvars`, with `PHASE3_BUDGET_START_UTC` and `PHASE3_BUDGET_END_UTC` available only for
  explicit replacement windows.
- `scripts/aws/phase3-cost-gate.sh` now records forecast window metadata in the JSON report and
  uses Python's CSV parser for totals, so quoted source fields with commas cannot corrupt the
  region totals.
- The selected region is chosen only from the required candidate region set, even if an input CSV
  contains extra non-candidate rows.
- `scripts/aws/phase3-apply.sh` verifies the AWS Budget has a 180 USD limit, USD unit, monthly time
  unit, the exact 2026-05-31_16:00 through 2026-06-14_16:00 UTC guardrail window, and all required
  actual-notification thresholds before applying EC2, ALB, RDS, S3, and observability resources.
- `scripts/aws/phase3-collect-evidence.sh` now stores safe Terraform outputs for the budget limit,
  time period, and notification thresholds in the ignored evidence directory.

Required runtime proof still missing:

- Regenerate live cost evidence with the new report schema.
- Apply the budget-first wrapper against AWS and preserve the resulting Budget API evidence.

## 2026-05-31 Fresh Live Pricing And Completion Audit Helper

Regenerated the live pricing evidence after the budget-window report schema was added, then added a
read-only completion audit helper for future goal closure checks.

Evidence:

- Least-privilege deployer identity still resolves through AWS STS:
  `arn:aws:iam::992382443806:user/cets-phase3-deployer`.
- `AWS_PROFILE=cets-phase3-deployer scripts/aws/phase3-live-price-csv.py --out infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv`
  generated a fresh ignored pricing CSV.
- `scripts/aws/phase3-cost-gate.sh infra/aws/free-tier-compose/cost-reports/live-pricing-input.csv`
  wrote a fresh ignored selected-region report with the new window metadata.
- Fresh cost-gate result:
  - Selected region: `us-east-1`.
  - Selected two-week forecast: `44.4957 USD`.
  - Candidate totals:
    - `us-east-1`: `44.4957 USD`
    - `us-east-2`: `44.4957 USD`
    - `us-west-2`: `44.4957 USD`
  - Forecast window: 2026-06-01 00:00 through 2026-06-15 00:00 Asia/Taipei.
  - Status: `pass`; below both the `150 USD` risk stop and `180 USD` hard cap.
- `scripts/aws/phase3-completion-audit.py` now checks local completion evidence without calling
  AWS or printing secrets. It is expected to fail until real AWS apply evidence exists.

Verification:

- `python3 -m py_compile scripts/aws/phase3-completion-audit.py scripts/aws/phase3-live-price-csv.py`
  passed.
- `scripts/aws/phase3-completion-audit.py --allow-missing-post-demo` failed as expected because
  local `terraform.tfvars` and real AWS evidence are missing.
- A synthetic non-secret evidence directory passed
  `scripts/aws/phase3-completion-audit.py --selected-region-env ... --evidence-root ... --allow-missing-post-demo`.
- `git diff --check` passed.

Required runtime proof still missing:

- Real untracked `terraform.tfvars`.
- AWS Budget API evidence after budget-first apply.
- Dev and demo-ha runtime evidence, failure drill evidence, LGTM/redaction evidence, and post-demo
  scale-down or destroy evidence.

## 2026-05-31 Exact Budget Window Verification

Tightened the local deploy gates so the AWS Budget and cost-gate evidence must prove the same
two-week window required by `goal.md`, not just any non-empty budget start/end.

Evidence:

- `scripts/aws/phase3-preflight.sh` now refuses `terraform.tfvars` values that override the
  default Budget window away from `2026-05-31_16:00` through `2026-06-14_16:00` UTC.
- `scripts/aws/phase3-preflight.sh` now refuses cost-gate reports whose Asia/Taipei forecast window
  is not exactly `2026-06-01T00:00:00+08:00` through `2026-06-15T00:00:00+08:00`.
- `scripts/aws/phase3-apply.sh` now checks the live AWS Budget API response for that exact UTC
  window before applying the broader infrastructure plan.
- `scripts/aws/phase3-completion-audit.py` now requires exact Budget output, Budget API, and
  cost-gate report windows before it can pass.

Required runtime proof still missing:

- Run the budget-first wrapper against AWS with real untracked `terraform.tfvars`.
- Preserve the AWS Budget API evidence collected after apply.

## 2026-05-31 Multi-Run Completion Audit And Drill Evidence

Hardened goal-closure checks so demo-ha proof and post-demo scale-down proof cannot be collapsed
into a single latest evidence directory.

Evidence:

- `scripts/aws/phase3-completion-audit.py` now auto-selects the latest ignored `demo-ha` evidence
  directory for HA/LGTM/redaction/failure-drill proof and the latest ignored `dev-single-az`
  evidence directory for post-demo scale-down proof.
- The audit now requires `demo-ha` evidence to have two healthy ALB targets, healthy AZ spread,
  LGTM datasource/proxy responses, redaction canary proof, and a failure-drill evidence file.
- `scripts/aws/phase3-app-failure-drill.sh` now supports `PHASE3_DRILL_EVIDENCE_DIR`, preserving
  stopped-target, verifier, restored-target, and summary evidence in the ignored demo evidence
  directory.
- `infra/aws/free-tier-compose/README.md` documents running the failure drill with the same
  evidence directory created for the demo-ha collector run.
- `scripts/aws/phase3-apply.sh demo-ha` now runs the app-node failure drill by default after the
  post-apply verifier and before evidence collection, using one `PHASE3_EVIDENCE_RUN_ID` for both
  drill and collector output. Set `PHASE3_RUN_FAILURE_DRILL=false` only for a documented rehearsal.
- The failure drill now suppresses deep LGTM evidence checks while an app node is intentionally
  stopped, keeping the drill focused on ALB failover while the normal demo-ha verifier remains
  responsible for Grafana/LGTM/redaction proof.

Required runtime proof still missing:

- Run `APPLY=true scripts/aws/phase3-apply.sh demo-ha` with real untracked inputs and Grafana
  credentials so the verifier, automatic failure drill, and collector populate a real demo-ha
  evidence directory, then collect post-demo `dev-single-az` evidence after scale-down.

## 2026-05-31 CI Self-Hosted Runner Fallback

GitHub Actions failed to start the remote `changes` job for run `26700543156` on SHA `da31fc3`
because the account had a billing/spending-limit block. The job never reached checkout or project
commands, so this was not a code or workflow logic failure.

Evidence:

- `gh pr checks 56 --json name,state,bucket,link,workflow` showed `changes` failed and all other
  CI jobs skipped.
- The check-run annotation for `changes` said the job was not started because recent account
  payments failed or the spending limit needed to be increased.
- Official GitHub billing docs state that GitHub Actions usage is free for self-hosted runners,
  while private-repository GitHub-hosted runners consume account quota.
- Official GitHub runner docs route workflow jobs to self-hosted runners through labels or groups.

Implemented mitigation:

- `.github/workflows/ci.yml` now resolves every job runner from repository variable
  `CI_RUNNER_LABELS`, falling back to `["ubuntu-latest"]` when the variable is unset.
- `docs/ci-self-hosted-runner.md` documents registering an Ubuntu x64 runner with labels
  `self-hosted`, `linux`, `x64`, and `cets-ci`, then setting
  `CI_RUNNER_LABELS=["self-hosted","linux","x64","cets-ci"]` to route checks away from
  GitHub-hosted minutes.
- `scripts/ci/self-hosted-runner-preflight.sh` verifies CPU, memory, disk, Node.js, pnpm, Go,
  Ruby, Docker, Compose, Buildx, and optional Docker image pull/run readiness before the repository
  variable is switched.

The workflow is intentionally still opt-in. Do not switch the repo variable until the runner is
online with Docker, Compose, and Buildx available, or jobs will queue indefinitely.

## 2026-05-31 Destroy Evidence Completion Path

Aligned the post-demo cleanup evidence path with `goal.md`, which allows either scaling back to
`dev-single-az` or destroying resources after the final demo.

Evidence:

- `scripts/aws/phase3-destroy.sh` now writes ignored destroy evidence by default under
  `infra/aws/free-tier-compose/cost-reports/evidence/destroy-<timestamp>/`.
- The destroy evidence includes identity-guard output, AWS identity, Terraform init/plan/apply
  logs, a `destroy-summary.json`, and `terraform-state-list-after-destroy.txt`.
- `scripts/aws/phase3-completion-audit.py` now accepts post-demo destroy evidence when
  `destroy-summary.json` is marked `destroyed`, the selected region matches the cost gate, a
  destroy reason is present, and the post-destroy Terraform state list is empty.
- `infra/aws/free-tier-compose/README.md` documents the destroy evidence files and the audit
  behavior.

Verification:

- `bash -n scripts/aws/phase3-destroy.sh` passed.
- `python3 -m py_compile scripts/aws/phase3-completion-audit.py` passed.
- A synthetic non-secret evidence root with `demo-ha` proof plus destroy evidence passed
  `scripts/aws/phase3-completion-audit.py --selected-region-env ... --evidence-root ...`.

Required runtime proof still missing:

- Run the guarded destroy helper after the final demo only if full cleanup is selected, then keep
  the ignored destroy evidence directory for the completion audit.

## 2026-05-31 Runtime Input Generation Hardening

Tightened the untracked `terraform.tfvars` generation path so bad runtime inputs fail before a
Terraform preflight or AWS apply is attempted.

Evidence:

- `scripts/aws/phase3-init-local-config.sh` now verifies the selected-region file points at an
  existing cost report whose status is `pass`.
- The generator now refuses cost reports outside the exact 2026-06-01 00:00 through 2026-06-15
  00:00 Asia/Taipei forecast window and refuses budget window overrides outside
  `2026-05-31_16:00` through `2026-06-14_16:00` UTC.
- The generator validates the demo hostname, budget email shape, repository URL scheme, and
  allowlist/public CIDRs before writing the ignored `terraform.tfvars`.
- The output write now uses a temporary file and moves it into place only after all validations and
  secret generation succeed.

Required runtime proof still missing:

- Provide real untracked domain, budget email, allowlist CIDRs, repository URL, and secrets, then
  run `scripts/aws/phase3-init-local-config.sh` and `scripts/aws/phase3-preflight.sh`.

## 2026-05-31 Self-Hosted Runner Safe Switch

GitHub Actions still fails before checkout when `CI_RUNNER_LABELS` is unset because the repository
falls back to GitHub-hosted `ubuntu-latest`, which is currently blocked by account billing or
spending-limit state.

Evidence:

- Remote run `26702462751` on SHA `d6f77f8` failed in `changes` before project commands ran.
- The `changes` check-run annotation says the job was not started because recent account payments
  failed or the spending limit needed to be increased.
- Local full `act push` passed on SHA `d6f77f8`, so the remaining remote failure is runner
  provisioning, not a project test failure.

Implemented mitigation:

- Added `scripts/ci/enable-self-hosted-runner.sh` to make the self-hosted runner switch safer.
- The script checks GitHub repository runners and requires at least one online, non-busy runner
  with labels `self-hosted`, `linux`, `x64`, and `cets-ci` before setting `CI_RUNNER_LABELS`.
- The script is dry-run by default; `APPLY=true` is required before it changes repository
  variables. `CETS_RUNNER_SWITCH_MODE=disable` removes the variable and returns the workflow to
  `ubuntu-latest`.
- `docs/ci-self-hosted-runner.md` now documents the safe CLI switch and cleanup path.
- A live dry run currently cannot inspect repository runners with the authenticated GitHub identity:
  `gh repo view` reports `WRITE` permission, while the runner-list API returns `404`. Use a
  repository administrator account or the GitHub settings UI for the final switch.

Required external action still missing:

- Register a repository self-hosted runner with label `cets-ci`, run
  `scripts/ci/self-hosted-runner-preflight.sh` on that host, then run
  `APPLY=true scripts/ci/enable-self-hosted-runner.sh` from an authenticated checkout.

## 2026-05-31 Demo-HA Pre-Write Schedule Guard

Closed an ordering gap in the guarded AWS apply flow. `scripts/aws/phase3-apply.sh demo-ha` now
checks the scheduled Asia/Taipei deploy/test or demo window before it target-applies the AWS Budget
guardrail or performs any other AWS write.

Evidence:

- Added `scripts/aws/phase3-demo-window-guard.py` as the shared schedule guard used by
  `phase3-mode.sh` and `phase3-apply.sh`.
- `phase3-mode.sh demo-ha` still refuses planning outside:
  - 2026-06-03 for Demo 1 deploy/test.
  - 2026-06-04 19:00-21:00 for Demo 1.
  - 2026-06-10 for Demo 2 deploy/test.
  - 2026-06-11 19:00-21:00 for Demo 2.
- `phase3-apply.sh demo-ha` now runs that same guard after local identity/static validation and
  before `aws_budgets_budget.phase3` target apply.
- A fake Terraform/AWS regression confirmed that
  `APPLY=true PHASE3_NOW_TAIPEI=2026-05-31T12:00:00 scripts/aws/phase3-apply.sh demo-ha` fails
  before any Budget, EC2, ALB, RDS, or S3 write path. The only fake calls made before refusal were
  AWS identity read plus Terraform `fmt`, `init`, and `validate`.

Verification:

- `bash -n scripts/aws/phase3-mode.sh scripts/aws/phase3-apply.sh` passed.
- `python3 -m py_compile scripts/aws/phase3-demo-window-guard.py scripts/aws/phase3-completion-audit.py scripts/aws/phase3-live-price-csv.py`
  passed.
- `scripts/aws/phase3-demo-window-guard.py` returned:
  - failure for `demo-ha` at `2026-05-31T12:00:00 Asia/Taipei`;
  - success for `demo-ha` at `2026-06-03T10:00:00 Asia/Taipei`;
  - success for `dev-single-az` outside the demo window;
  - success for explicit `--allow-outside-window` rehearsal override.
