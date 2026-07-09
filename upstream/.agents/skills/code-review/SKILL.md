---
name: code-review
description: >-
  Review a pull request or code change in tekton-pruner. Use when asked to
  review a PR, evaluate a diff, or assess code quality. Applies Tekton community
  standards, Go best practices, operator patterns, and pruner-specific domain
  knowledge (reconciler logic, ConfigMap validation, TTL/history semantics).
license: Apache-2.0
metadata:
  project: tekton-pruner
allowed-tools: Read Grep Glob Bash(git diff:*) Bash(git log:*)
---

# Code Review

tekton-pruner follows [Tekton community review standards](https://github.com/tektoncd/community/blob/main/standards.md).

## Review Checklist

### Correctness

- [ ] Logic changes have corresponding unit tests in the relevant `_test.go` file
- [ ] New ConfigMap fields are covered in `pkg/config/config_validation_test.go`
  and `config_validation_hierarchical_test.go`
- [ ] Reconciler changes do not break idempotency (re-running reconcile must be safe)
- [ ] TTL and history-limit logic in `pkg/config/ttl_handler.go` and
  `history_limiter.go` correctly handles zero-value, negative, and missing fields
- [ ] No data races — verify with `make test-unit` (race detector is on by default)

### Go Quality

- [ ] `make fmt` produces no diff
- [ ] `go vet ./...` is clean
- [ ] No use of `init()` for non-trivial side effects
- [ ] Errors are wrapped with `fmt.Errorf("...: %w", err)` — not swallowed
- [ ] Contexts are propagated, not ignored
- [ ] No direct use of `os.Exit` outside `main`

### Kubernetes / Operator Patterns

- [ ] RBAC changes in `config/200-clusterrole.yaml` / `config/200-role.yaml`
  are least-privilege — add only the verbs actually required
- [ ] New CRDs or config resources include corresponding YAML in `config/`
- [ ] Informers and listers are used for reads; direct API calls for writes
- [ ] Reconcile loops return `controller.NewPermanentError` only for truly
  unrecoverable conditions
- [ ] Webhook validation in `pkg/webhook/configmapvalidation.go` rejects invalid
  pruner specs early, with clear error messages

### Domain: Pruner Logic

- [ ] ConfigMap selector logic (`pkg/config/config.go`, `pkg/config/helper.go`) correctly
  resolves namespace-level overrides vs cluster-level defaults
- [ ] `pkg/config/constants.go` is the single source of truth for annotation
  and label keys — no hardcoded strings elsewhere
- [ ] History and TTL limits are validated against each other where both are set
- [ ] The pruner reconciler correctly handles `PipelineRun` and `TaskRun`
  independently; shared logic belongs in `pkg/config/helper.go`

### Observability

- [ ] New controller actions emit metrics via `pkg/metrics/`
- [ ] Log statements use structured logging (`zap`) with appropriate levels
  (`Info` for normal operations, `Error` for failures, `Debug` for verbose paths)

### Documentation

- [ ] Public functions and types have Go doc comments
- [ ] User-facing ConfigMap fields are documented in `docs/` (especially
  `docs/tutorials/` and `docs/configmap-validation.md`)
- [ ] `ARCHITECTURE.md` is updated if a new component or major design change
  is introduced

## What to Approve

Approve when:
- All checklist items pass
- Tests cover the changed behavior
- The change is focused (one concern per PR)
- Commit messages follow [Tekton commit conventions](https://github.com/tektoncd/community/blob/main/standards.md#commit-messages)

## What to Block

Block (request changes) when:
- Logic is untested or tests are trivially green
- RBAC grants broad wildcard verbs
- Errors are silently swallowed in reconcile loops
- ConfigMap validation can be bypassed
