---
name: running-tests
description: >-
  Run unit tests, linters, and e2e tests for tekton-pruner. Use when verifying
  a code change, preparing a PR, debugging a test failure, or checking overall
  project health. Covers `make test-unit`, `make fmt`, `make yamllint`, and
  the e2e test suite requiring a live cluster.
license: Apache-2.0
compatibility: Requires go, make; e2e tests also require a running Kubernetes cluster with Tekton Pipelines
metadata:
  project: tekton-pruner
allowed-tools: Bash(make:*) Bash(go test:*) Bash(go vet:*) Bash(kubectl:*) Read Grep
---

# Running Tests

## Unit Tests

Unit tests are pure Go tests with no cluster dependency.

```bash
# Run all unit tests (with race detector and coverage)
make test-unit

# Verbose output
make test-unit-verbose

# Run tests for a specific package
go test -timeout 5m -race -cover ./pkg/config/...
go test -timeout 5m -race -cover ./pkg/reconciler/...
```

Key test packages:

| Package | What it tests |
|---------|--------------|
| `pkg/config/` | ConfigMap parsing, validation, selector logic, TTL handler, history limiter |
| `pkg/metrics/` | Prometheus metrics registration and recording |
| `pkg/webhook/` | ConfigMap admission webhook validation |
| `pkg/version/` | Version string formatting |
| `cmd/controller/` | Controller main entrypoint |

## Code Formatting

Always format before committing:

```bash
make fmt
# equivalent to: go fmt ./...
```

If `make fmt` produces diffs, the CI will reject the PR.

## YAML Lint

```bash
make yamllint
```

Runs `yamllint` against `config/` and `.github/workflows/`. Fix any reported
errors before pushing. If `yamllint` is not installed, install it with
`pip install yamllint`.

## Static Analysis / Vet

```bash
go vet ./...
```

Run this to catch common Go mistakes not caught by `go fmt`.

## e2e Tests

e2e tests require a running Kubernetes cluster with Tekton Pipelines and the
pruner deployed. They are gated by the `e2e` build tag.

1. Ensure a cluster is available and `KUBECONFIG` points to it.
2. Deploy the pruner: `ko apply -f config/`
3. Run:

```bash
go test -timeout 20m -tags e2e -v ./test/...
```

> **Note:** The e2e suite is marked `TO BE UPDATED` in `DEVELOPMENT.md` —
> if tests are missing, file an issue or add them before the next release.

## CI Checks

Pull requests are gated by GitHub Actions. Locally replicate CI with:

```bash
make fmt          # formatting
go vet ./...      # static analysis
make test-unit    # unit tests
make yamllint     # YAML lint
```

All four must pass before requesting a review.

## Interpreting Test Failures

- **`-race` detector failures**: indicate a concurrency bug; do not suppress.
- **Coverage drops**: check `pkg/config/` and `pkg/reconciler/` — these have
  the highest coverage requirements.
- **ConfigMap validation failures**: review `pkg/config/config_validation_test.go`
  and the hierarchical test file for the expected spec format.
