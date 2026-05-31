# Phase 3 Compose HA k6 + LGTM Goal

## Objective

Iterate on the Phase 3 single-machine Docker Compose HA simulation until k6 load proves the
edge-to-gateway-to-frontend-to-backend path distributes traffic across three frontend replicas and
three backend replicas, and LGTM evidence supports a RED -> trace -> logs -> profile investigation
workflow.

This goal follows `docs/specs/phase3-local-ha-compose-lgtm.md`. Kubernetes, Cloudflare, pod/node
graph, production multi-AZ HA, disaster recovery, and multi-region active-active are not completion
criteria for this iteration.

## Required Evidence

- `scripts/compose/phase3-deploy.sh` starts the Phase 3 Compose topology with gateway, frontend,
  backend, worker-isolation, and LGTM services healthy.
- k6 smoke and stress profiles run through the external edge URL.
- k6 evidence proves at least three unique frontend replicas and three unique backend replicas
  served traffic during the run.
- Controlled error traffic is generated through existing application error paths and is visible in
  RED metrics without being counted as an unexpected k6 failure.
- Backend HTTP requests emit OpenTelemetry spans with route-pattern names, not raw identifiers.
- Backend logs include both the existing `trace_id` and an `otel_trace_id` value compatible with
  Grafana/Loki trace drilldown.
- Tempo returns backend traces, Prometheus returns non-zero service graph/span metrics, Loki returns
  trace-correlated backend logs, and Pyroscope returns backend CPU profile samples.
- Telemetry redaction canaries pass: no raw PII, signed QR/token material, provider secrets, raw
  idempotency keys, email bodies, or raw recipient emails are queryable in Loki.
- `scripts/compose/phase3-drill.sh` still proves one gateway, frontend, or backend replica can be
  stopped and restored without sustained smoke-test failure.

## Iteration Rules

- Keep changes small and independently verifiable.
- Use `gpt-5.3-codex-spark` for narrow parallel review or mapping tasks when useful, especially k6
  topology evidence, observability contract checks, and final diff review.
- Do not mark the goal complete until every required evidence item has current local evidence or a
  concrete blocker is recorded here.
- If a check fails, fix the failure and rerun the relevant check before moving on.
- Do not add production-only fault endpoints. Controlled error load must use existing business or
  validation error paths.
- Do not route booking, ticket, check-in, or audit truth through Redis, observability storage, local
  files, or worker memory.

## Stop Conditions

- Stop if any document or script describes this Compose simulation as production HA, production
  multi-AZ, completed Kubernetes HA, completed Cloudflare HA, or completed disaster recovery.
- Stop if telemetry exposes protected sensitive values.
- Stop if k6 distribution evidence can pass without traffic traversing the Phase 3 edge URL.
- Stop if verification can pass without backend traces, trace-correlated logs, RED metrics, profile
  samples, or replica distribution evidence.
- Stop if tests or Sonar identify quality issues that remain unresolved.

## Verification Sequence

```sh
docker compose --env-file services/api/deploy/.env.example \
  -f services/api/deploy/compose.yaml \
  -f services/api/deploy/compose.worker-isolation.yaml \
  -f services/api/deploy/compose.phase3-ha.yaml \
  --profile phase3-ha \
  --profile worker-isolation \
  --profile observability \
  --profile phase3-canary \
  config

scripts/compose/phase3-deploy.sh
K6_PHASE3_PROFILE=smoke scripts/compose/phase3-k6.sh
K6_PHASE3_PROFILE=stress scripts/compose/phase3-k6.sh
K6_PHASE3_PROFILE=investigate scripts/compose/phase3-k6.sh
scripts/compose/phase3-verify.sh
scripts/compose/phase3-drill.sh
cd services/api && go test ./... -count=1
pnpm --filter cets-web lint
pnpm --filter cets-web test
pnpm --filter cets-web build
pnpm sonar:scan
act push
```

Run Sonar only when local scanner tooling and required Sonar environment are available. Before
pushing, `act push` must pass; if it fails, fix the failure or record the blocker with the failed job
and log excerpt.

## Current State

- Phase 3 Compose topology, LGTM services, and deploy/verify/drill scripts already exist.
- The current gap is runtime proof: existing checks declare replicas and LGTM services, but do not
  yet prove k6 load distribution across frontend/backend replicas or complete request trace/log
  correlation.
