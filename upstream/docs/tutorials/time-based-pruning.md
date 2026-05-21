<!--
---
linkTitle: "Tutorial: Time-based Pruning"
weight: 300
---
-->

# Time-based Pruning (TTL)

Delete completed resources after a specified duration using `ttlSecondsAfterFinished`.

## How It Works

TTL applies to **all completed runs** (successful and failed). The timer starts when the run finishes.

## Basic Configuration

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
    ttlSecondsAfterFinished: 3600  # Delete after 1 hour
```

## Common TTL Values

| Duration | Seconds | Use Case |
|----------|---------|----------|
| 5 minutes | `300` | Dev/test rapid iteration |
| 1 hour | `3600` | CI pipelines |
| 1 day | `86400` | General workloads |
| 7 days | `604800` | Staging/release |
| 30 days | `2592000` | Production/audit/compliance |

## Environment-specific TTLs

```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 3600  # Default
    namespaces:
      dev:
        ttlSecondsAfterFinished: 300      # 5 min
      staging:
        ttlSecondsAfterFinished: 86400    # 1 day
      prod:
        ttlSecondsAfterFinished: 2592000
```

## Pipeline-specific TTLs

Use selectors in namespace ConfigMaps for pipeline-specific TTLs:

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
    ttlSecondsAfterFinished: 3600
    pipelineRuns:
      - selector:
        - matchLabels:
            pipeline-type: release
        ttlSecondsAfterFinished: 604800
      - selector:
        - matchLabels:
            pipeline-type: test
        ttlSecondsAfterFinished: 300
```

## Combining TTL with History Limits

> **Important**: Setting a history limit does NOT prevent TTL from deleting runs.

If you set both TTL and history limit, they run separately. TTL will still delete runs after the time passes, even if you're under the history limit.

```yaml
data:
  ns-config: |
    ttlSecondsAfterFinished: 300
    successfulHistoryLimit: 5
    failedHistoryLimit: 10
```

**Example**: With the config above, if you only have 3 successful runs and 2 failed runs, and they're all older than 5 minutes, all 5 will be deleted by TTL.

If you want to delete runs purely based on time, **don't set history limits** - just use TTL alone.

## Verification

```bash
# Check run completion times
kubectl get pipelineruns --sort-by=.status.completionTime

# Monitor pruning
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller | grep "Deleting"
```

## Best Practices

1. **Development**: Short TTLs (5-60 min) for rapid iteration
2. **Production**: Long TTLs (7-30 days) for audit/compliance
3. **Critical Pipelines**: Use selectors for extended retention
4. **Balance**: Consider storage costs vs. retention needs

## Related

- [History-based Pruning](./history-based-pruning.md) - Retain N runs regardless of age
- [Namespace Configuration](./namespace-configuration.md) - Per-environment TTL settings
- [Resource Groups](./resource-groups.md) - Pipeline-specific TTLs via selectors