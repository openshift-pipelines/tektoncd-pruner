package taskrun

import (
	"context"
	"os"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1/taskrun"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/taskrun"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	// Obtain an informer to both the main and child resources. These will be started by
	// the injection framework automatically. They'll keep a cached representation of the
	// cluster's state of the respective resource at all times.
	taskRunInformer := taskruninformer.Get(ctx)

	logger := logging.FromContext(ctx)

	taskRunFuncs := &TrFuncs{
		client: pipelineclient.Get(ctx),
	}
	ttlHandler, err := config.NewTTLHandler(clock.RealClock{}, taskRunFuncs)
	if err != nil {
		logger.Fatal("error on getting ttl handler", zap.Error(err))
	}

	historyLimiter, err := config.NewHistoryLimiter(taskRunFuncs)
	if err != nil {
		logger.Fatal("error on getting history limiter", zap.Error(err))
	}

	r := &Reconciler{
		// The client will be needed to create/delete Pods via the API.
		kubeclient:     kubeclient.Get(ctx),
		ttlHandler:     ttlHandler,
		historyLimiter: historyLimiter,
	}

	// number of works to process the events
	concurrentWorkers, err := config.GetEnvValueAsInt(config.EnvTTLConcurrentWorkersTaskRun, config.DefaultTTLConcurrentWorkersTaskRun)
	if err != nil {
		logger.Fatalw("error on getting TaskRun ttl concurrent workers count",
			"environmentKey", config.EnvTTLConcurrentWorkersTaskRun, "environmentValue", os.Getenv(config.EnvTTLConcurrentWorkersTaskRun),
			zap.Error(err),
		)
	}

	ctrlOptions := controller.Options{
		Concurrency: concurrentWorkers,
	}

	impl := taskrunreconciler.NewImpl(ctx, r, func(impl *controller.Impl) controller.Options { return ctrlOptions })

	_, err = taskRunInformer.Informer().AddEventHandler(controller.HandleAll(filterTaskRun(logger, impl)))
	if err != nil {
		logger.Fatal("Failed to add event handler", zap.Error(err))
	}

	return impl
}

// filters the taskrun which has a parent
func filterTaskRun(logger *zap.SugaredLogger, impl *controller.Impl) func(obj interface{}) {
	return func(obj interface{}) {
		taskRun, err := kmeta.DeletionHandlingAccessor(obj)
		if err != nil {
			logger.Errorw("error on getting object as Accessor", zap.Error(err))
			return
		}

		if !isStandaloneTaskRun(taskRun) {
			return
		}

		impl.EnqueueKey(types.NamespacedName{Namespace: taskRun.GetNamespace(), Name: taskRun.GetName()})
	}
}

// returns true if the TaskRun is part of a PipelineRun
func isStandaloneTaskRun(taskRun metav1.Object) bool {
	// verify the taskRun is not part of a pipelineRun
	if taskRun.GetLabels() != nil && taskRun.GetLabels()[config.LabelPipelineRunName] != "" {
		return false
	}

	// if the resource has owner reference as PipelineRun, it is not a standalone TaskRun
	// if so, ignore this taskRun
	if len(taskRun.GetOwnerReferences()) > 0 {
		for _, ownerReference := range taskRun.GetOwnerReferences() {
			if ownerReference.Kind == config.KindPipelineRun {
				return false
			}
		}
	}

	return true
}
