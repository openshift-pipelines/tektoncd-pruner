<!--
---
linkTitle: "Tutorial: Resource Groups"
weight: 500
---
-->

# Resource Groups

Apply different pruning policies to different sets of PipelineRuns/TaskRuns using selectors.

## How It Works

- **Match by labels or annotations** on PipelineRuns/TaskRuns
- **First match wins**: Groups evaluated in order
- **Fallback**: Unmatched resources use namespace or global defaults

## Selector Types

**Label selectors:**
```yaml
pipelineRuns:
  - selector:
      matchLabels:
        environment: production
        tier: frontend
    ttlSecondsAfterFinished: 604800
    successfulHistoryLimit: 10
```

**Annotation selectors:**
```yaml
pipelineRuns:
  - selector:
      matchAnnotations:
        tekton.dev/release: "true"
    ttlSecondsAfterFinished: 2592000  # 30 days
```

**Mixed selectors:**
```yaml
pipelineRuns:
  - selector:
      matchLabels:
        app: myapp
      matchAnnotations:
        critical: "true"
    successfulHistoryLimit: 50
```

## Common Patterns

**By Pipeline Type:**
```yaml
data:
  global-config: |
    ttlSecondsAfterFinished: 3600  # Default
    pipelineRuns:
      - selector:
          matchLabels:
            pipeline-type: build
        ttlSecondsAfterFinished: 300      # 5 min
      - selector:
          matchLabels:
            pipeline-type: test
        ttlSecondsAfterFinished: 3600     # 1 hour
      - selector:
          matchLabels:
            pipeline-type: release
        ttlSecondsAfterFinished: 604800   # 7 days
        successfulHistoryLimit: 20
```

**By Environment:**
```yaml
pipelineRuns:
  - selector:
      matchLabels:
        env: dev
    ttlSecondsAfterFinished: 300
  - selector:
      matchLabels:
        env: staging
    ttlSecondsAfterFinished: 86400
  - selector:
      matchLabels:
        env: prod
    ttlSecondsAfterFinished: 604800
```

**By Criticality:**
```yaml
pipelineRuns:
  - selector:
      matchLabels:
        critical: "true"
    ttlSecondsAfterFinished: 2592000  # 30 days
    successfulHistoryLimit: 50
  - selector:
      matchLabels:
        critical: "false"
    ttlSecondsAfterFinished: 3600     # 1 hour
    successfulHistoryLimit: 3
```

## Order Matters

**First match wins** - order selectors from most to least specific:

```yaml
pipelineRuns:
  - selector:  # Match first: prod + critical
      matchLabels:
        env: prod
        critical: "true"
    ttlSecondsAfterFinished: 2592000
  - selector:  # Match second: just prod
      matchLabels:
        env: prod
    ttlSecondsAfterFinished: 604800
  - selector:  # Match last: everything else
         matchLabels:
           app: myapp
   ```

3. Document Group Purpose
   ```yaml
   pipelineRuns:
     # Critical security scanning pipelines
     - selector:
         matchLabels:
           pipeline-type: security
   ```

## Advanced Configurations

### Multi-tier Application

```yaml
data:
  global-config: |
    pipelineRuns:
      # Frontend components
      - selector:
          matchLabels:
            tier: frontend
        ttlSecondsAfterFinished: 604800
        successfulHistoryLimit: 10
        
      - selector:  # Match last: everything else (no labels)
    ttlSecondsAfterFinished: 3600
```

## Combining with Namespaces

Resource groups work within namespace configs:

```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    namespaces:
      prod:
        ttlSecondsAfterFinished: 604800  # Namespace default
        pipelineRuns:
          - selector:
              matchLabels:
                critical: "true"
            ttlSecondsAfterFinished: 2592000  # Override for critical
```

## Labeling Your Pipelines

Add labels to PipelineRuns for grouping:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  generateName: my-pipeline-
  labels:
    pipeline-type: release
    env: prod
    critical: "true"
spec:
  pipelineRef:
    name: my-pipeline
```

## Verification

```bash
# Check labels on runs
kubectl get pr --show-labels

# Monitor which group matched
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller | grep "selector"
```

## Best Practices

1. **Consistent labeling scheme** across all pipelines
2. **Order selectors** from most to least specific
3. **Use standard labels**: `app`, `component`, `env`, `tier`
4. **Document** your grouping strategy
5. **Test** with sample runs before production

## Related

- [Namespace Configuration](./namespace-configuration.md) - Combine groups with namespace configs
- [Time-based Pruning](./time-based-pruning.md) - TTL strategies for groups
- [History-based Pruning](./history-based-pruning.md) - Retention strategies for groups
```

### Release Types

```yaml
data:
  global-config: |
    pipelineRuns:
      # Feature releases
      - selector:
          matchLabels:
            release-type: feature
        ttlSecondsAfterFinished: 604800     # 1 week
        
      # Hotfix releases
      - selector:
          matchLabels:
            release-type: hotfix
        ttlSecondsAfterFinished: 2592000    # 30 days
        
      # Major releases
      - selector:
          matchLabels:
            release-type: major
        ttlSecondsAfterFinished: 7776000    # 90 days
```

## Testing Resource Groups

1. Apply labels to your PipelineRuns:
```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  generateName: test-pipeline-
  labels:
    tier: frontend
    environment: production
```

2. Verify group matching:
```bash
kubectl get pipelineruns --show-labels
```

3. Monitor pruning behavior:
```bash
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller
```

## Next Steps

- Review [History-based Pruning](./history-based-pruning.md)
- Explore [Time-based Pruning](./time-based-pruning.md)
- Learn about [Namespace Configuration](./namespace-configuration.md)