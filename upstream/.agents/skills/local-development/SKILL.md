---
name: local-development
description: >-
  Set up a local development environment for tekton-pruner. Use when onboarding,
  bootstrapping a kind cluster, deploying the operator locally, or iterating on
  controller and webhook changes. Covers kind, ko, Tekton Pipelines, and the
  observability stack (Prometheus + Jaeger).
license: Apache-2.0
compatibility: Requires go, kind, ko, kubectl, docker (or podman)
metadata:
  project: tekton-pruner
  maintainers: tektoncd
allowed-tools: Bash(kind:*) Bash(ko:*) Bash(kubectl:*) Bash(docker:*) Bash(make:*) Read
---

# Local Development

tekton-pruner is a Kubernetes controller and admission webhook that prunes
Tekton `PipelineRun` and `TaskRun` resources based on TTL or history limits
configured via a `ConfigMap` or per-namespace annotations.

## Prerequisites

Install the following tools before proceeding:

| Tool | Purpose |
|------|---------|
| `go` (≥ 1.26) | Build language |
| `kind` | Local Kubernetes cluster |
| `ko` | Build and push Go container images |
| `kubectl` | Interact with the cluster |
| `docker` or `podman` | Container runtime (podman sets `CONTAINER_RUNTIME=podman`) |

## Quick-Start: Full Cluster Setup

The recommended path for a fresh local environment:

```bash
cd hack/dev/kind/
./install.sh
```

This script:
1. Starts a local container registry on port `5000` (`kind-registry`)
2. Creates a `kind` cluster named `kind` (override with `KIND_CLUSTER_NAME`)
3. Wires the cluster to the registry
4. Sets `KUBECONFIG` to `~/.kube/config.kind`
5. Deploys Tekton Pipelines and the pruner controller via `ko apply`

After it completes, verify:

```bash
export KUBECONFIG=~/.kube/config.kind
kubectl get pods -n tekton-pipelines
```

## Deploy Only the Pruner

If you already have a cluster with Tekton Pipelines running:

```bash
export KO_DOCKER_REPO=<your-registry>   # e.g. ko.local or localhost:5000
ko apply -f config/
```

To redeploy individual components after code changes:

```bash
# Controller only
ko apply -f config/controller.yaml

# Webhook only
ko apply -f config/webhook.yaml
```

## Observability Stack (Optional)

For metrics and tracing during development:

```bash
./hack/setup-observability-dev.sh
```

Endpoints after setup:

| Service | URL |
|---------|-----|
| Prometheus | http://localhost:9091 |
| Jaeger (tracing) | http://localhost:16686 |
| Pruner metrics | http://localhost:9090/metrics |

## Iterating on Code

1. Edit Go source under `pkg/` or `cmd/`.
2. Run unit tests to verify: `make test-unit`
3. Format code: `make fmt`
4. Rebuild and redeploy (ensure `KO_DOCKER_REPO` is set — see
   "Deploy Only the Pruner" above): `ko apply -f config/controller.yaml`
5. Check controller logs:

```bash
kubectl -n tekton-pipelines logs deployment/tekton-pruner-controller -f
```

## Tear Down

```bash
ko delete -f config/
# or to destroy the entire kind cluster:
kind delete cluster --name kind
```

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `KO_DOCKER_REPO` | _(required)_ | Image registry for ko builds |
| `KIND_CLUSTER_NAME` | `kind` | Name of the kind cluster |
| `CONTAINER_RUNTIME` | `docker` | Set to `podman` for rootless builds |

## Common Issues

- **`ko` not found**: Run `make apply` — it auto-installs `ko` into `.bin/`.
- **Registry not reachable**: Ensure `kind-registry` container is running:
  `docker ps | grep kind-registry`
- **Webhook TLS errors**: The webhook cert is managed by the operator. If
  invalid, delete the `Secret` and restart the webhook pod.
