<!--

---
title: "Tekton resource pruning based on predefined configuration"
linkTitle: "Tekton Resource Pruning"
weight: 10
description: Configuration based event driven pruning solution for Tekton
cascade:
  github_project_repo: https://github.com/openshift-pipelines/tektoncd-pruner
---
-->

# Tekton Pruner

TO BE UPDATED [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/6408/badge)](https://bestpractices.coreinfrastructure.org/projects/6408)

Event based pruning of Tekton resources

<p align="center">
<img src="tekton_chains-color.png" alt="Tekton Pruner logo"></img>
</p>

## Getting Started

Tekton Pruner is a controller that continuously monitors a predefined configuration in a ConfigMap and
removes PipelineRuns and TaskRuns according to the specified settings.

By default, the Pruner controller watches the ConfigMap tekton-pruner-default-spec. Additionally, 
it includes two other controllers that monitor every PipelineRun and TaskRun event in the cluster, 
continuously reconciling to remove them based on the ConfigMap settings.

<p align="center">
<img src="docs/images/pruner_functional_abstract.png" alt="Tekton Pruner overview"></img>
</p>

Current Features:

1. Ability to set a cluster-wide pruning configuration that applies to all namespaces in the cluster.

   Pruning Configuration Variations (Prioritized from Highest to Lowest):

   - Pruning PipelineRuns and TaskRuns based on TTL (time-to-live) in seconds.`ttlSecondsAfterFinished`
   - Defining the number of successful PipelineRuns/TaskRuns to retain.`successfulHistoryLimit`
   - Defining the number of failed PipelineRuns/TaskRuns to retain.`failedHistoryLimit`
   - Setting a fixed number of PipelineRuns/TaskRuns to retain, regardless of status.`historyLimit`

   sample configMap depiciting the above settings that apply to all namespaces in the cluster

   ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
        name: tekton-pruner-default-spec
        namespace: tekton-pipelines
    data:
        global-config: |
            ttlSecondsAfterFinished: 600 # 5 minutes
            successfulHistoryLimit: 3 
            failedHistoryLimit: 3
            historyLimit: 5  
   ```

Work In Progress Features:

2. Ability to set pruning configuration specific to a Namespace that can overrides the cluster-wide settings

3. Ability to set pruning configuration specific to PipelineRuns/TaskRuns that can be identified either by 
      - Parent Pipleline/Task Name 
      - Matching Labels
      - Matching Annotations
   This configurations override the Namespace specific configuration


### Installation **TO BE UPDATED**

Prerequisite: you'll need
[Tekton Pipelines](https://github.com/tektoncd/pipeline/blob/main/docs/install.md) installed on your cluster before you install Pruner.

To install the latest version of Pruner to your Kubernetes cluster, run:

```shell
kubectl apply --filename ??release.yaml
```

To install a specific version of Chains, run:

```shell
kubectl apply -f ??release.yaml
```

To verify that installation was successful, wait until all Pods have Status
`Running`:

```shell
kubectl get po -n tekton-pipelines --watch
```

```
NAME                                          READY   STATUS      RESTARTS   AGE
tekton-pruner-controller-5756bc7cb9-lblnx      1/1     Running      0        10s
```

### Setup **WIP**

To view and edit the default configuartions:

- [Set up any additional configuration](docs/config.md)

## Tutorials **WIP**

To get started with pruner, try out our
[getting started tutorial](docs/tutorials/getting-started-tutorial.md).

## Want to contribute - TO BE UPDATED

We are so excited to have you!

- See [CONTRIBUTING.md](CONTRIBUTING.md) for an overview of our processes
- See [DEVELOPMENT.md](DEVELOPMENT.md) for how to get started
- See [ROADMAP.md](ROADMAP.md) for the current roadmap Check out our good first
  issues and our help wanted issues to get started!
- See [releases.md](releases.md) for our release cadence and processes