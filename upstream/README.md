<!--

---
title: "Tekton resource pruning based on configuration"
linkTitle: "Tekton Resource Pruning"
weight: 10
description: Configuration based event driven pruning for Tekton
cascade:
  github_project_repo: https://github.com/tektoncd/pruner
---
-->

# Tekton Pruner

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/tektoncd/pruner/blob/main/LICENSE)

Tekton Pruner automatically manages the lifecycle of Tekton resources by cleaning up completed PipelineRuns and TaskRuns based on configurable time-based (TTL) and history-based policies.

## Overview

Tekton Pruner provides event-driven and configuration-based cleanup through four controllers:
- **Main Pruner Controller**: Processes cleanup based on ConfigMap settings
- **Namespace Pruner Config Controller**: Watches namespace-level ConfigMaps
- **PipelineRun Controller**: Handles PipelineRun events
- **TaskRun Controller**: Handles standalone TaskRun events

<p align="center">
<img src="docs/images/pruner_functional_abstract.png" alt="Tekton Pruner overview"></img>
</p>

## Key Features

- **Time-based Pruning (TTL)**: Delete resources after specified duration using `ttlSecondsAfterFinished`
- **History-based Pruning**: Retain fixed number of runs using `successfulHistoryLimit`, `failedHistoryLimit`, or `historyLimit`
- **Hierarchical Configuration**: Global (cluster-wide) → Namespace → Resource Group (Tech Preview)
- **Flexible Selectors**: Group resources by labels, annotations, or names for fine-grained control

## Installation

**Prerequisites:**
- Kubernetes cluster with [Tekton Pipelines](https://github.com/tektoncd/pipeline/blob/main/docs/install.md) installed

**Install:**
```bash
export VERSION=0.1.0  # Update as needed
kubectl apply -f "https://github.com/tektoncd/pruner/releases/download/v$VERSION/release-v$VERSION.yaml"
```

**Verify:**
```bash
kubectl get pods -n tekton-pipelines -l app=tekton-pruner-controller
```

## Configuration

> **CRITICAL**: All pruner ConfigMaps **MUST** include these labels for validation and processing:
> 
> ```yaml
> labels:
>   app.kubernetes.io/part-of: tekton-pruner
>   pruner.tekton.dev/config-type: <global|namespace>
> ```
> 
> **System Boundaries**: Do NOT create namespace-level ConfigMaps in:
> - System namespaces (`kube-*`, `openshift-*`)
> - Tekton controller namespaces (`tekton-pipelines`, `tekton-*`)

### Configuration Hierarchy

1. **Global Config** (cluster-wide defaults in `tekton-pipelines` namespace)
2. **Namespace Config** (per-namespace overrides when `enforcedConfigLevel: namespace`)
3. **Resource Groups** (fine-grained control via selectors)

### Quick Start: Global Configuration

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

### Namespace-Specific Configuration

**Option 1: Inline in Global ConfigMap**
```yaml
data:
  global-config: |
    enforcedConfigLevel: namespace
    namespaces:
      my-namespace:
        ttlSecondsAfterFinished: 60
```

**Option 2: Separate Namespace ConfigMap** (Recommended for self-service)
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec
  namespace: my-app-namespace  # User namespace only
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: namespace
data:
  ns-config: |
    ttlSecondsAfterFinished: 300
    successfulHistoryLimit: 5
```

### Resource Groups (Fine-grained Control)

Group resources by labels/annotations for different policies:

```yaml
data:
  global-config: |
    pipelineRuns:
      - selector:
          matchLabels:
            environment: production
        ttlSecondsAfterFinished: 604800  # 7 days
        successfulHistoryLimit: 10
      - selector:
          matchLabels:
            environment: development
        ttlSecondsAfterFinished: 300     # 5 minutes
        successfulHistoryLimit: 3
```

**For detailed tutorials, see:**
- [Getting Started](docs/tutorials/getting-started.md)
- [Namespace Configuration](docs/tutorials/namespace-configuration.md)
- [Resource Groups](docs/tutorials/resource-groups.md)
- [ConfigMap Validation](docs/configmap-validation.md) - How ConfigMaps are validated by the webhook

## Contributing

- See [DEVELOPMENT.md](DEVELOPMENT.md) for development setup
- Submit issues and pull requests
- Follow coding standards and test coverage requirements

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details