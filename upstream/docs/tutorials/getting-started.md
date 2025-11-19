<!--
---
linkTitle: "Tutorial: Getting Started"
weight: 100
---
-->

# Getting Started with Tekton Pruner

This tutorial walks you through installing Tekton Pruner and creating your first configuration.

## Prerequisites

- Kubernetes cluster with [Tekton Pipelines](https://github.com/tektoncd/pipeline/blob/main/docs/install.md) installed

## Installation

```bash
kubectl apply -f https://raw.githubusercontent.com/tektoncd/pruner/main/release.yaml
kubectl get pods -n tekton-pipelines -l app=tekton-pruner-controller
```

## Understanding Required Labels

> **CRITICAL**: All pruner ConfigMaps **MUST** have these labels:
> 
> ```yaml
> labels:
>   app.kubernetes.io/part-of: tekton-pruner
>   pruner.tekton.dev/config-type: <global|namespace>
> ```
> 
> These labels enable the pruner webhook to validate ConfigMaps and controllers to process them correctly.

## Create Your First Configuration

Create a global configuration that deletes completed resources after 5 minutes and keeps the last 3 runs:

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
    enforcedConfigLevel: global
    ttlSecondsAfterFinished: 300
    successfulHistoryLimit: 3
    failedHistoryLimit: 3
```

Apply and verify:

```bash
kubectl apply -f pruner-config.yaml
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines
```

## Test the Configuration

Create test PipelineRuns to verify pruning:

```bash
# Create a simple pipeline
kubectl apply -f - <<EOF
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: hello-pipeline
spec:
  tasks:
    - name: hello
      taskSpec:
        steps:
          - name: echo
            image: ubuntu
            command: ['echo']
            args: ['hello world']
EOF

# Create multiple runs
for i in {1..5}; do
  kubectl create -f - <<EOF
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  generateName: hello-pipeline-run-
spec:
  pipelineRef:
    name: hello-pipeline
EOF
done

# Watch pruning in action (after 5 minutes + completion time)
kubectl get pipelineruns -w
```

## Configuration Options

| Setting | Description | Example |
|---------|-------------|---------|
| `ttlSecondsAfterFinished` | Delete after N seconds | `300` (5 min) |
| `successfulHistoryLimit` | Keep N successful runs | `3` |
| `failedHistoryLimit` | Keep N failed runs | `3` |
| `historyLimit` | Keep N runs (both types) | `5` |
| `enforcedConfigLevel` | Config hierarchy level | `global` or `namespace` |

## Next Steps

- **[Namespace Configuration](./namespace-configuration.md)** - Per-namespace settings and validation boundaries
- **[Resource Groups](./resource-groups.md)** - Fine-grained control with selectors
- **[Time-based Pruning](./time-based-pruning.md)** - TTL strategies for different environments
- **[History-based Pruning](./history-based-pruning.md)** - Retention strategies by status

