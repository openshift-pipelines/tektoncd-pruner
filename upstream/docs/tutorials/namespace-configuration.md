<!--
---
linkTitle: "Tutorial: Namespace Configuration"
weight: 400
---
-->

# Namespace Configuration

Configure different pruning policies per namespace for environment-specific retention needs.

## Configuration Hierarchy

Settings flow: **Global** → **Namespace** → **Resource Group**

Set `enforcedConfigLevel: namespace` in global config to enable namespace-level overrides.

## Validation Boundaries

> **CRITICAL - System Boundaries**: Create namespace-level ConfigMaps **ONLY** in user namespaces where PipelineRuns/TaskRuns run.
> 
> **FORBIDDEN namespaces** (validation will reject):
> - System: `kube-*`, `openshift-*`
> - Tekton controllers: `tekton-pipelines`, `tekton-*`
>
> **Required labels** for all configs:
> ```yaml
> labels:
>   app.kubernetes.io/part-of: tekton-pruner
>   pruner.tekton.dev/config-type: <global|namespace>
> ```

## Method 1: Inline Namespace Specs (Centralized)

Define all namespace configs in the global ConfigMap:

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
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 3600  # Global fallback
    namespaces:
      dev:
        ttlSecondsAfterFinished: 300
        successfulHistoryLimit: 3
      staging:
        ttlSecondsAfterFinished: 86400
        successfulHistoryLimit: 5
      prod:
        ttlSecondsAfterFinished: 604800
        successfulHistoryLimit: 10
```

## Method 2: Per-Namespace ConfigMaps (Self-Service)

**Recommended** for namespace isolation and team autonomy.

**Step 1:** Enable namespace-level config in global ConfigMap:

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
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 3600  # Fallback for namespaces without config
```

**Step 2:** Create namespace-specific ConfigMap (fixed name: `tekton-pruner-namespace-spec`):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec  # Fixed name
  namespace: my-app  # User namespace only
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: namespace
data:
  ns-config: |
    ttlSecondsAfterFinished: 300
    successfulHistoryLimit: 5
    failedHistoryLimit: 10
```

**Benefits:**
- Self-service for namespace owners
- Independent lifecycle management
- Takes priority over global config

## Validation Rules

Namespace configurations are validated against limits to prevent resource exhaustion.

### 1. Explicit Global Limits
When the global config defines a limit, namespace configs cannot exceed it.

```yaml
data:
  global-config: |
    ttlSecondsAfterFinished: 3600
    successfulHistoryLimit: 10
    namespaces:
      development:
        ttlSecondsAfterFinished: 7200   # Invalid: exceeds global limit
        successfulHistoryLimit: 5       # Valid: within global limit
```

### 2. System Default Limits
When no global limit is defined, the system enforces these maximums:

| Configuration Field | System Maximum |
|---------------------|----------------|
| `ttlSecondsAfterFinished` | 2,592,000 seconds (30 days) |
| `successfulHistoryLimit` | 100 |
| `failedHistoryLimit` | 100 |
| `historyLimit` | 100 |

Example:

```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    # No limits defined - system maximums apply
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec
  namespace: my-app
data:
  ns-config: |
    ttlSecondsAfterFinished: 2592001   # Invalid: exceeds system maximum
    successfulHistoryLimit: 150        # Invalid: exceeds system maximum
```

### 3. Override Defaults
Cluster admins can set stricter limits via global config. Global limits override system defaults but cannot exceed system maximums.

```yaml
data:
  global-config: |
    ttlSecondsAfterFinished: 86400     # Admin limit: 1 day
    successfulHistoryLimit: 20         # Admin limit: 20 runs
    namespaces:
      development:
        ttlSecondsAfterFinished: 3600   # Valid: within global limit
        ttlSecondsAfterFinished: 172800 # Invalid: exceeds global limit
```

## Configuration Inheritance

Unspecified settings inherit from higher levels:

```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 3600     # Global default
    successfulHistoryLimit: 5          # Global default
    namespaces:
      dev:
        ttlSecondsAfterFinished: 300   # Override TTL
        # Inherits successfulHistoryLimit: 5 from global
```

## Configuration Inheritance

Unspecified settings inherit from higher levels:

```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 3600     # Global default
    successfulHistoryLimit: 5          # Global default
    namespaces:
      dev:
        ttlSecondsAfterFinished: 300   # Override TTL
        # Inherits successfulHistoryLimit: 5 from global
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

## Common Patterns

**Environment-based:**
```yaml
namespaces:
  dev:
    ttlSecondsAfterFinished: 300    # 5 min
    successfulHistoryLimit: 3
  staging:
    ttlSecondsAfterFinished: 86400  # 1 day
    successfulHistoryLimit: 5
  prod:
    ttlSecondsAfterFinished: 604800 # 7 days
    successfulHistoryLimit: 10
```

**With Resource Groups:**
```yaml
namespaces:
  prod:
    ttlSecondsAfterFinished: 604800  # Namespace default
    pipelineRuns:
      - selector:
          matchLabels:
            critical: "true"
        ttlSecondsAfterFinished: 2592000  # 30 days for critical
```

## Verification

**Check namespace config:**
```bash
# For inline method
kubectl get cm tekton-pruner-default-spec -n tekton-pipelines -o jsonpath='{.data.global-config}'

# For per-namespace method
kubectl get cm tekton-pruner-namespace-spec -n <namespace> -o yaml
```

**Monitor pruning:**
```bash
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller | grep "namespace:"
```

**Validate Configuration Against Limits:**

The webhook validates ConfigMaps at creation and update time:

```bash
kubectl apply -f namespace-config.yaml
# Error if limits exceeded:
# admission webhook denied: namespace-config: ttlSecondsAfterFinished (3000000) 
# cannot exceed system maximum (2592000 seconds / 30 days)
```

Check current global limits:
```bash
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines -o jsonpath='{.data.global-config}'
```

## Best Practices

1. Use clear namespace naming conventions
2. Start with permissive limits in development within system maximums
3. Implement stricter retention in production
4. Document namespace configuration decisions
5. Regularly review and adjust settings
6. Plan configurations within system limits (30 days / 100 runs)
7. Use global limits for cluster-wide governance
8. Test configurations before deployment

## Next Steps

- Learn about [Resource Groups](./resource-groups.md)
- Review [History-based Pruning](./history-based-pruning.md)
- Explore [Time-based Pruning](./time-based-pruning.md)