package pipelinerun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelineversioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/pipelinerun"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

// Reconciler includes the kubernetes client to interact with the cluster
type Reconciler struct {
	kubeclient     kubernetes.Interface
	ttlHandler     *config.TTLHandler
	historyLimiter *config.HistoryLimiter
}

// Check that our Reconciler implements Interface
var _ pipelinerunreconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, pr *pipelinev1.PipelineRun) reconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Debugw("received a PipelineRun event",
		"namespace", pr.Namespace, "name", pr.Name,
	)

	// execute the history limiter earlier than the ttl handler

	// execute history limit action
	err := r.historyLimiter.ProcessEvent(ctx, pr)
	if err != nil {
		logger.Errorw("error on processing history limiting for a PipelineRun",
			"namespace", pr.Namespace, "name", pr.Name,
			zap.Error(err),
		)
		return err
	}

	// execute ttl handler
	err = r.ttlHandler.ProcessEvent(ctx, pr)
	if err != nil {
		isRequeueKey, _ := controller.IsRequeueKey(err)
		// the error is not a requeue error, print the error
		if !isRequeueKey {
			data, _ := json.Marshal(pr)
			logger.Errorw("error on processing ttl for a PipelineRun",
				"namespace", pr.Namespace, "name", pr.Name,
				"resource", string(data),
				zap.Error(err),
			)
		}
		return err
	}

	return nil
}

// PrFuncs provides methods for working with PipelineRun resources
// it contains a client to interact with the pipeline API and manage PipelineRuns
type PrFuncs struct {
	client pipelineversioned.Interface
}

// Type returns the kind of resource represented by the PRFuncs struct, which is "PipelineRun".
func (prf *PrFuncs) Type() string {
	return config.KindPipelineRun
}

// List returns a list of PipelineRuns in a given namespace with a label selector.
func (prf *PrFuncs) List(ctx context.Context, namespace, label string) ([]metav1.Object, error) {
	// TODO: should we have to implement pagination support?
	prsList, err := prf.client.TektonV1().PipelineRuns(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return nil, err
	}

	prs := []metav1.Object{}
	for _, pr := range prsList.Items {
		prs = append(prs, pr.DeepCopy())
	}
	return prs, nil
}

// List returns a list of PipelineRuns in a given namespace with label and annotation selectors.
// Annotations take higher priority. If annotations match, labels are ignored for that resource.
func (prf *PrFuncs) MultiSelectorList(ctx context.Context, namespace string, annotations interface{}, labels interface{}) ([]metav1.Object, error) {
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

	// Prepare options to list resources with the correct label selector
	options := metav1.ListOptions{}

	// Apply label selector if provided
	if labelSelector != "" {
		options.LabelSelector = labelSelector
	}

	// List PipelineRuns using the constructed label selector
	prsList, err := prf.client.TektonV1().PipelineRuns(namespace).List(ctx, options)
	if err != nil {
		return nil, err
	}

	// Filter by annotations first if annotations are provided
	var filteredPRs []metav1.Object
	if annotationSelector != "" {
		for _, pr := range prsList.Items {
			matches := true
			// Check annotations if the selector matches
			annotations := pr.GetAnnotations()
			for key, value := range annotations {
				if !strings.Contains(annotationSelector, key+"="+value) {
					matches = false
					break
				}
			}
			if matches {
				// If annotations match, include the resource and skip label filtering
				filteredPRs = append(filteredPRs, pr.DeepCopy())
			}
		}
	} else {
		// If no annotations are provided, apply label filtering
		for _, pr := range prsList.Items {
			if labelSelector == "" || config.MatchLabels(pr.GetLabels(), labelSelector) {
				filteredPRs = append(filteredPRs, pr.DeepCopy())
			}
		}
	}

	// Return the filtered list of PipelineRuns
	return filteredPRs, nil
}

// Get retrieves a specific PipelineRun by name in the given namespace.
func (prf *PrFuncs) Get(ctx context.Context, namespace, name string) (metav1.Object, error) {
	return prf.client.TektonV1().PipelineRuns(namespace).Get(ctx, name, metav1.GetOptions{})
}

