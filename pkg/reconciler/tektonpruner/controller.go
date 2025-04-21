package tektonpruner

import (
	"context"
	"encoding/json"
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
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"

	clockUtil "k8s.io/utils/clock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
)

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

	return impl
}

// safeRunGarbageCollector is a thread-safe wrapper around the garbage collection process.
func safeRunGarbageCollector(ctx context.Context, logger *zap.SugaredLogger) {
	var gcMutex sync.Mutex

	logger.Debug("Waiting to acquire cleanup thread lock")
	gcMutex.Lock()
	defer gcMutex.Unlock()

	logger.Info("Running Cleanup")
	runGarbageCollector(ctx)
	logger.Info("Cleanup thread completed")
}

func runGarbageCollector(ctx context.Context) {
	logger := logging.FromContext(ctx)
	kubeClient := kubeclient.Get(ctx)

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

	configMapUpdateTime := time.Now().Format(time.RFC3339)

	// Get filtered namespaces
	namespaces, err := getFilteredNamespaces(ctx, kubeClient)
	if err != nil {
		logger.Error("Failed to filter namespaces for GC", zap.Error(err))
		return
	}

	logger.Infow("Namespaces selected for garbage collection", "namespaces", namespaces)

	// Get worker count from config or default to 5
	workerCount, err := config.PrunerConfigStore.WorkerCount(ctx, configMap)
	if err != nil {
		logger.Error("Failed to get worker count from config", zap.Error(err))
		workerCount = config.DefaultWorkerCountForNamespaceCleanup
	}

	// Setup channels
	nsChan := make(chan string)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for ns := range nsChan {
				logger.Infow("Worker processing namespace", "worker", workerID, "namespace", ns)

				if err := cleanupPRs(ctx, ns, configMapUpdateTime); err != nil {
					logger.Errorw("Error collecting PipelineRuns", zap.String("namespace", ns), zap.Error(err))
					continue
				}
				if err := cleanupTRs(ctx, ns, configMapUpdateTime); err != nil {
					logger.Errorw("Error collecting TaskRuns", zap.String("namespace", ns), zap.Error(err))
					continue
				}
			}
		}(i)
	}

	// Send namespaces to workers
	for _, ns := range namespaces {
		nsChan <- ns
	}
	close(nsChan)

	wg.Wait()
	logger.Info("Garbage collection completed")
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

// CleanupPRs is responsible for cleaning up completed PipelineRuns based on their TTL and history limit.
func cleanupPRs(ctx context.Context, namespace string, configMapUpdateTime string) error {

	logger := logging.FromContext(ctx)
	logger.Debugw("Start Cleanup PipelineRuns", "namespace", namespace)

	pipelineClient := pipelineclient.Get(ctx)
	prFuncs := pipelinerun.NewPrFuncs(pipelineClient)

	prTTLHandler, err := config.NewTTLHandler(clockUtil.RealClock{}, prFuncs)
	if err != nil {
		logger.Fatal("error on getting ttl handler", zap.Error(err))
	}

	prHistoryLimiter, err := config.NewHistoryLimiter(prFuncs)
	if err != nil {
		logger.Fatal("error on getting history limiter", zap.Error(err))
	}

	prsList, err := pipelineClient.TektonV1().PipelineRuns(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	logger.Debugw("Progressing cleanup PipelineRuns list", "list", prsList.Items, "namespace", namespace)

	if len(prsList.Items) > 0 {

		for _, prInstance := range prsList.Items {
			logger.Debugw("Processing PipelineRun", "name", prInstance.Name, "namespace", prInstance.Namespace)
			// Check if the PipelineRun is completed
			if prInstance.Status.CompletionTime != nil {
				pr := &prInstance

				// Check if the history limit processed time which is stored as a string in annotation of PR config.AnnotationHistoryLimitCheckProcessed is not nil
				// and earlier than the configmap update time
				if prInstance.Annotations[config.AnnotationHistoryLimitCheckProcessed] != "" {
					// Parse the annotation value to a time.Time object
					annotationTime, err := time.Parse(time.RFC3339, prInstance.Annotations[config.AnnotationHistoryLimitCheckProcessed])
					if err != nil {
						logger.Errorw("error parsing history limit check processed time", "namespace", pr.Namespace, "name", pr.Name, zap.Error(err))
						return err
					}
					// Compare the annotation time with the configmap update time
					// If the configmap update time is after the annotation time, remove the annotation and patch the PipelineRun
					// to trigger the history limit check again

					updateTime, err := time.Parse(time.RFC3339, configMapUpdateTime)
					if err != nil {
						logger.Errorw("error parsing configmap update time", "namespace", pr.Namespace, "name", pr.Name, zap.Error(err))
						return err
					}

					if updateTime.After(annotationTime) {
						// If the configmap update time is after the annotation time, remove the annotation and patch the PipelineRun
						annotations := pr.GetAnnotations()
						delete(annotations, config.AnnotationHistoryLimitCheckProcessed)
						// Create a patch with the new annotations
						patchData := map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": annotations,
							},
						}

						// Convert patchData to JSON
						patchBytes, err := json.Marshal(patchData)
						if err != nil {
							logger.Errorw("error marshaling patch data", zap.Error(err))
							return err
						}

						// Patch the PipelineRun to remove the annotation
						_, err = pipelineClient.TektonV1().PipelineRuns(pr.Namespace).Patch(ctx, pr.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
						if err != nil {
							logger.Errorw("error patching PipelineRun to remove history limit check processed annotation", "namespace", pr.Namespace, "name", pr.Name, zap.Error(err))
							return err
						}
					}
				}

				err := prHistoryLimiter.ProcessEvent(ctx, pr)
				if err != nil {
					logger.Errorw("error processing history limiting for a PipelineRun", "namespace", pr.Namespace, "name", pr.Name, zap.Error(err))
					return err
				}
				// execute ttl handler
				err = prTTLHandler.ProcessEvent(ctx, pr)
				if err != nil {
					isRequeueKey, _ := controller.IsRequeueKey(err)
					// the error is not a requeue error, print the error
					if !isRequeueKey {
						data, _ := json.Marshal(pr)
						logger.Errorw("error processing ttl for a PipelineRun", "namespace", pr.Namespace, "name", pr.Name, "resource", string(data), zap.Error(err))
					}
					return err
				}
			}

		}
	}
	return nil
}

