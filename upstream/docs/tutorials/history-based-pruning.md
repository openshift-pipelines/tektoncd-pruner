<!--
---
linkTitle: "Tutorial: History-based Pruning"
weight: 200
---
-->

# History-based Pruning Tutorial

This tutorial demonstrates how to configure history-based pruning in Tekton Pruner to maintain a specific number of PipelineRuns and TaskRuns based on their completion status.

## Understanding History Limits

Tekton Pruner supports three types of history limits:

1. `successfulHistoryLimit`: Number of successful runs to retain
2. `failedHistoryLimit`: Number of failed runs to retain
3. `historyLimit`: When individual limits are not set, this value is used as the limit for both successful and failed runs individually

## Basic History-based Configuration

Here's a basic configuration that keeps the last 5 successful runs and last 3 failed runs:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    successfulHistoryLimit: 5    # Keep last 5 successful runs
    failedHistoryLimit: 3        # Keep last 3 failed runs
```

## Using the Combined History Limit

When you want to keep the same number of runs regardless of status:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    historyLimit: 5    # Keep last 5 successful and last 5 failed runs individually
```

## Pipeline-specific History Limits

You can set history limits for specific pipelines using labels:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            tekton.dev/pipeline: my-important-pipeline
        successfulHistoryLimit: 10    # Keep more history for important pipelines
        failedHistoryLimit: 5
```

## Best Practices

1. Start with larger history limits and adjust based on your needs
2. Consider storage implications when setting limits
3. Use different limits for different environments (dev/staging/prod)
4. Set higher limits for critical pipelines
5. Monitor storage usage after implementing history-based pruning

## Examples

### Development Environment

```yaml
data:
  global-config: |
    namespaces:
      development:
        successfulHistoryLimit: 3
        failedHistoryLimit: 5        # Keep more failed runs for debugging
```

### Production Environment

```yaml
data:
  global-config: |
    namespaces:
      production:
        successfulHistoryLimit: 10    # Keep more history in production
        failedHistoryLimit: 10
```

### CI/CD Pipeline

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            type: ci-cd
        successfulHistoryLimit: 20    # Keep extensive history for CI/CD
        failedHistoryLimit: 10
```

## Verifying History-based Pruning

1. Check retained PipelineRuns:
```bash
kubectl get pipelineruns --sort-by=.metadata.creationTimestamp
```

2. Monitor pruning activities in controller logs:
```bash
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller
```

## Next Steps

- Learn about [Time-based Pruning](./time-based-pruning.md)
- Explore [Namespace Configuration](./namespace-configuration.md)
- Set up [Resource Groups](./resource-groups.md)