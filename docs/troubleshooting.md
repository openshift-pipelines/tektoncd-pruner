# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with Tekton Pruner.

## Common Issues

### 1. Resources Not Being Pruned

#### Symptoms
- PipelineRuns or TaskRuns remain after their expected deletion time
- No pruning activities visible in logs

#### Possible Causes and Solutions

1. Configuration Not Applied
```bash
# Check if config exists
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines

# Verify configuration content
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines -o yaml
```

2. Controller Not Running
```bash
# Check controller status
kubectl get pods -n tekton-pipelines -l app=tekton-pruner-controller

# Check controller logs
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller
```

3. Incorrect Resource Labels
```bash
# Check resource labels
kubectl get pipelineruns --show-labels
```

### 2. Unexpected Resource Deletion

#### Symptoms
- Resources being deleted earlier than expected
- Too many resources being pruned

#### Possible Causes and Solutions

1. Check TTL Configuration
```bash
# Verify TTL settings in config
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines -o jsonpath='{.data.global-config}'
```

2. Check History Limits
- Ensure history limits are set appropriately
- Verify resource completion status is being detected correctly

### 3. Permission Issues

#### Symptoms
- Error messages about RBAC in controller logs
- Unable to delete resources

#### Solutions

1. Verify RBAC Configuration
```bash
# Check ClusterRole
kubectl get clusterrole tekton-pruner-controller

# Check ClusterRoleBinding
kubectl get clusterrolebinding tekton-pruner-controller

# Check ServiceAccount
kubectl get serviceaccount tekton-pruner-controller -n tekton-pipelines
```

2. Apply Missing RBAC Rules
```bash
kubectl apply -f config/200-clusterrole.yaml
kubectl apply -f config/201-clusterrolebinding.yaml
```

## Collecting Debug Information

### 1. Controller Logs

```bash
# Get recent logs
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller --tail=100

# Get logs with timestamps
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller --timestamps=true

# Follow logs in real-time
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller -f
```

### 2. Resource Status

```bash
# List PipelineRuns with details
kubectl get pipelineruns -o wide

# Get specific PipelineRun details
kubectl describe pipelinerun <pipelinerun-name>
```

### 3. Configuration Validation

```bash
# Export current config
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines -o yaml > current-config.yaml

# Compare with default config
diff current-config.yaml config/600-tekton-pruner-default-spec.yaml
```

## Best Practices for Troubleshooting

1. Start with Controller Logs
   - Check for error messages
   - Look for pruning activity
   - Verify configuration is being read

2. Verify Resource State
   - Check resource status
   - Verify labels and annotations
   - Confirm completion timestamps

3. Test with Simple Configuration
   - Start with basic global settings
   - Add complexity gradually
   - Test one feature at a time

4. Monitor Resource Changes
   - Watch resources in real-time
   - Track deletion patterns
   - Verify pruning behavior

## Getting Help

If you're still experiencing issues:

1. Search existing GitHub issues
2. Collect relevant logs and configuration
3. Open a new issue with:
   - Clear description of the problem
   - Steps to reproduce
   - Expected vs actual behavior
   - Relevant logs and configuration
   - Kubernetes and Tekton versions

## Upgrading and Downgrading

If issues persist, try:

1. Upgrading to the latest version
```bash
kubectl apply -f https://raw.githubusercontent.com/openshift-pipelines/tektoncd-pruner/main/release.yaml
```

2. Reverting to a known working version
```bash
# Replace VERSION with the desired version
kubectl apply -f https://raw.githubusercontent.com/openshift-pipelines/tektoncd-pruner/VERSION/release.yaml
```