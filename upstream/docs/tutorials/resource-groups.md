<!--
---
linkTitle: "Tutorial: Resource Groups"
weight: 500
---
-->

# Resource Groups

Apply different pruning policies to different sets of PipelineRuns/TaskRuns using selectors.

**IMPORTANT:** Selectors only work in **namespace-level ConfigMaps** (`tekton-pruner-namespace-spec`). Selectors in global ConfigMaps are ignored by the pruner.

## How It Works

- **Match by labels or annotations** on PipelineRuns/TaskRuns
- **First match wins**: Groups evaluated in order
- **Fallback**: Unmatched resources use namespace or global defaults
- **Location**: Must be in namespace ConfigMap, not global ConfigMap

## Selector Types

**Label selectors:**
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
    pipelineRuns:
      - selector:
        - matchLabels:
            environment: production
            tier: frontend
        ttlSecondsAfterFinished: 604800
        successfulHistoryLimit: 10
```

**Annotation selectors:**
```yaml
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchAnnotations:
            tekton.dev/release: "true"
        ttlSecondsAfterFinished: 2592000
```

**Mixed selectors** (both labels and annotations must match):
```yaml
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchLabels:
            app: myapp
          matchAnnotations:
            critical: "true"
        successfulHistoryLimit: 50
```

## Common Patterns

**By Pipeline Type:**
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
            pipeline-type: build
        ttlSecondsAfterFinished: 300
      - selector:
        - matchLabels:
            pipeline-type: test
        ttlSecondsAfterFinished: 3600
      - selector:
        - matchLabels:
            pipeline-type: release
        ttlSecondsAfterFinished: 604800
        successfulHistoryLimit: 20
```

**By Environment:**
```yaml
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchLabels:
            env: dev
        ttlSecondsAfterFinished: 300
      - selector:
        - matchLabels:
            env: staging
        ttlSecondsAfterFinished: 86400
      - selector:
        - matchLabels:
            env: prod
        ttlSecondsAfterFinished: 604800
```

**By Criticality:**
```yaml
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchLabels:
            critical: "true"
        ttlSecondsAfterFinished: 2592000
        successfulHistoryLimit: 50
      - selector:
        - matchLabels:
            critical: "false"
        ttlSecondsAfterFinished: 3600
        successfulHistoryLimit: 3
```

## Order Matters

**First match wins** - order selectors from most to least specific:

```yaml
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchLabels:
            env: prod
            critical: "true"
        ttlSecondsAfterFinished: 2592000
      - selector:
        - matchLabels:
            env: prod
        ttlSecondsAfterFinished: 604800
      - selector:
        - matchLabels:
            app: myapp
        ttlSecondsAfterFinished: 3600
```

## Best Practices

1. **Use namespace ConfigMaps** for selector-based groups
2. **Order selectors** from most to least specific (first match wins)
3. **Use consistent labels**: `app`, `component`, `env`, `tier`
4. **Document groups** with comments above selectors
5. **Test** with sample runs before production

## Advanced Configurations

### Multi-tier Application

```yaml
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchLabels:
            tier: frontend
        ttlSecondsAfterFinished: 604800
        successfulHistoryLimit: 10
      - selector:
        - matchLabels:
            tier: backend
        ttlSecondsAfterFinished: 1209600
        successfulHistoryLimit: 15
      - selector:
        - matchLabels:
            tier: database
        ttlSecondsAfterFinished: 2592000
        successfulHistoryLimit: 30
```

### Release Types

```yaml
data:
  ns-config: |
    pipelineRuns:
      - selector:
        - matchLabels:
            release-type: feature
        ttlSecondsAfterFinished: 604800
      - selector:
        - matchLabels:
            release-type: hotfix
        ttlSecondsAfterFinished: 2592000
      - selector:
        - matchLabels:
            release-type: major
        ttlSecondsAfterFinished: 7776000
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

## Related

- [Namespace Configuration](./namespace-configuration.md) - Set up namespace ConfigMaps
- [Time-based Pruning](./time-based-pruning.md) - TTL strategies for groups
- [History-based Pruning](./history-based-pruning.md) - Retention strategies for groups