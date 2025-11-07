# ConfigMap Validation

Tekton Pruner uses a ValidatingWebhook to ensure ConfigMaps meet required specifications before they are created or updated in the cluster. This document explains how the validation works and what requirements must be met.

## Overview

The pruner webhook validates ConfigMaps to:
- Enforce required labels and naming conventions
- Prevent invalid configurations from being applied
- Protect against accidental deletion of critical configs
- Ensure namespace-level configs respect global boundaries

## How Validation Works

The validation webhook is automatically installed with pruner and intercepts ConfigMap CREATE, UPDATE, and DELETE operations. It only validates ConfigMaps with the label `app.kubernetes.io/part-of: tekton-pruner`, ensuring regular ConfigMaps in your cluster are unaffected.

## Validation Rules

### 1. Required Labels

**All pruner ConfigMaps MUST have these labels:**

```yaml
metadata:
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: <global|namespace>
```

**Validation checks:**
- Both labels must be present
- `app.kubernetes.io/part-of` must equal `tekton-pruner`
- `pruner.tekton.dev/config-type` must be either `global` or `namespace`

**Error example:**
```
Invalid pruner ConfigMap labels: ConfigMap must have label app.kubernetes.io/part-of=tekton-pruner
```

### 2. Selector Restrictions

**CRITICAL:** Selectors (matchLabels/matchAnnotations) are ONLY supported in namespace-level ConfigMaps (`tekton-pruner-namespace-spec`), NOT in global ConfigMaps.

**Valid - Selectors in Namespace ConfigMap:**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec
  namespace: dev
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: namespace
data:
  namespace-config: |
    pipelineRuns:
      - selector:                  # OK - This is a namespace ConfigMap
          - matchLabels:
              app: myapp
        ttlSecondsAfterFinished: 1800
```

**Invalid - Selectors in Global ConfigMap:**

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
    namespaces:
      dev:
        pipelineRuns:
          - selector:              # WILL FAIL VALIDATION
              - matchLabels:
                  app: myapp
        ttlSecondsAfterFinished: 1800
```

**Error example:**
```
Invalid pruner configuration: global-config.namespaces.dev.pipelineRuns[0]: 
selectors are NOT supported in global ConfigMap. 
Use namespace-level ConfigMap (tekton-pruner-namespace-spec) instead
```

**Why this restriction?**
- Selector-based resource matching requires access to the resource's labels/annotations at runtime
- Global ConfigMaps are designed for cluster-wide defaults, not resource-specific matching
- Namespace ConfigMaps provide the correct scope for dynamic resource selection

**See also:** [Resource Groups Tutorial](tutorials/resource-groups.md) for selector usage examples

### 3. Naming Requirements

**Global Config:**
- **Name:** Must be `tekton-pruner-default-spec` (fixed)
- **Namespace:** Must be `tekton-pipelines` (or system namespace)
- **Label:** `pruner.tekton.dev/config-type: global`

**Namespace Config:**
- **Name:** Must be `tekton-pruner-namespace-spec` (fixed)
- **Namespace:** Must be in a user namespace (NOT in system or tekton namespaces)
- **Label:** `pruner.tekton.dev/config-type: namespace`

**Error examples:**
```
Global config must be named 'tekton-pruner-default-spec', got: my-custom-name
```
```
Namespace config must be named 'tekton-pruner-namespace-spec', got: pruner-config
```

### 3. Namespace Restrictions

**Forbidden namespaces for namespace-level configs:**
- System namespaces: `kube-*`, `openshift-*`
- Tekton namespaces: `tekton-pipelines`, `tekton-*`

Attempting to create a namespace-level config in these locations will be rejected.

**Error example:**
```
Invalid pruner ConfigMap configuration: wrong config-type label or namespace combination
```

### 4. Configuration Content Validation

The webhook validates configuration data including:

- **Time values**: ttlSecondsAfterFinished must be non-negative
- **History limits**: historyLimit, successfulHistoryLimit, and failedHistoryLimit must be non-negative and cannot exceed global maximums if enforced
- **Selectors** (namespace ConfigMaps only): Label and annotation selectors must have valid key-value pairs; name selectors must be valid resource names

