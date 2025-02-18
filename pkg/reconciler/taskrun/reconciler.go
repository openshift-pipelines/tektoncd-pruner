package taskrun

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineversioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/taskrun"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

// Reconciler implements simpledeploymentreconciler.Interface for
// SimpleDeployment resources.
type Reconciler struct {
	kubeclient     kubernetes.Interface
	ttlHandler     *config.TTLHandler
	historyLimiter *config.HistoryLimiter
}

// Check that our Reconciler implements Interface
var _ taskrunreconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, tr *pipelinev1.TaskRun) reconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Debugw("received a TaskRun event",
		"namespace", tr.Namespace, "name", tr.Name,
	)

	// if the TaskRun is not a standalone, no action needed
	// if so, will be handled by it is parent resource(PipelineRun)
	if !isStandaloneTaskRun(tr) {
		return nil
	}

	// execute the history limiter earlier than the ttl handler

	// execute history limit action
	err := r.historyLimiter.ProcessEvent(ctx, tr)
	if err != nil {
		logger.Errorw("error on processing history limiting for a TaskRun",
			"namespace", tr.Namespace, "name", tr.Name,
			zap.Error(err),
		)
		return err
	}

	// execute ttl handler
	err = r.ttlHandler.ProcessEvent(ctx, tr)
	if err != nil {
		isRequeueKey, _ := controller.IsRequeueKey(err)
		// the error is not a requeue error, print the error
		if !isRequeueKey {
			data, _ := json.Marshal(tr)
			logger.Errorw("error on processing ttl for a TaskRun",
				"namespace", tr.Namespace, "name", tr.Name,
				"resource", string(data),
				zap.Error(err),
			)
		}
		return err
	}

	return nil
}

// TrFuncs provides methods for working with TaskRun resources
// it contains a client to interact with the pipeline API and manage TaskRuns
type TrFuncs struct {
	client pipelineversioned.Interface
}

// Type returns the kind of resource represented by the TaskRunFuncs struct, which is "TaskRun".
func (trf *TrFuncs) Type() string {
	return config.KindTaskRun
}