// Delete removes a specific PipelineRun by name in the given namespace.
func (prf *PrFuncs) Delete(ctx context.Context, namespace, name string) error {
	return prf.client.TektonV1().PipelineRuns(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Update modifies an existing PipelineRun resource.
func (prf *PrFuncs) Update(ctx context.Context, resource metav1.Object) error {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return fmt.Errorf("invalid type received. Namespace:%s, Name:%s", resource.GetNamespace(), resource.GetName())
	}
	_, err := prf.client.TektonV1().PipelineRuns(resource.GetNamespace()).Update(ctx, pr, metav1.UpdateOptions{})
	return err
}

// GetCompletionTime retrieves the completion time of a PipelineRun resource.
func (prf *PrFuncs) GetCompletionTime(resource metav1.Object) (metav1.Time, error) {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return metav1.Time{}, fmt.Errorf("resource type error, this is not a PipelineRun resource. namespace:%s, name:%s, type:%T",
			resource.GetNamespace(), resource.GetName(), resource)
	}
	if pr.Status.CompletionTime != nil {
		return *pr.Status.CompletionTime, nil
	}
	for _, c := range pr.Status.Conditions {
		if c.Type == apis.ConditionSucceeded && c.Status != corev1.ConditionUnknown {
			finishAt := c.LastTransitionTime
			if finishAt.Inner.IsZero() {
				return metav1.Time{}, fmt.Errorf("unable to find the time when the resource '%s/%s' finished", pr.Namespace, pr.Name)
			}
			return c.LastTransitionTime.Inner, nil
		}
	}

	// This should never happen if the Resource has finished
	return metav1.Time{}, fmt.Errorf("unable to find the status of the finished resource: %s/%s", pr.Namespace, pr.Name)
}

// Ignore returns true if the resource should be ignored based on labels and annotations.
func (prf *PrFuncs) Ignore(resource metav1.Object) bool {
	// labels and annotations are not populated, lets wait sometime
	if resource.GetLabels() == nil {
		if resource.GetAnnotations() == nil || resource.GetAnnotations()[config.AnnotationTTLSecondsAfterFinished] == "" {
			return true
		}
	}
	return false
}

// IsCompleted checks if the PipelineRun resource has completed.
func (prf *PrFuncs) IsCompleted(resource metav1.Object) bool {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return false
	}

	if pr.Status.StartTime == nil {
		return false
	}

	if pr.Status.CompletionTime != nil {
		return true
	}

	if pr.IsPending() {
		return false
	}

	// check the status from conditions
	condition := pr.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil || condition.Status == corev1.ConditionUnknown {
		return false
	}

	return true
}

// IsSuccessful checks if the PipelineRun resource has successfully completed.
func (prf *PrFuncs) IsSuccessful(resource metav1.Object) bool {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return false
	}

	if pr.IsPending() {
		return false
	}

	condition := pr.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil {
		return false
	}

	runReason := pipelinev1.PipelineRunReason(condition.Reason)

	if runReason == pipelinev1.PipelineRunReasonSuccessful || runReason == pipelinev1.PipelineRunReasonCompleted {
		return true
	}

	return false
}

// IsFailed checks if the PipelineRun resource has failed.
func (prf *PrFuncs) IsFailed(resource metav1.Object) bool {
	pr, ok := resource.(*pipelinev1.PipelineRun)
	if !ok {
		return false
	}

	if pr.IsPending() {
		return false
	}

	return !prf.IsSuccessful(resource)
}

// GetDefaultLabelKey returns the default label key for PipelineRun resources.
func (prf *PrFuncs) GetDefaultLabelKey() string {
	return config.LabelPipelineName
}

// GetTTLSecondsAfterFinished retrieves the TTL (time-to-live) in seconds after a PipelineRun finishes.
func (prf *PrFuncs) GetTTLSecondsAfterFinished(namespace, pipelineName string, selectors config.SelectorSpec) *int32 {
	return config.PrunerConfigStore.GetPipelineTTLSecondsAfterFinished(namespace, pipelineName, selectors)
}

// GetSuccessHistoryLimitCount retrieves the success history limit count for a PipelineRun.
func (prf *PrFuncs) GetSuccessHistoryLimitCount(namespace, name string, selectors config.SelectorSpec) *int32 {
	return config.PrunerConfigStore.GetPipelineSuccessHistoryLimitCount(namespace, name, selectors)
}

// GetFailedHistoryLimitCount retrieves the failed history limit count for a PipelineRun.
func (prf *PrFuncs) GetFailedHistoryLimitCount(namespace, name string, selectors config.SelectorSpec) *int32 {
	return config.PrunerConfigStore.GetPipelineFailedHistoryLimitCount(namespace, name, selectors)
}

// GetEnforcedConfigLevel retrieves the enforced config level for a PipelineRun.
func (prf *PrFuncs) GetEnforcedConfigLevel(namespace, name string, selectors config.SelectorSpec) config.EnforcedConfigLevel {
	return config.PrunerConfigStore.GetPipelineEnforcedConfigLevel(namespace, name, selectors)
}
