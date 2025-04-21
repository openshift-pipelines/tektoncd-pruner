<!--
---
linkTitle: "Tutorial: Getting Started"
weight: 100
---
-->

# Pruner Getting Started Tutorial

This tutorial will guide you through the process of setting up and configuring Tekton Pruner to remove Tekton-specific custom resources, initially focusing on PipelineRun (PR) and TaskRun (TR), along with all related objects owned by these PRs/TRs, from the etcd of the specified cluster.

## Prerequisites

A Kubernetes cluster with the following installed:

* [Tekton Pipelines](https://github.com/tektoncd/pipeline/blob/main/docs/install.md)

### Pruner Installation

1. Install Tekton Pruner using kubectl:

```bash
kubectl apply -f https://raw.githubusercontent.com/openshift-pipelines/tektoncd-pruner/main/release.yaml
```

2. Verify the installation:

```bash
kubectl get pods -n tekton-pipelines
```

You should see the `tekton-pruner-controller` pod running.

### Basic Pruner Configuration

1. Create a basic configuration for the pruner:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
data:
  global-config: |
    enforcedConfigLevel: global
    ttlSecondsAfterFinished: 300    # Delete resources 5 minutes after completion
    successfulHistoryLimit: 3        # Keep last 3 successful runs
    failedHistoryLimit: 3           # Keep last 3 failed runs
```

Apply the configuration:

```bash
kubectl apply -f pruner-config.yaml
```

2. Verify the configuration:

```bash
kubectl get configmap tekton-pruner-default-spec -n tekton-pipelines -o yaml
```

### Testing the Pruner

1. Create a simple pipeline for testing:

```yaml
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
```

2. Create multiple PipelineRuns:

```bash
kubectl create -f - <<EOF
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  generateName: hello-pipeline-run-
spec:
  pipelineRef:
    name: hello-pipeline
EOF
```

3. Watch the PipelineRuns:

```bash
kubectl get pipelineruns -w
```

You should see older completed PipelineRuns being automatically cleaned up based on your configuration.

### Next Steps

- Learn about [History-based Pruning](./history-based-pruning.md)
- Explore [Time-based Pruning](./time-based-pruning.md)
- Configure [Namespace-specific Settings](./namespace-configuration.md)
- Set up [Resource Groups](./resource-groups.md)

