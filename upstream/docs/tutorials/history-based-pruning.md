<!--
---
linkTitle: "Tutorial: History-based Pruning"
weight: 200
---
-->

# History-based Pruning

Retain a fixed number of runs based on their status, regardless of age.

## How It Works

History limits **override TTL** to guarantee minimum retention. Always keeps the N most recent runs.

## Configuration Options

| Setting | Description |
|---------|-------------|
| `successfulHistoryLimit` | Keep N most recent successful runs |
| `failedHistoryLimit` | Keep N most recent failed runs |
| `historyLimit` | Keep N runs of EACH status (when specific limits not set) |

## Basic Configuration

**Separate limits by status:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: global
data:
  global-config: |
    successfulHistoryLimit: 5    # Keep last 5 successful
    failedHistoryLimit: 10       # Keep last 10 failed (for debugging)
```

**Same limit for both:**
```yaml
data:
  global-config: |
    historyLimit: 5  # Keep last 5 successful AND last 5 failed
```

## Environment-specific Limits

```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    namespaces:
      dev:
        successfulHistoryLimit: 3
        failedHistoryLimit: 5     # More failed runs for debugging
      staging:
        successfulHistoryLimit: 5
        failedHistoryLimit: 5
      prod:
        successfulHistoryLimit: 10
        failedHistoryLimit: 20
```

## Pipeline-specific Limits

Use selectors in namespace ConfigMaps for pipeline-specific limits:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec
  namespace: my-app
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: namespace
data:
  ns-config: |
    successfulHistoryLimit: 3
    pipelineRuns:
      - selector:
          matchLabels:
            critical: "true"
        successfulHistoryLimit: 20
        failedHistoryLimit: 30
      - selector:
          matchLabels:
            pipeline-type: test
        successfulHistoryLimit: 3
        failedHistoryLimit: 5
```

## Interaction with TTL

History limits take priority over TTL:

```yaml
data:
  ns-config: |
    ttlSecondsAfterFinished: 300
    successfulHistoryLimit: 5
    failedHistoryLimit: 10
```

**Result**: The 5 most recent successful and 10 most recent failed runs are kept indefinitely, regardless of age.

## Verification

```bash
# Check retained runs by status
kubectl get pr -l tekton.dev/pipeline=<name> --field-selector status.conditions[0].status=True
kubectl get pr -l tekton.dev/pipeline=<name> --field-selector status.conditions[0].status=False# Monitor pruning
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller | grep "history"
```

## Best Practices

1. **Keep more failed runs** than successful for debugging
2. **Critical pipelines**: Higher limits for audit trails
3. **Development**: Lower limits (3-5) for rapid iteration
4. **Production**: Higher limits (10-20) for analysis
5. **Monitor storage**: Adjust limits based on cluster capacity

## Related

- [Time-based Pruning](./time-based-pruning.md) - Age-based deletion
- [Namespace Configuration](./namespace-configuration.md) - Per-environment limits
- [Resource Groups](./resource-groups.md) - Pipeline-specific limits