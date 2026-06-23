---
name: pruner-config
description: >-
  Understand, author, validate, or debug tekton-pruner ConfigMap configuration.
  Use when working with TTL-based or history-based pruning rules, namespace
  overrides, resource-group selectors, or per-resource annotations. Covers the
  full spec format, validation rules, and hierarchical override semantics.
license: Apache-2.0
metadata:
  project: tekton-pruner
allowed-tools: Read Bash(kubectl get:*) Bash(kubectl apply:*) Bash(kubectl describe:*)
---

# Pruner Configuration

tekton-pruner is configured via a `ConfigMap` named `tekton-pruner-default-spec`
in the `tekton-pipelines` namespace, plus optional per-namespace annotations.

## Configuration Hierarchy

Settings are resolved in this order (most specific wins):

```
per-resource annotation
  └─ namespace-level ConfigMap entry
       └─ cluster-level default (tekton-pruner-default-spec)
```

See `docs/tutorials/namespace-configuration.md` and
`docs/tutorials/history-based-pruning.md` for worked examples.

## ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    enforcedConfigLevel: global
    ttlSecondsAfterFinished: 3600     # delete 1h after completion
    successfulHistoryLimit: 3
    failedHistoryLimit:     3
    historyLimit:           100
    namespaces:
      my-namespace:
        ttlSecondsAfterFinished: 600
        successfulHistoryLimit: 5
```

> **Note:** The global ConfigMap only reads the `global-config` data key.
> Namespace overrides are nested under `namespaces:` within that key.
> For selector-based rules, use the separate namespace ConfigMap
> (`tekton-pruner-namespace-spec`) instead — see Resource-Group Selectors below.

## Key Fields

| Field | Type | Description |
|-------|------|-------------|
| `enforcedConfigLevel` | string | Config level enforcement (`global`, `namespace`, or `resource`) |
| `ttlSecondsAfterFinished` | int | Seconds after completion before deletion (0 = disabled) |
| `successfulHistoryLimit` | int | Max successful runs (PipelineRun or TaskRun) to keep |
| `failedHistoryLimit` | int | Max failed runs (PipelineRun or TaskRun) to keep |
| `historyLimit` | int | Max runs to keep regardless of status (fallback when granular limits are not set) |

## Validation

The admission webhook (`pkg/webhook/configmapvalidation.go`) validates all
ConfigMap entries on create/update. Common validation errors:

- Negative values for any limit field
- TTL and history limit both set to `0` (would delete everything immediately)
- Unknown keys in the spec YAML

Validate locally by reviewing `pkg/config/config_validation_test.go`.

## Apply and Verify

```bash
# Apply config
kubectl apply -f config/600-tekton-pruner-default-spec.yaml

# Check the webhook accepted it
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines -o yaml

# Watch controller logs for pruning activity
kubectl -n tekton-pipelines logs deployment/tekton-pruner-controller -f | grep -E "pruned|TTL|history"
```

## Resource-Group Selectors

Advanced scoping by label selector — see `docs/tutorials/resource-groups.md`.

**Important:** Selectors only work in **namespace-level ConfigMaps**
(`tekton-pruner-namespace-spec`), not in the global ConfigMap. Selectors in
global ConfigMaps are silently ignored.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec
  namespace: my-namespace
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: namespace
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchLabels:
            pipeline: nightly-build
        successfulHistoryLimit: 1
```

## Per-Resource Annotations

Override config at the individual resource level:

```yaml
annotations:
  pruner.tekton.dev/ttlSecondsAfterFinished: "300"
  pruner.tekton.dev/successfulHistoryLimit: "2"
  pruner.tekton.dev/failedHistoryLimit: "1"
```

Annotation keys are defined in `pkg/config/constants.go`. Never hardcode
annotation strings elsewhere.

## References

- `docs/tutorials/getting-started.md` — first-time setup
- `docs/tutorials/time-based-pruning.md` — TTL examples
- `docs/tutorials/history-based-pruning.md` — history limit examples
- `docs/configmap-validation.md` — validation rules reference
- `config/600-tekton-pruner-default-spec.yaml` — default ConfigMap shipped with the release