// CleanupTRs is responsible for cleaning up completed TaskRuns based on their TTL and history limit.
// It checks if the TaskRun has a completion time and is not owned by a PipelineRun before processing.
func cleanupTRs(ctx context.Context, namespace string, configMapUpdateTime string) error {

	logger := logging.FromContext(ctx)
	logger.Debugw("Start Cleanup TaskRuns", "namespace", namespace)

	pipelineClient := pipelineclient.Get(ctx)
	trFuncs := taskrun.NewTrFuncs(pipelineClient)

	trTTLHandler, err := config.NewTTLHandler(clockUtil.RealClock{}, trFuncs)
	if err != nil {
		logger.Fatal("error on getting ttl handler", zap.Error(err))
	}

	trHistoryLimiter, err := config.NewHistoryLimiter(trFuncs)
	if err != nil {
		logger.Fatal("error on getting history limiter", zap.Error(err))
	}

	trsList, err := pipelineClient.TektonV1().TaskRuns(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	if len(trsList.Items) > 0 {

		for _, trInstance := range trsList.Items {
			if trInstance.Status.CompletionTime != nil && !trInstance.HasPipelineRunOwnerReference() {
				tr := &trInstance

				// Check if the history limit processed time which is stored as a string in annotation of PR config.AnnotationHistoryLimitCheckProcessed is not nil
				// and earlier than the configmap update time
				if trInstance.Annotations[config.AnnotationHistoryLimitCheckProcessed] != "" {
					// Parse the annotation value to a time.Time object
					annotationTime, err := time.Parse(time.RFC3339, trInstance.Annotations[config.AnnotationHistoryLimitCheckProcessed])
					if err != nil {
						logger.Errorw("error parsing history limit check processed time", "namespace", tr.Namespace, "name", tr.Name, zap.Error(err))
						return err
					}
					// Compare the annotation time with the configmap update time
					// If the configmap update time is after the annotation time, remove the annotation and patch the TaskRun
					// to trigger the history limit check again

					updateTime, err := time.Parse(time.RFC3339, configMapUpdateTime)
					// If the configmap update time is after the annotation time, remove the annotation and patch the TaskRun

					if updateTime.After(annotationTime) {
						// If the configmap update time is after the annotation time, remove the annotation and patch the TaskRun
						annotations := tr.GetAnnotations()
						delete(annotations, config.AnnotationHistoryLimitCheckProcessed)
						// Create a patch with the new annotations
						patchData := map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": annotations,
							},
						}

						// Convert patchData to JSON
						patchBytes, err := json.Marshal(patchData)
						if err != nil {
							logger.Errorw("error marshaling patch data", zap.Error(err))
							return err
						}

						// Patch the PipelineRun to remove the annotation
						_, err = pipelineClient.TektonV1().TaskRuns(tr.Namespace).Patch(ctx, tr.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
						if err != nil {
							logger.Errorw("error patching PipelineRun to remove history limit check processed annotation", "namespace", tr.Namespace, "name", tr.Name, zap.Error(err))
							return err
						}
					}
				}

				err := trHistoryLimiter.ProcessEvent(ctx, tr)
				if err != nil {
					logger.Errorw("error processing history limiting for a TaskRun", "namespace", tr.Namespace, "name", tr.Name, zap.Error(err))
					return err
				}
				// execute ttl handler
				err = trTTLHandler.ProcessEvent(ctx, tr)
				if err != nil {
					isRequeueKey, _ := controller.IsRequeueKey(err)
					// the error is not a requeue error, print the error
					if !isRequeueKey {
						data, _ := json.Marshal(tr)
						logger.Errorw("error processing ttl for a TaskRun", "namespace", tr.Namespace, "name", tr.Name, "resource", string(data), zap.Error(err))
					}
					return err
				}
			}

		}
	}
	return nil
}