**Note:** Selectors (pipelineRuns, taskRuns arrays with matchLabels/matchAnnotations) are only processed in namespace-level ConfigMaps. They are ignored in global ConfigMaps.

### 5. Deletion Protection

The webhook prevents deletion of the global config if namespace-level configs still exist. You must delete all namespace configs before deleting the global config. Namespace configs can be deleted without restrictions.

### 6. Global Config Enforcement

When creating or updating namespace-level configs, the webhook fetches the global config and validates that namespace values do not exceed global maximums if defined (e.g., maxTTLSecondsAfterFinished, maxHistoryLimit).

## Common Validation Errors

### Missing Labels Error
```
ConfigMap must have labels
```
**Solution:** Add the required labels to your ConfigMap:
```yaml
labels:
  app.kubernetes.io/part-of: tekton-pruner
  pruner.tekton.dev/config-type: global  # or namespace
```

### Wrong Config Type Error
```
label pruner.tekton.dev/config-type must be 'global' or 'namespace', got: local
```
**Solution:** Use only `global` or `namespace` as config-type values.

### Wrong Name Error
```
Global config must be named 'tekton-pruner-default-spec', got: tekton-pruner-config
```
**Solution:** Use the exact required name:
- Global: `tekton-pruner-default-spec`
- Namespace: `tekton-pruner-namespace-spec`

### Exceeds Global Limit Error
```
Invalid pruner configuration: namespace config ttlSecondsAfterFinished (172800) exceeds global maximum (86400)
```
**Solution:** Reduce the namespace value to be within global limits, or request admin to increase global maximum.

### Deletion Blocked Error
```
Cannot delete global config: 2 namespace config(s) still exist
```
**Solution:** Delete all namespace configs first:
```bash
kubectl delete cm tekton-pruner-namespace-spec -n dev
kubectl delete cm tekton-pruner-namespace-spec -n staging
# Then delete global config
kubectl delete cm tekton-pruner-default-spec -n tekton-pipelines
```

## Troubleshooting

### Verify Webhook Status

```bash
# Check webhook configuration
kubectl get validatingwebhookconfigurations tekton-pruner-validating-webhook

# Check webhook pod and service
kubectl get pods,svc -n tekton-pipelines -l app.kubernetes.io/component=webhook

# View webhook logs
kubectl logs -n tekton-pipelines -l app.kubernetes.io/component=webhook
```

### Test Validation

Create an invalid ConfigMap to verify webhook is functioning:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    ttlSecondsAfterFinished: 300
EOF
```

Expected error: `admission webhook denied the request: Invalid pruner ConfigMap labels`

## Bypassing Validation (Not Recommended)

**Warning:** Bypassing validation can lead to misconfigured pruner behavior and should only be done in emergency situations.

To temporarily bypass validation:

1. Remove the identifying label:
   ```bash
   kubectl label cm tekton-pruner-default-spec -n tekton-pipelines \
     app.kubernetes.io/part-of-
   ```

2. Make necessary changes

3. Re-add the label:
   ```bash
   kubectl label cm tekton-pruner-default-spec -n tekton-pipelines \
     app.kubernetes.io/part-of=tekton-pruner
   ```

**Note:** The pruner controller will not process ConfigMaps without proper labels.

## Best Practices

1. **Always include required labels** from the start
2. **Use exact naming conventions** for global and namespace configs
3. **Test configurations** in a non-production namespace first
4. **Check validation errors** carefully - they indicate what needs to be fixed
5. **Monitor webhook logs** when rolling out new configurations
6. **Delete namespace configs** before attempting to delete global config

## Related Documentation

- [Getting Started](tutorials/getting-started.md) - Basic configuration setup
- [Namespace Configuration](tutorials/namespace-configuration.md) - Namespace-level config details
- [Troubleshooting](troubleshooting.md) - General troubleshooting guide
