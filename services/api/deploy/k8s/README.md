# Kubernetes deployment manifests

These manifests are an optional local or staging-safe deployment template for the
Corporate Event Ticketing System. Docker Compose remains the Phase 1 primary
local entry point.

Apply the stack with:

```sh
kubectl apply -k services/api/deploy/k8s
```

The stack creates the API, worker, migration job, suspended seed job,
PostgreSQL, Redis, MinIO, and Mailhog in the `cets` namespace. The API image
defaults to `cets-api:dev`; override it with Kustomize before using a shared
staging cluster.

The `cets-api-secret` manifest contains placeholder values only. Replace those
values with cluster-managed secrets before shared staging use. The committed
placeholders are intentionally not production credentials.

The `cets-seed` job is suspended by default so staging does not receive demo
data accidentally. To run it for a demo namespace, patch it explicitly:

```sh
kubectl patch job cets-seed -n cets --type merge -p '{"spec":{"suspend":false}}'
```
