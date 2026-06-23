# Commit Types Reference

Complete reference for conventional commit types used in the
tekton-pruner project.

## Scope and Issue References

- Prefer **component names** as scope (e.g. `config`, `controller`,
  `webhook`).
- To link work to a GitHub issue, add `Fixes #NNN` or `Closes #NNN` in
  the **commit body** (not the subject scope). Merging the PR then
  closes the issue automatically.

## Standard Types

### feat — New Features

New functionality added to the codebase.

- `feat(config): add enforcedConfigLevel validation`
- `feat(controller): add periodic cleanup for orphaned annotations`
- `feat(webhook): reject ConfigMaps with unknown fields`
- `feat(metrics): add pruner_config_reloads_total counter`

### fix — Bug Fixes

Bug fixes that resolve incorrect behavior.

- `fix(controller): prevent duplicate TTL annotations on PipelineRuns`
- `fix(config): handle zero-value TTL in namespace override`
- `fix(webhook): return clear error for negative history limits`
- `fix(cve): GO-2026-XXXX — update golang.org/x/net`

**CVE / security fixes**: Use scope `cve` and cite the advisory.
Routine dependency bumps from Dependabot use `chore(deps):`, not `fix`.

### docs — Documentation

Documentation-only changes.

- `docs(tutorials): add resource-group selector examples`
- `docs(README): update installation steps`
- `docs(ARCHITECTURE): document namespace config controller`
- `docs(configmap-validation): add troubleshooting section`

### refactor — Code Refactoring

Code restructuring that does not change behavior.

- `refactor(config): extract selector matching into helper function`
- `refactor(controller): simplify reconcile loop error handling`
- `refactor(webhook): consolidate validation logic`

**Must not** change observable behavior. If behavior changes, use `fix`
(bug) or `feat` (new capability) instead.

### test — Testing

Adding or updating tests.

- `test(config): add hierarchical validation edge cases`
- `test(controller): improve TTL handler coverage`
- `test(webhook): add admission rejection tests`

### chore — Maintenance Tasks

Routine maintenance and dependency updates.

- `chore(deps): bump github.com/tektoncd/pipeline from 1.12.0 to 1.13.0`
- `chore(vendor): run hack/update-deps.sh`
- `chore(Makefile): update golangci-lint version`

### build — Build System

Changes to how the project is built.

- `build(Makefile): add yamllint target`
- `build(ko): configure multi-arch image builds`
- `build(go.mod): bump Go version to 1.26`

### ci — CI/CD Changes

Changes to continuous integration and release automation.

- `ci(.github/workflows): add golangci-lint to CI`
- `ci(.github/workflows): update e2e test matrix`
- `ci(.tekton): update release pipeline`

### perf — Performance Improvements

Changes that improve performance.

- `perf(controller): batch TTL annotation updates`
- `perf(config): cache parsed ConfigMap specs`

### style — Code Style

Formatting and style changes with no behavior change.

- `style(format): run go fmt`
- `style(lint): fix golangci-lint warnings`

### revert — Revert Previous Commit

Reverting a previous commit.

- `revert: undo breaking API change`
- `revert(config): revert enforcedConfigLevel default`

Include reference to the original commit in the body.

## Breaking Changes

Add `!` after type/scope to indicate a breaking change:

- `feat(config)!: rename historyLimit to retentionLimit`
- `fix(webhook)!: reject previously accepted invalid specs`

Body should include:

```
BREAKING CHANGE: <description and migration path>
```

## Type Selection Guide

1. **Does it add new functionality?** → `feat`
2. **Does it fix a bug?** → `fix`
3. **Is it documentation only?** → `docs`
4. **Does it change code structure without behavior change?** → `refactor`
5. **Is it test-related?** → `test`
6. **Is it dependency/maintenance?** → `chore`
7. **Is it build system related?** → `build`
8. **Is it CI/CD related?** → `ci`
9. **Does it improve performance?** → `perf`
10. **Is it formatting/style only?** → `style`
11. **Does it revert a previous commit?** → `revert`

## Multiple Changes in One Commit

Choose the most significant type. Mention secondary changes in the body.

- Adding feature + tests → `feat(webhook): add handler` (body: "Includes
  integration tests")
- Bug fix + refactoring → `fix(controller): resolve race condition`
  (body: "Refactored reconciliation logic for clarity")

If changes are too diverse, split into multiple commits.