// List returns a list of TaskRuns in a given namespace with a label selector.
func (trf *TrFuncs) List(ctx context.Context, namespace, labelSelector string) ([]metav1.Object, error) {
	// TODO: should we have to implement pagination support?
	prsList, err := trf.client.TektonV1().TaskRuns(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	trs := []metav1.Object{}
	for _, tr := range prsList.Items {
		trs = append(trs, tr.DeepCopy())
	}
	return trs, nil
}

// Get retrieves a specific TaskRun by name in the given namespace.
func (trf *TrFuncs) Get(ctx context.Context, namespace, name string) (metav1.Object, error) {
	return trf.client.TektonV1().TaskRuns(namespace).Get(ctx, name, metav1.GetOptions{})
}

// Delete removes a specific TaskRun by name in the given namespace.
func (trf *TrFuncs) Delete(ctx context.Context, namespace, name string) error {
	return trf.client.TektonV1().TaskRuns(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Update modifies an existing TaskRun resource.
func (trf *TrFuncs) Update(ctx context.Context, resource metav1.Object) error {
	tr, ok := resource.(*pipelinev1.TaskRun)
	if !ok {
		return fmt.Errorf("invalid type received. Namespace:%s, Name:%s", resource.GetNamespace(), resource.GetName())
	}
	_, err := trf.client.TektonV1().TaskRuns(resource.GetNamespace()).Update(ctx, tr, metav1.UpdateOptions{})
	return err
}

// GetCompletionTime retrieves the completion time of a TaskRun resource.
func (trf *TrFuncs) GetCompletionTime(resource metav1.Object) (metav1.Time, error) {
	tr, ok := resource.(*pipelinev1.TaskRun)
	if !ok {
		return metav1.Time{}, fmt.Errorf("resource type error, this is not a TaskRun resource. namespace:%s, name:%s, type:%T",
			resource.GetNamespace(), resource.GetName(), resource)
	}
	if tr.Status.CompletionTime != nil {
		return *tr.Status.CompletionTime, nil
	}

	// check the status from conditions
	condition := tr.Status.GetCondition(apis.ConditionSucceeded)
	if condition != nil && condition.Status != corev1.ConditionUnknown {
		finishAt := condition.LastTransitionTime
		if finishAt.Inner.IsZero() {
			return metav1.Time{}, fmt.Errorf("unable to find the time when the resource '%s/%s' finished", tr.Namespace, tr.Name)
		}
		return condition.LastTransitionTime.Inner, nil
	}

	// This should never happen if the Resource has finished
	return metav1.Time{}, fmt.Errorf("unable to find the status of the finished resource: %s/%s", tr.Namespace, tr.Name)
}

// Ignore returns true if the resource should be ignored based on labels and annotations.
func (trf *TrFuncs) Ignore(resource metav1.Object) bool {
	// labels and annotations are not populated, lets wait sometime
	if resource.GetLabels() == nil {
		if resource.GetAnnotations() == nil || resource.GetAnnotations()[config.AnnotationTTLSecondsAfterFinished] == "" {
			return true
		}
	}
	return false
}

// IsCompleted checks if the TaskRun resource has completed.
func (trf *TrFuncs) IsCompleted(resource metav1.Object) bool {
	tr, ok := resource.(*pipelinev1.TaskRun)
	if !ok {
		return false
	}

	if tr.Status.StartTime == nil {
		return false
	}

	if tr.Status.CompletionTime != nil {
		return true
	}

	// check the status from conditions
	condition := tr.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil || condition.Status == corev1.ConditionUnknown {
		return false
	}

	return true
}

// IsSuccessful checks if the TaskRun resource has successfully completed.
func (trf *TrFuncs) IsSuccessful(resource metav1.Object) bool {
	tr, ok := resource.(*pipelinev1.TaskRun)
	if !ok {
		return false
	}

	condition := tr.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil {
		return false
	}

	runReason := pipelinev1.TaskRunReason(condition.Reason)
	return runReason == pipelinev1.TaskRunReasonSuccessful
}

// IsFailed checks if the TaskRun resource has failed.
func (trf *TrFuncs) IsFailed(resource metav1.Object) bool {
	_, ok := resource.(*pipelinev1.TaskRun)
	if !ok {
		return false
	}

	return !trf.IsSuccessful(resource)
}

// GetDefaultLabelKey returns the default label key for TaskRun resources.
func (trf *TrFuncs) GetDefaultLabelKey() string {
	return config.LabelTaskName
}

// GetTTLSecondsAfterFinished retrieves the TTL (time-to-live) in seconds after a TaskRun finishes.
func (trf *TrFuncs) GetTTLSecondsAfterFinished(namespace, taskName string) *int32 {
	return config.PrunerConfigStore.GetTaskTTLSecondsAfterFinished(namespace, taskName)
}

// GetSuccessHistoryLimitCount retrieves the success history limit count for a TaskRun.
func (trf *TrFuncs) GetSuccessHistoryLimitCount(namespace, name string) *int32 {
	return config.PrunerConfigStore.GetTaskSuccessHistoryLimitCount(namespace, name)
}

// GetFailedHistoryLimitCount retrieves the failed history limit count for a TaskRun.
func (trf *TrFuncs) GetFailedHistoryLimitCount(namespace, name string) *int32 {
	return config.PrunerConfigStore.GetTaskFailedHistoryLimitCount(namespace, name)
}

// GetEnforcedConfigLevel retrieves the enforced config level for a TaskRun.
func (trf *TrFuncs) GetEnforcedConfigLevel(namespace, name string) config.EnforcedConfigLevel {
	return config.PrunerConfigStore.GetTaskEnforcedConfigLevel(namespace, name)
}
