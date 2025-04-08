package tektonpruner

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/system"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/pipelinerun"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/taskrun"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/version"
	pipelineversioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	clockUtil "k8s.io/utils/clock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
)

type TrFuncs struct {
	client pipelineversioned.Interface
}

// Type returns the kind of resource represented by the TaskRunFuncs struct, which is "TaskRun".
func (trf *TrFuncs) Type() string {
	return config.KindTaskRun
}

type PrFuncs struct {
	client pipelineversioned.Interface
}

// Type returns the kind of resource represented by the TaskRunFuncs struct, which is "TaskRun".
func (prf *PrFuncs) Type() string {
	return config.KindPipelineRun
}

// TTLHandler is responsible for managing resources with a Time-To-Live (TTL) configuration
type TTLHandler struct {
	clock      clockUtil.Clock // the clock for tracking time
	resourceFn config.TTLResourceFuncs
}

// NewController creates a Reconciler and returns the result of NewImpl.
// It also sets up a periodic garbage collection (GC) process that runs every 5 minutes.
// The GC process is responsible for cleaning up resources based on the TTL configuration.
// Additionally, it watches for changes to the ConfigMap and triggers GC immediately when a change is detected.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)

	logger.Info("Started Pruner controller")

	ver := version.Get()
	logger.Infow("pruner version details",
		"version", ver.Version, "arch", ver.Arch, "platform", ver.Platform,
		"goVersion", ver.GoLang, "buildDate", ver.BuildDate, "gitCommit", ver.GitCommit,
	)

	r := &Reconciler{
		kubeclient: kubeclient.Get(ctx),
	}

	impl := controller.NewContext(ctx, r, controller.ControllerOptions{
		Logger:        logger,
		WorkQueueName: "pruner",
	})

	// ConfigMap watcher triggers GC
	cmw.Watch(config.PrunerConfigMapName, func(cm *corev1.ConfigMap) {
		go safeRunGarbageCollector(ctx, logger)
	})

	// Periodic GC trigger
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		safeRunGarbageCollector(ctx, logger)

		for {
			select {
			case <-ctx.Done():
				logger.Info("Stopping periodic garbage collection")
				return
			case <-ticker.C:
				logger.Info("Periodic GC tick")
				safeRunGarbageCollector(ctx, logger)
			}
		}
	}()

	return impl
}

// safeRunGarbageCollector is a thread-safe wrapper around the garbage collection process.
func safeRunGarbageCollector(ctx context.Context, logger *zap.SugaredLogger) {
	var gcMutex sync.Mutex

	logger.Debug("Waiting to acquire GC lock")
	gcMutex.Lock()
	defer gcMutex.Unlock()

	logger.Info("Running garbage collector")
	runGarbageCollector(ctx)
	logger.Info("Garbage collection completed")
}

// runGarbageCollector performs the actual garbage collection process.
func runGarbageCollector(ctx context.Context) {
	logger := logging.FromContext(ctx)
	kubeClient := kubeclient.Get(ctx)

	taskRunFuncs := &TrFuncs{
		client: pipelineclient.Get(ctx),
	}

	prFuncs := &PrFuncs{
		client: pipelineclient.Get(ctx),
	}

	//Retreive the namespace where the controller is running. It is expected that the configMap with clusterwide settings will co-exist
	// in the same namespace as that of the controller.
	namespace := system.Namespace()

	// Load config from ConfigMap
	configMap, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, config.PrunerConfigMapName, metav1.GetOptions{})
	if err != nil {
		logger.Error("Failed to load ConfigMap for GC", zap.Error(err))
		return
	}

	if err := config.PrunerConfigStore.LoadGlobalConfig(ctx, configMap); err != nil {
		logger.Error("Error loading pruner global config", zap.Error(err))
		return
	}

	// Get filtered namespaces
	namespaces, err := getFilteredNamespaces(ctx, kubeClient)
	if err != nil {
		logger.Error("Failed to filter namespaces for GC", zap.Error(err))
		return
	}

	logger.Infow("Namespaces selected for garbage collection", "namespaces", namespaces)

	for _, ns := range namespaces {
		prs, err := prFuncs.PrListForGarbageCollection(ctx, ns, "completed")
		if err != nil {
			logger.Errorw("Error collecting PipelineRuns", zap.String("namespace", ns), zap.Error(err))
			continue
		}
		if len(prs) > 0 {
			logger.Infow("Pruned PipelineRuns", "namespace", ns, "count", len(prs))
		}

		trs, err := taskRunFuncs.TrListForGarbageCollection(ctx, ns, "completed")
		if err != nil {
			logger.Errorw("Error collecting TaskRuns", zap.String("namespace", ns), zap.Error(err))
			continue
		}
		if len(trs) > 0 {
			logger.Infow("Pruned TaskRuns", "namespace", ns, "count", len(trs))
		}
	}
}

