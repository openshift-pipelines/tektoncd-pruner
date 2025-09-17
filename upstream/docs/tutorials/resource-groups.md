<!--
---
linkTitle: "Tutorial: Resource Groups"
weight: 500
---
-->

# Resource Groups Tutorial

This tutorial demonstrates how to use resource groups in Tekton Pruner to apply different pruning policies to different sets of PipelineRuns and TaskRuns based on their labels, annotations, or names.

## Understanding Resource Groups

Resource groups allow you to:
- Group resources based on labels, annotations, or names
- Apply different pruning policies to different groups
- Create fine-grained control over resource lifecycle

## Basic Resource Group Configuration

Here's a basic configuration that groups resources by their labels:

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
            environment: production
            tier: frontend
        ttlSecondsAfterFinished: 604800    # 1 week
        successfulHistoryLimit: 10
      - selector:
          matchLabels:
            environment: production
            tier: backend
        ttlSecondsAfterFinished: 1209600   # 2 weeks
        successfulHistoryLimit: 15
```

## Selector Types

### Label Selectors

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            app: myapp
            component: api
```

### Annotation Selectors

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchAnnotations:
            tekton.dev/release: "true"
```

### Mixed Selectors

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            app: myapp
          matchAnnotations:
            importance: high
```

## Common Use Cases

### CI/CD Pipeline Groups

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            pipeline-type: build
        ttlSecondsAfterFinished: 3600       # 1 hour
        successfulHistoryLimit: 5
      - selector:
          matchLabels:
            pipeline-type: test
        ttlSecondsAfterFinished: 86400      # 1 day
        successfulHistoryLimit: 10
      - selector:
          matchLabels:
            pipeline-type: deploy
        ttlSecondsAfterFinished: 604800     # 1 week
        successfulHistoryLimit: 20
```

### Environment-based Groups

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            env: dev
        ttlSecondsAfterFinished: 300        # 5 minutes
      - selector:
          matchLabels:
            env: staging
        ttlSecondsAfterFinished: 86400      # 1 day
      - selector:
          matchLabels:
            env: prod
        ttlSecondsAfterFinished: 604800     # 1 week
```

## Best Practices

1. Use Consistent Labels
   ```yaml
   # Good - consistent label scheme
   matchLabels:
     app: myapp
     component: frontend
     tier: web
   ```

2. Prioritize Groups Correctly
   ```yaml
   pipelineRuns:
     - selector:  # More specific matcher first
         matchLabels:
           app: myapp
           critical: "true"
     - selector:  # General matcher later
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
        
      # Backend API services
      - selector:
          matchLabels:
            tier: backend
        ttlSecondsAfterFinished: 1209600
        successfulHistoryLimit: 15
        
      # Database operations
      - selector:
          matchLabels:
            tier: database
        ttlSecondsAfterFinished: 2592000    # 30 days
        successfulHistoryLimit: 20
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