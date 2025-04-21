<!--
---
linkTitle: "Tutorial: Namespace Configuration"
weight: 400
---
-->

# Namespace Configuration Tutorial

This tutorial demonstrates how to configure Tekton Pruner with different settings for different namespaces, allowing you to have fine-grained control over pruning behavior across your cluster.

## Understanding Namespace Configuration

Tekton Pruner supports a hierarchical configuration model where settings can be:
- Global (cluster-wide defaults)
- Namespace-specific
- Resource group-specific within namespaces

## Basic Namespace Configuration

Here's a basic configuration with different settings for different namespaces:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    enforcedConfigLevel: namespace    # Enable namespace-level configuration
    ttlSecondsAfterFinished: 3600    # Global default: 1 hour
    namespaces:
      development:
        ttlSecondsAfterFinished: 300           # 5 minutes
        successfulHistoryLimit: 3
        failedHistoryLimit: 5
      staging:
        ttlSecondsAfterFinished: 86400         # 1 day
        successfulHistoryLimit: 5
        failedHistoryLimit: 5
      production:
        ttlSecondsAfterFinished: 604800        # 1 week
        successfulHistoryLimit: 10
        failedHistoryLimit: 10
```

## Configuration Inheritance

Settings flow from global → namespace → resource group. Any setting not specified at a lower level inherits from the level above:

```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 3600    # Global default
    successfulHistoryLimit: 5         # Global default
    namespaces:
      development:
        ttlSecondsAfterFinished: 300  # Override TTL only
        # inherits successfulHistoryLimit from global
```

## Namespace Groups

You can group namespaces with similar requirements:

```yaml
data:
  global-config: |
    namespaces:
      dev-*:    # Applies to dev-team1, dev-team2, etc.
        ttlSecondsAfterFinished: 300
        successfulHistoryLimit: 3
      prod-*:   # Applies to prod-team1, prod-team2, etc.
        ttlSecondsAfterFinished: 604800
        successfulHistoryLimit: 10
```

## Combining with Resource Groups

Within each namespace, you can further configure specific resource groups:

```yaml
data:
  global-config: |
    namespaces:
      production:
        ttlSecondsAfterFinished: 604800    # Namespace default: 1 week
        pipelineRuns:
          - selector:
              matchLabels:
                importance: critical
            ttlSecondsAfterFinished: 2592000    # Critical pipelines: 30 days
```

## Best Practices

1. Use clear namespace naming conventions
2. Start with permissive limits in development
3. Implement stricter retention in production
4. Document namespace configuration decisions
5. Regularly review and adjust settings

## Common Patterns

### Development Workflow

```yaml
data:
  global-config: |
    namespaces:
      development:
        ttlSecondsAfterFinished: 300       # Quick cleanup
        successfulHistoryLimit: 3           # Minimal history
      testing:
        ttlSecondsAfterFinished: 3600      # Keep for analysis
        successfulHistoryLimit: 5           # More history for testing
      staging:
        ttlSecondsAfterFinished: 86400     # 24-hour retention
        successfulHistoryLimit: 10          # Substantial history
```

### Team-based Configuration

```yaml
data:
  global-config: |
    namespaces:
      team1-dev:
        ttlSecondsAfterFinished: 300
      team1-prod:
        ttlSecondsAfterFinished: 604800
      team2-dev:
        ttlSecondsAfterFinished: 300
      team2-prod:
        ttlSecondsAfterFinished: 604800
```

## Verifying Namespace Configuration

1. Check configuration for a specific namespace:
```bash
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines -o jsonpath='{.data.global-config}' | grep -A 5 "namespaces:"
```

2. Monitor pruning behavior:
```bash
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller | grep "namespace: your-namespace"
```

## Next Steps

- Learn about [Resource Groups](./resource-groups.md)
- Review [History-based Pruning](./history-based-pruning.md)
- Explore [Time-based Pruning](./time-based-pruning.md)