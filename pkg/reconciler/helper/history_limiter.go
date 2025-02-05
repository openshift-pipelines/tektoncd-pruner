package helper

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	tektonprunerv1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
)

type HistoryLimiterResourceFuncs interface {
	Type() string
	Get(ctx context.Context, namespace, name string) (metav1.Object, error)
	Update(ctx context.Context, resource metav1.Object) error
	Delete(ctx context.Context, namespace, name string) error
	List(ctx context.Context, namespace, label string) ([]metav1.Object, error)
	GetFailedHistoryLimitCount(namespace, name string) *int32
	GetSuccessHistoryLimitCount(namespace, name string) *int32
	IsSuccessful(resource metav1.Object) bool
	IsFailed(resource metav1.Object) bool
	IsCompleted(resource metav1.Object) bool
	GetDefaultLabelKey() string
	GetEnforcedConfigLevel(namespace, name string) tektonprunerv1alpha1.EnforcedConfigLevel
}

type HistoryLimiter struct {
	resourceFn HistoryLimiterResourceFuncs
}

func NewHistoryLimiter(resourceFn HistoryLimiterResourceFuncs) (*HistoryLimiter, error) {
	hl := &HistoryLimiter{
		resourceFn: resourceFn,
	}
	if hl.resourceFn == nil {
		return nil, fmt.Errorf("resourceFunc interface can not be nil")
	}

	return hl, nil
}

func (hl *HistoryLimiter) ProcessEvent(ctx context.Context, resource metav1.Object) error {
	logger := logging.FromContext(ctx)
	logger.Debugw("processing an event",
		"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
	)

	// if the resource is on deletion state, no action needed
	if resource.GetDeletionTimestamp() != nil {
		logger.Debugw("resource is in deletion state",
			"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
		)
		return nil
	}

	if hl.isProcessed(resource) {
		logger.Debugw("already processed",
			"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
		)
		return nil
	}

	// if the resource is still in running state, ignore it
	if !hl.resourceFn.IsCompleted(resource) {
		logger.Debugw("resource is not in completion state",
			"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
		)
		return nil
	}

	defer hl.markAsProcessed(ctx, resource)

	if hl.resourceFn.IsSuccessful(resource) {
		return hl.doSuccessfulResourceCleanup(ctx, resource)
	}

	if hl.resourceFn.IsFailed(resource) {
		return hl.doFailedResourceCleanup(ctx, resource)
	}

	return nil
}

// adds an annotation, indicates this resource is already processed
// no action needed on the further reconcile loop for this Resource
func (hl *HistoryLimiter) markAsProcessed(ctx context.Context, resource metav1.Object) {
	logger := logging.FromContext(ctx)
	logger.Debugw("marking as resource as processed",
		"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
	)
	// if user sets the history limit to 0, there is no Resource will be retained
	// hence, fetch the resource and if available update 'mark as processed'
	resourceLatest, err := hl.resourceFn.Get(ctx, resource.GetNamespace(), resource.GetName())
	if err != nil {
		if errors.IsNotFound(err) {
			return
		}
		logger.Errorw("error on getting a resource",
			"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
			zap.Error(err),
		)
		return
	}

	processedTimeAsString := time.Now().Format(time.RFC3339)
	annotations := resourceLatest.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationHistoryLimitCheckProcessed] = processedTimeAsString
	resourceLatest.SetAnnotations(annotations)
	err = hl.resourceFn.Update(ctx, resourceLatest)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Errorw("error on updating 'mark as processed' on a resource",
			"resource", hl.resourceFn.Type(), "namespace", resourceLatest.GetNamespace(), "name", resourceLatest.GetName(),
			zap.Error(err),
		)
	}

}

func (hl *HistoryLimiter) isProcessed(resource metav1.Object) bool {
	annotations := resource.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, found := annotations[AnnotationHistoryLimitCheckProcessed]
	return found
}

func (hl *HistoryLimiter) doSuccessfulResourceCleanup(ctx context.Context, resource metav1.Object) error {
	return hl.doResourceCleanup(ctx, resource, AnnotationSuccessfulHistoryLimit, hl.resourceFn.GetSuccessHistoryLimitCount, hl.isSuccessfulResource)
}

func (hl *HistoryLimiter) doFailedResourceCleanup(ctx context.Context, resource metav1.Object) error {
	return hl.doResourceCleanup(ctx, resource, AnnotationFailedHistoryLimit, hl.resourceFn.GetFailedHistoryLimitCount, hl.isFailedResource)
}

