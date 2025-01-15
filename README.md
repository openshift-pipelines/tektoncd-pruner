# Tektoncd Pruner Controller

Tektoncd Pruner is Kubernetes Custom Resource Definition (CRD) controller. 

Tekton Pruner is designed to manage the lifecycle of Tekton resources such as PipelineRuns and TaskRuns. It achieves this by applying configurable retention policies and time-to-live (TTL) rules


You can refer the Tekton Pruner CR example 

```yaml
---
apiVersion: pruner.tekton.dev/v1alpha1
kind: TektonPruner
metadata:
  name: ns-1
  namespace: ns-1
spec:
  ttlSecondsAfterFinished: 99 # default ttl for all the resources on this namespace
  pipelines:
    - name: foo
      ttlSecondsAfterFinished: 120 # 60 seconds
    - name: bar
      ttlSecondsAfterFinished: 600 # 10 minutes
    - name: echo-string
      ttlSecondsAfterFinished: 111
      successfulHistoryLimit: 3
      failedHistoryLimit: 2
  tasks:
    - name: task1
      ttlSecondsAfterFinished: 60 # 60 seconds
```

### Features

* Namespace specific specifications should be supplied via TektonPruner CR on a namespace
  * Order precedence as follows (bottom to top)
    * Global Configuration - root level
    * Namespaced Configuration - root level
    * Global Configuration - resource level
    * Namespaced Configuration - resource level
    * Resource level annotations  (always win precedence)
      * “pruner.tekton.dev/ttlSecondsAfterFinished”
      * “pruner.tekton.dev/successfulHistoryLimit”
      * “pruner.tekton.dev/failedHistoryLimit”