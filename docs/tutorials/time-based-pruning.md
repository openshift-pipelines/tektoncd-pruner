<!--
---
linkTitle: "Tutorial: Time-based Pruning"
weight: 300
---
-->

# Time-based Pruning Tutorial

This tutorial demonstrates how to configure time-based pruning (TTL - Time To Live) in Tekton Pruner to automatically delete PipelineRuns and TaskRuns after they've been completed for a specified duration.

## Understanding TTL-based Pruning

The `ttlSecondsAfterFinished` setting determines how long a completed PipelineRun or TaskRun should be kept before being deleted. This applies to both successful and failed runs.

## Basic TTL Configuration

Here's a basic configuration that deletes resources 1 hour after completion:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    ttlSecondsAfterFinished: 3600    # Delete after 1 hour
```

## Common TTL Configurations

### Short-lived Development Resources

```yaml
data:
  global-config: |
    namespaces:
      development:
        ttlSecondsAfterFinished: 300    # 5 minutes
```

### Production Audit Requirements

```yaml
data:
  global-config: |
    namespaces:
      production:
        ttlSecondsAfterFinished: 2592000    # 30 days
```

## Pipeline-specific TTL

You can set different TTL values for specific pipelines:

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            tekton.dev/pipeline: release-pipeline
        ttlSecondsAfterFinished: 604800    # Keep release runs for 1 week
```

## Combining TTL with History Limits

TTL and history limits can be used together:

```yaml
data:
  global-config: |
    ttlSecondsAfterFinished: 3600          # Delete after 1 hour
    successfulHistoryLimit: 5               # But always keep last 5 successful runs
    failedHistoryLimit: 3                   # And last 3 failed runs
```

## Best Practices

1. Set shorter TTLs in development environments
2. Use longer TTLs in production for audit purposes
3. Consider regulatory requirements when setting TTLs
4. Balance storage costs with retention needs
5. Use labels to identify critical pipelines that need longer retention

## Common TTL Values

| Duration | Seconds | Use Case |
|----------|---------|----------|
| 5 minutes | 300 | Development/testing |
| 1 hour | 3600 | CI pipelines |
| 1 day | 86400 | General workloads |
| 1 week | 604800 | Release pipelines |
| 30 days | 2592000 | Production/audit |

## Verifying TTL-based Pruning

1. Check the age of your PipelineRuns:
```bash
kubectl get pipelineruns --sort-by=.status.completionTime
```

2. Monitor pruning activities:
```bash
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller
```

## Example Scenarios

### CI/CD Pipeline Configuration

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            purpose: ci
        ttlSecondsAfterFinished: 3600    # CI runs cleaned up after 1 hour
      - selector:
          matchLabels:
            purpose: cd
        ttlSecondsAfterFinished: 604800   # CD runs kept for 1 week
```

### Environment-based Configuration

```yaml
data:
  global-config: |
    namespaces:
      development:
        ttlSecondsAfterFinished: 300      # 5 minutes
      staging:
        ttlSecondsAfterFinished: 86400    # 1 day
      production:
        ttlSecondsAfterFinished: 2592000  # 30 days
```

## Next Steps

- Explore [Namespace Configuration](./namespace-configuration.md)
- Learn about [Resource Groups](./resource-groups.md)
- Review [History-based Pruning](./history-based-pruning.md)