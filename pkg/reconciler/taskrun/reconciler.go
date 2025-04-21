package taskrun

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
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

// NewTrFuncs creates a new instance of TrFuncs with the provided pipeline client.
// This client is used to interact with the Tekton pipeline API.
func NewTrFuncs(client pipelineversioned.Interface) *TrFuncs {
	return &TrFuncs{client: client}
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

// ListByLabels returns a list of TaskRuns in a given namespace filtered by multiple labels.
func (trf *TrFuncs) ListByLabels(ctx context.Context, namespace string, labels map[string]string) ([]metav1.Object, error) {
	logger := logging.FromContext(ctx)
	selector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: labels})

	trsList, err := trf.client.TektonV1().TaskRuns(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	trs := []metav1.Object{}
	for _, tr := range trsList.Items {
		trs = append(trs, tr.DeepCopy())
	}

	logger.Debugw("TaskRuns list by labels", "namespace", namespace, "labels", labels)

	return trs, nil
}

// ListByAnnotations returns a list of TaskRuns in a given namespace filtered by annotations.
func (trf *TrFuncs) ListByAnnotations(ctx context.Context, namespace string, annotations map[string]string) ([]metav1.Object, error) {
	logger := logging.FromContext(ctx)
	allTrs, err := trf.List(ctx, namespace, "")
	if err != nil {
		return nil, err
	}

	filteredTrs := []metav1.Object{}
	for _, tr := range allTrs {
		match := true
		for key, value := range annotations {
			if tr.GetAnnotations()[key] != value {
				match = false
				break
			}
		}
		if match {
			filteredTrs = append(filteredTrs, tr)
		}
	}

	logger.Debugw("TaskRuns list by annotations", "namespace", namespace, "annotations", annotations)

	return filteredTrs, nil
}

// ListByNamespaces returns a list of TaskRuns across multiple namespaces.
func (trf *TrFuncs) ListByNamespaces(ctx context.Context, namespaces []string) (map[string][]metav1.Object, error) {
	logger := logging.FromContext(ctx)
	results := make(map[string][]metav1.Object)

	for _, ns := range namespaces {
		trs, err := trf.List(ctx, ns, "")
		if err != nil {
			logger.Errorw("Failed to list TaskRuns", "namespace", ns, "error", err)
			continue
		}
		results[ns] = trs
	}

	logger.Debugw("TaskRuns list by namespaces", "namespaces", namespaces)

	return results, nil
}

/*
// List returns a list of TaskRuns in a given namespace with label and annotation selectors.
// Annotations take higher priority. If annotations match, labels are ignored for that resource.
func (trf *TrFuncs) List(ctx context.Context, namespace string, annotations interface{}, labels interface{}) ([]metav1.Object, error) {
	logger := logging.FromContext(ctx)

	var annotationSelector string
	var labelSelector string

	// Handle annotations (manually filter later if provided)
	if annotations != nil {
		switch v := annotations.(type) {
		case string:
			// If a single annotation is provided, use it directly
			annotationSelector = v
		case map[string]string:
			// If a map of annotations is provided, construct a selector for multiple annotations
			for key, value := range v {
				if annotationSelector != "" {
					annotationSelector += ","
				}
				annotationSelector += key + "=" + value
			}
		default:
			return nil, fmt.Errorf("invalid annotations type: must be string or map[string]string")
		}
	}
	logger.Debugw("annotationSelector", annotationSelector)

	// Handle labels
	if labels != nil {
		switch v := labels.(type) {
		case string:
			// If a single label is provided, use it directly
			labelSelector = v
		case map[string]string:
			// If a map of labels is provided, construct a selector for multiple labels
			for key, value := range v {
				if labelSelector != "" {
					labelSelector += ","
				}
				labelSelector += key + "=" + value
			}
		default:
			return nil, fmt.Errorf("invalid labels type: must be string or map[string]string")
		}
	}
	logger.Debugw("labelSelector", labelSelector)

	// Prepare options to list resources with the correct label selector
	options := metav1.ListOptions{}

	// Apply label selector if provided
	if labelSelector != "" {
		options.LabelSelector = labelSelector
	}

	// List TaskRuns using the constructed label selector
	trsList, err := trf.client.TektonV1().TaskRuns(namespace).List(ctx, options)
	if err != nil {
		return nil, err
	}

	// Filter by annotations first if annotations are provided
	var filteredTRs []metav1.Object
	if annotationSelector != "" {
		for _, tr := range trsList.Items {
			matches := true
			// Check annotations if the selector matches
			annotations := tr.GetAnnotations()
			for key, value := range annotations {
				if !strings.Contains(annotationSelector, key+"="+value) {
					matches = false
					break
				}
			}
			if matches {
				// If annotations match, include the resource and skip label filtering
				filteredTRs = append(filteredTRs, tr.DeepCopy())
			}
		}
	} else {
		// If no annotations are provided, apply label filtering
		for _, tr := range trsList.Items {
			if labelSelector == "" || config.MatchLabels(tr.GetLabels(), labelSelector) {
				filteredTRs = append(filteredTRs, tr.DeepCopy())
			}
		}
	}

	// Return the filtered list of TaskRuns
	return filteredTRs, nil
}
*/

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

// Patch modifies an existing TaskRun resource using a JSON patch.
// This is useful for updating only specific fields of the resource.
func (trf *TrFuncs) Patch(ctx context.Context, namespace, name string, patchBytes []byte) error {
	_, err := trf.client.TektonV1().TaskRuns(namespace).Patch(
		ctx,
		name,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)

	if err != nil {
		return fmt.Errorf("failed to patch TaskRun %s/%s: %w", namespace, name, err)
	}

	return nil
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
func (trf *TrFuncs) GetTTLSecondsAfterFinished(namespace, taskName string, selectors config.SelectorSpec) (*int32, string) {
	return config.PrunerConfigStore.GetTaskTTLSecondsAfterFinished(namespace, taskName, selectors)
}

// GetSuccessHistoryLimitCount retrieves the success history limit count for a TaskRun.
func (trf *TrFuncs) GetSuccessHistoryLimitCount(namespace, name string, selectors config.SelectorSpec) (*int32, string) {
	return config.PrunerConfigStore.GetTaskSuccessHistoryLimitCount(namespace, name, selectors)
}

// GetFailedHistoryLimitCount retrieves the failed history limit count for a TaskRun.
func (trf *TrFuncs) GetFailedHistoryLimitCount(namespace, name string, selectors config.SelectorSpec) (*int32, string) {
	return config.PrunerConfigStore.GetTaskFailedHistoryLimitCount(namespace, name, selectors)
}

// GetEnforcedConfigLevel retrieves the enforced config level for a TaskRun.
func (trf *TrFuncs) GetEnforcedConfigLevel(namespace, name string, selectors config.SelectorSpec) config.EnforcedConfigLevel {
	return config.PrunerConfigStore.GetTaskEnforcedConfigLevel(namespace, name, selectors)
}