// getFilteredNamespaces returns namespaces not starting with "kube" or "openshift"
func getFilteredNamespaces(ctx context.Context, client kubernetes.Interface) ([]string, error) {
	nsList, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var filtered []string
	for _, ns := range nsList.Items {
		name := ns.Name
		if !strings.HasPrefix(name, "kube") && !strings.HasPrefix(name, "openshift") && !strings.HasPrefix(name, "tekton") {
			filtered = append(filtered, name)
		}
	}
	return filtered, nil
}

// List returns a list of completed TaskRuns that are not owned by a PipelineRun in a given namespace.
func (trf *TrFuncs) TrListForGarbageCollection(ctx context.Context, namespace string, status string) ([]metav1.Object, error) {
	logger := logging.FromContext(ctx)
	tq := &TTLHandler{
		clock:      clockUtil.RealClock{},
		resourceFn: &taskrun.TrFuncs{},
	}

	trsList, err := trf.client.TektonV1().TaskRuns(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	if len(trsList.Items) > 0 {
		trsforgc := []metav1.Object{}
		trnames := []string{}
		for _, tr := range trsList.Items {
			if status == "completed" {
				resourceAnnotations := tr.GetAnnotations()
				resourceLabels := tr.GetLabels()
				// Construct the selectors with both matchLabels and matchAnnotations
				resourceSelectors := config.SelectorSpec{}
				if len(resourceAnnotations) > 0 {
					resourceSelectors.MatchAnnotations = resourceAnnotations
				}
				if len(resourceLabels) > 0 {
					resourceSelectors.MatchLabels = resourceLabels
				}
				// Check if the TaskRun is completed and not owned by a PipelineRun
				// and if the TTL has expired or limit threshold exceeded
				if tr.Status.CompletionTime != nil && !tr.HasPipelineRunOwnerReference() {

					ttlForResource, _ := tq.resourceFn.GetTTLSecondsAfterFinished(tr.GetNamespace(), tr.GetName(), resourceSelectors)
					if ttlForResource != nil {
						if tq.clock.Since(tr.Status.CompletionTime.Time) > time.Duration(*ttlForResource)*time.Second {
							trsforgc = append(trsforgc, tr.DeepCopy())
							trnames = append(trnames, tr.GetName())
						}
					}
				}
			}
			if len(trsforgc) > 0 {
				for _, eachtr := range trsforgc {
					err := trf.client.TektonV1().TaskRuns(namespace).Delete(ctx, eachtr.GetName(), metav1.DeleteOptions{})
					if err != nil {
						logger.Error("Failed to delete TaskRun", zap.Error(err))
						continue
					}
				}
			}
			logger.Debugw("TaskRun list", "namespace", namespace, "status", status, "trnames", trnames)
			return trsforgc, nil
		}
	}
	return nil, nil
}

// List returns a list of compledted PipelineRuns in a given namespace.
func (prf *PrFuncs) PrListForGarbageCollection(ctx context.Context, namespace string, status string) ([]metav1.Object, error) {
	logger := logging.FromContext(ctx)
	tq := &TTLHandler{
		clock:      clockUtil.RealClock{},
		resourceFn: &pipelinerun.PrFuncs{},
	}

	prsList, err := prf.client.TektonV1().PipelineRuns(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	if len(prsList.Items) > 0 {

		prsforgc := []metav1.Object{}
		prnames := []string{}
		for _, pr := range prsList.Items {
			if status == "completed" {
				if pr.Status.CompletionTime != nil {
					resourceAnnotations := pr.GetAnnotations()
					resourceLabels := pr.GetLabels()

					// Construct the selectors with both matchLabels and matchAnnotations
					resourceSelectors := config.SelectorSpec{}

					if len(resourceAnnotations) > 0 {
						resourceSelectors.MatchAnnotations = resourceAnnotations
					}

					if len(resourceLabels) > 0 {
						resourceSelectors.MatchLabels = resourceLabels
					}
					ttlForResource, _ := tq.resourceFn.GetTTLSecondsAfterFinished(pr.GetNamespace(), pr.GetName(), resourceSelectors)

					if ttlForResource != nil {
						if tq.clock.Since(pr.Status.CompletionTime.Time) > time.Duration(*ttlForResource)*time.Second {
							prsforgc = append(prsforgc, pr.DeepCopy())
							prnames = append(prnames, pr.GetName())
						}
					}
				}
			}
		}

		if len(prsforgc) > 0 {
			for _, eachPr := range prsforgc {
				err := prf.client.TektonV1().PipelineRuns(namespace).Delete(ctx, eachPr.GetName(), metav1.DeleteOptions{})
				if err != nil {
					logger.Error("Failed to delete PipelineRun", zap.Error(err))
					continue
				}
			}
		}
		logger.Debugw("PipelineRuns list", "namespace", namespace, "status", status, "prnames", prnames)
		return prsforgc, nil
	}
	return nil, nil
}

/*
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)

	logger.Info("Started Pruner controller")

	ver := version.Get()
	logger.Infow("pruner version details",
		"version", ver.Version, "arch", ver.Arch, "platform", ver.Platform,
		"goVersion", ver.GoLang, "buildDate", ver.BuildDate, "gitCommit", ver.GitCommit,
	)

	r := &Reconciler{
		kubeclient: kubeclient.Get(ctx),
	}

	impl := controller.NewContext(ctx, r, controller.ControllerOptions{
		Logger:        logger,
		WorkQueueName: "configmap-watcher",
	})

	// Watch for changes to the ConfigMap, and trigger the `onConfigChange` function
	cmw.Watch(config.PrunerConfigMapName, onConfigChange(ctx))

	return impl
}
*/

/*
// onConfigChange handles updates to the ConfigMap
func onConfigChange(ctx context.Context) configmap.Observer {
	logger := logging.FromContext(ctx)
	kubeClient := kubeclient.Get(ctx)

	taskRunFuncs := &TrFuncs{
		client: pipelineclient.Get(ctx),
	}

	prFuncs := &PrFuncs{
		client: pipelineclient.Get(ctx),
	}

	return func(configMap *corev1.ConfigMap) {
		logger.Infow("ConfigMap changed. Reloading pruner config...",
			"newGlobalConfig", configMap.Data[config.PrunerGlobalConfigKey],
		)

		// Load new config
		if err := config.PrunerConfigStore.LoadGlobalConfig(ctx, configMap); err != nil {
			logger.Error("Error loading pruner global config. Skipping GC.", zap.Error(err))
			return
		}

		// Get namespaces to prune
		namespaces, err := getFilteredNamespaces(ctx, kubeClient)
		if err != nil {
			logger.Error("Failed to filter namespaces. Skipping GC.", zap.Error(err))
			return
		}

		logger.Infow("Filtered namespaces for garbage collection", "namespaces", namespaces)

		for _, ns := range namespaces {
			// --- Garbage collect PipelineRuns ---
			pipelineRuns, err := prFuncs.PrListForGarbageCollection(ctx, ns, "completed")
			if err != nil {
				logger.Error("Failed to list PipelineRuns", zap.Error(err))
				continue
			}
			if len(pipelineRuns) > 0 {
				logger.Infow("Pruned PipelineRuns through Garbage collection", "namespace", ns, "count", len(pipelineRuns))
			}

			taskRuns, err := taskRunFuncs.TrListForGarbageCollection(ctx, ns, "completed")
			if err != nil {
				logger.Error("Failed to list TaskRuns", zap.Error(err))
				continue
			}
			if len(taskRuns) > 0 {
				logger.Infow("Pruneed TaskRuns through Garbage collection", "namespace", ns, "count", len(taskRuns))
			}
		}
	}
}
*/
