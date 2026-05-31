# CI Self-Hosted Runner Runbook

## Purpose

Use a dedicated GitHub Actions self-hosted runner when GitHub-hosted minutes, billing state, or
spending limits prevent PR checks from starting. The workflow remains on `ubuntu-latest` by
default. Switching to self-hosted is an explicit repository variable change, not a code change.

Official references:

- GitHub Actions billing: https://docs.github.com/en/billing/concepts/product-billing/github-actions
- Self-hosted runner workflow labels: https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners/use-in-a-workflow
- Runner selection syntax: https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job

## Recommended Runner

- Ubuntu Linux x64 host with at least 4 CPU cores, 12 GB RAM, and 80 GB free disk.
- Docker Engine with the Compose plugin and Buildx available to the runner user.
- Network access to GitHub, npm/pnpm registries, Go module proxy, Docker Hub, public ECR, and
  Ubuntu package repositories.
- A dedicated non-root OS user for the runner process.
- Custom runner label: `cets-ci`.

Prefer a local machine or already-paid host for this project. Do not create a long-running paid AWS
instance just to save GitHub Actions minutes unless it is included in the same Phase 3 cost gate.

## Registration

1. Open the repository on GitHub.
2. Go to `Settings` -> `Actions` -> `Runners` -> `New self-hosted runner`.
3. Choose Linux x64.
4. Follow GitHub's generated download and configure commands on the runner host.
5. Add the custom label `cets-ci` during configuration.
6. From a checkout of this repository on the runner host, run:

   ```bash
   CETS_RUNNER_LABELS='["self-hosted","linux","x64","cets-ci"]' scripts/ci/self-hosted-runner-preflight.sh
   ```

7. Install and start the runner as a service only after the preflight passes.

Do not commit the generated registration token, runner URL, `.credentials`, `.runner`, or service
environment files. Rotate or remove the runner from GitHub if the host is no longer controlled.

## Workflow Switch

The CI workflow uses this runner expression:

```yaml
runs-on: ${{ fromJSON(vars.CI_RUNNER_LABELS || '["ubuntu-latest"]') }}
```

Default behavior needs no repository variable and stays on GitHub-hosted `ubuntu-latest`.

To route CI to the self-hosted runner:

1. Go to `Settings` -> `Secrets and variables` -> `Actions` -> `Variables`.
2. Add repository variable `CI_RUNNER_LABELS`.
3. Set its value to `["self-hosted","linux","x64","cets-ci"]`.
4. Confirm the runner is `Idle` in `Settings` -> `Actions` -> `Runners`.
5. Re-run the PR workflow.

The safer CLI path checks the registered runner before it changes the repository variable:

```bash
scripts/ci/enable-self-hosted-runner.sh
APPLY=true scripts/ci/enable-self-hosted-runner.sh
```

The first command is a dry run. The second command sets `CI_RUNNER_LABELS` only after GitHub
returns at least one online, non-busy repository runner with labels `self-hosted`, `linux`, `x64`,
and `cets-ci`.

The CLI path requires a GitHub identity that can list repository self-hosted runners and update
Actions variables. If GitHub returns `404` while `gh repo view` works, re-authenticate with a
repository administrator account or do the switch from the GitHub settings UI.

To return to GitHub-hosted runners, delete `CI_RUNNER_LABELS` or set it to `["ubuntu-latest"]`.
The CLI cleanup path is:

```bash
APPLY=true CETS_RUNNER_SWITCH_MODE=disable scripts/ci/enable-self-hosted-runner.sh
```

## Verification

After switching, the first workflow should show each job running on the `cets-ci` runner. The
`backend-test` job uses a PostgreSQL service container, and `live-gates` uses Docker Compose and
Buildx, so both jobs prove the host is correctly prepared.

If jobs remain queued, confirm the runner is online and has the exact `cets-ci` label. If jobs fail
before starting with a billing or spending-limit message, keep the workflow on local `act push`
verification until the GitHub account billing block is cleared.

If the runner is hosted on AWS, record the instance type, schedule, and expected two-week cost in
the Phase 3 cost gate before enabling it. A local machine or already-paid host is preferred because
it avoids shifting the GitHub billing problem into unmanaged AWS spend.