func (hl *HistoryLimiter) isFailedResource(resource metav1.Object) bool {
	return hl.resourceFn.IsCompleted(resource) && hl.resourceFn.IsFailed(resource)
}

func (hl *HistoryLimiter) isSuccessfulResource(resource metav1.Object) bool {
	return hl.resourceFn.IsCompleted(resource) && hl.resourceFn.IsSuccessful(resource)
}

func (hl *HistoryLimiter) doResourceCleanup(ctx context.Context, resource metav1.Object, historyLimitAnnotation string, getHistoryLimitFn func(string, string) *int32, getResourceFilterFn func(metav1.Object) bool) error {
	logger := logging.FromContext(ctx)

	labelKey := getResourceNameLabelKey(resource, hl.resourceFn.GetDefaultLabelKey())
	resourceName := getResourceName(resource, labelKey)

	// can not proceed without labelKey or resourceName
	if labelKey == "" || resourceName == "" {
		return nil
	}

	enforcedConfigLevel := hl.resourceFn.GetEnforcedConfigLevel(resource.GetNamespace(), resourceName)
	var historyLimit *int32
	// check the limit history from the resource annotations
	annotations := resource.GetAnnotations()
	// if the "enforceConfigLevel" is not resource level, do not take limit from resource annotations
	// take it from namespace config or global config
	if enforcedConfigLevel == tektonprunerv1alpha1.EnforcedConfigLevelResource && len(annotations) != 0 && annotations[historyLimitAnnotation] != "" {
		_limit, err := strconv.Atoi(annotations[historyLimitAnnotation])
		if err != nil {
			logger.Errorw("error on converting history limit to int",
				"resource", hl.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(),
				"historyLimitAnnotation", historyLimitAnnotation, "historyLimitValue", annotations[historyLimitAnnotation],
				zap.Error(err),
			)
			return err
		}
		historyLimit = ptr.Int32(int32(_limit))
	} else {
		// update from namespace or from global-config, if present
		historyLimit = getHistoryLimitFn(resource.GetNamespace(), resourceName)
	}

	// if there is not limit present, or in negative value, do not delete
	if historyLimit == nil || *historyLimit < 0 {
		return nil
	}

	// get resource list with a label filter
	label := fmt.Sprintf("%s=%s", labelKey, resourceName)
	resources, err := hl.resourceFn.List(ctx, resource.GetNamespace(), label)
	if err != nil {
		return err
	}

	// if the resource is within the count, no action is needed
	if int(*historyLimit) > len(resources) {
		return nil
	}

	// filter only completed resources
	resourcesFiltered := []metav1.Object{}
	for _, res := range resources {
		if getResourceFilterFn(res) {
			resourcesFiltered = append(resourcesFiltered, res)
		}
	}
	// add the filtered result into resources
	resources = resourcesFiltered

	// recheck the count after filtered
	// if the resource is within the count, no action is needed
	if int(*historyLimit) > len(resources) {
		return nil
	}

	slices.SortStableFunc(resources, func(a, b metav1.Object) int {
		objA := a.GetCreationTimestamp()
		objB := b.GetCreationTimestamp()
		// sort by reverse order
		// order by, newer to older...
		if objA.Time.Before(objB.Time) {
			return 1
		} else if objA.Time.After(objB.Time) {
			return -1
		}
		return 0
	})

	var selectionForDeletion []metav1.Object

	if *historyLimit == 0 {
		// remove all the history
		selectionForDeletion = resources
	} else {
		selectionForDeletion = resources[*historyLimit:]
	}

	for _, _res := range selectionForDeletion {
		logger.Debugw("deleting a resource",
			"resource", hl.resourceFn.Type(), "namespace", _res.GetNamespace(), "name", _res.GetName(),
			"resourceCreationTimestamp", _res.GetCreationTimestamp(),
		)
		err := hl.resourceFn.Delete(ctx, _res.GetNamespace(), _res.GetName())
		if err != nil {
			// ignore the error, if the resource is not found
			if errors.IsNotFound(err) {
				return nil
			}
			logger.Errorw("error on removing a resource",
				"resource", hl.resourceFn.Type(), "namespace", _res.GetNamespace(), "name", _res.GetName(),
				zap.Error(err),
			)
		}
	}

	return nil
}
