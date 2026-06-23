# Tekton Pruner

Kubernetes controller and admission webhook that automatically prunes
Tekton `PipelineRun` and `TaskRun` resources based on configurable
time-based (TTL) and history-based retention policies.

---

## Build & Test Commands

```bash
# Build
make all                       # format + build controller and webhook binaries
make bin/controller             # controller binary only
make bin/webhook                # webhook binary only

# Test — runs without a cluster
make test-unit                  # unit tests (race detector + coverage, 5m timeout)
make test-unit-verbose          # same, with -v

# Lint — must pass before every PR
make fmt                        # go fmt ./...
go vet ./...                    # static analysis
make yamllint                   # YAML lint on config/ and .github/workflows/

# Deploy to a cluster (requires ko + KO_DOCKER_REPO)
ko apply -f config/             # deploy everything
ko apply -f config/controller.yaml  # redeploy controller only
ko apply -f config/webhook.yaml     # redeploy webhook only
ko delete -f config/            # tear down

# Local kind cluster from scratch
make dev-setup                  # runs hack/dev/kind/install.sh

# Code generation — required after API type changes
./hack/update-codegen.sh

# Dependency update — required after go.mod changes
./hack/update-deps.sh
```

E2E tests require a live cluster with Tekton Pipelines and are tagged
`//go:build e2e`. Run with `go test -timeout 20m -tags e2e -v ./test/...`

---

## Key Conventions

1. **Hierarchical ConfigMap configuration.** Settings resolve
   Global → Namespace → Resource Selector (most specific wins).
   The global ConfigMap is `tekton-pruner-default-spec`; namespace
   overrides go in `tekton-pruner-namespace-spec`. Selectors (label/
   annotation matching) only work in namespace-level ConfigMaps.

2. **Annotation keys live in `pkg/config/constants.go`.**
   Never hardcode annotation or label strings elsewhere. Key constants:
   `AnnotationTTLSecondsAfterFinished`, `AnnotationSuccessfulHistoryLimit`,
   `AnnotationFailedHistoryLimit`.

3. **Reconciler idempotency.** Re-running any reconcile loop must be
   safe. Use `controller.NewPermanentError` only for truly unrecoverable
   conditions.

4. **Structured logging with `zap`.** `Info` for normal operations,
   `Error` for failures, `Debug` for verbose paths. Metrics via
   `pkg/metrics/`.

5. **Vendored dependencies.** Run `./hack/update-deps.sh` after
   any `go.mod` change, then commit the `vendor/` diff.

---

## Architecture (non-obvious parts)

See [ARCHITECTURE.md](./ARCHITECTURE.md) for the full C4 diagram and
design decisions.

**Built on Knative controller runtime.** Reconcilers implement
`ReconcileKind` from `knative.dev/pkg/reconciler`. Do not use
`controller-runtime` — it is not in this project.

**Two ConfigMaps, not one.** The global ConfigMap
(`tekton-pruner-default-spec`) holds cluster-wide defaults and can
embed per-namespace defaults under `namespaces:`. The namespace
ConfigMap (`tekton-pruner-namespace-spec`) lives in each namespace
and supports selector-based rules (`pipelineRuns:` / `taskRuns:`
arrays). Selectors in the global ConfigMap are silently ignored.

**Config resolution is in `pkg/config/`, not in reconcilers.** All
hierarchy resolution (global → namespace → resource selector →
annotation), field validation, and type parsing live in
`pkg/config/config.go` and `pkg/config/helper.go`. Reconcilers call
`GetPipeline*` / `GetTask*` methods on `prunerConfigStore` — they
never parse ConfigMaps directly.

**TTL and history are independent code paths.** TTL logic is in
`pkg/config/ttl_handler.go`; history-limit logic is in
`pkg/config/history_limiter.go`. Both can be active simultaneously
on the same resource.

**Four controllers + one webhook:**

| Component | Package |
|-----------|---------|
| Main Pruner Controller | `pkg/reconciler/tektonpruner/` |
| PipelineRun Controller | `pkg/reconciler/pipelinerun/` |
| TaskRun Controller | `pkg/reconciler/taskrun/` |
| Namespace Config Controller | `pkg/reconciler/namespaceprunerconfig/` |
| Validating Webhook | `pkg/webhook/` |

Entrypoints: `cmd/controller/main.go`, `cmd/webhook/main.go`.

---

## Pattern References for Common Changes

| Change | Canonical example to follow |
|--------|----------------------------|
| Reconciler structure | `pkg/reconciler/tektonpruner/` |
| Event-driven handler | `pkg/reconciler/pipelinerun/` |
| ConfigMap parsing + hierarchy | `pkg/config/config.go`, `pkg/config/helper.go` |
| Validation logic | `pkg/config/config_validation_test.go` |
| Webhook admission | `pkg/webhook/configmapvalidation.go` |
| Metrics registration | `pkg/metrics/metrics.go` |
| Constants (annotations, labels) | `pkg/config/constants.go` |
| Default ConfigMap YAML | `config/600-tekton-pruner-default-spec.yaml` |

---

## PR Conventions

- Follow [Tekton community standards](https://github.com/tektoncd/community/blob/main/standards.md)
- Conventional commit messages: `<type>(<scope>): <description>`
  (see the `commit-message` skill for details)
- Before opening a PR, run:
  ```bash
  make fmt && go vet ./... && make test-unit && make yamllint
  ```
- One concern per PR. Separate refactoring from behavior changes.
- Include `Signed-off-by` trailer for DCO compliance

---

## Windows Checkout

`CLAUDE.md` points to `AGENTS.md`, and `.claude/skills` points to
`../.agents/skills`. This works on Linux, macOS, and GitHub; on
Windows, enable symlinks when cloning:

```bash
git clone -c core.symlinks=true https://github.com/tektoncd/pruner.git
```

---

## Skills

For complex workflows, use these repo-local skills:

- **Commit messages**: Conventional commits with component scopes,
  line length validation, DCO `Signed-off-by`, and `Assisted-by` trailers.
  Trigger: "create commit", "commit changes", "generate commit message"
- **Code review**: Review PRs against Tekton community and pruner-specific
  quality standards. Trigger: "review PR", "review this change"
- **Local development**: Bootstrap a kind cluster, deploy the operator,
  set up observability. Trigger: "set up dev environment", "local cluster"
- **Running tests**: Run unit tests, linters, and e2e tests.
  Trigger: "run tests", "check CI"
- **Pruner config**: Author, validate, and debug ConfigMap configuration.
  Trigger: "configure pruner", "write ConfigMap"
- **Filing issues**: File well-crafted GitHub issues for bugs and features.
  Trigger: "file issue", "report bug"
- **Sync plumbing workflows**: Update pinned SHA digests for
  tektoncd/plumbing reusable workflows. Trigger: "update plumbing",
  "sync workflow pins"
