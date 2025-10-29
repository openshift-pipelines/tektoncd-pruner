package namespaceprunerconfig

import (
	"context"

	"github.com/tektoncd/pruner/pkg/config"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	configmapinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/configmap"
	namespaceinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/namespace"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)
	logger.Info("Starting Namespace Pruner Config controller")

	kubeClient := kubeclient.Get(ctx)
	configMapInformer := configmapinformer.Get(ctx)
	namespaceInformer := namespaceinformer.Get(ctx)

	r := &Reconciler{
		kubeclient: kubeClient,
	}

	impl := controller.NewContext(ctx, r, controller.ControllerOptions{
		Logger:        logger,
		WorkQueueName: "namespace-pruner-config",
	})

	// Add event handler to watch ConfigMaps with the specified name across filtered namespaces
	// The informer will only watch namespaces specified via injection.WithNamespaceScope()
	_, err := configMapInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			// Only react to ConfigMaps with the namespace-level pruner config name
			return cm.Name == config.PrunerNamespaceConfigMapName
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { impl.Enqueue(obj) },
			UpdateFunc: func(oldObj, newObj interface{}) { impl.Enqueue(newObj) },
			DeleteFunc: func(obj interface{}) { impl.Enqueue(obj) },
		},
	})
	if err != nil {
		logger.Fatal("Failed to add ConfigMap event handler", zap.Error(err))
	}

	// Watch for namespace deletions to clean up orphaned config entries
	// This prevents memory leaks when namespaces are deleted
	_, err = namespaceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			ns, ok := obj.(*corev1.Namespace)
			if !ok {
				// Handle tombstone case (object was deleted but informer hasn't caught up)
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					logger.Warnw("Failed to decode namespace deletion event", "object", obj)
					return
				}
				ns, ok = tombstone.Obj.(*corev1.Namespace)
				if !ok {
					logger.Warnw("Tombstone contained unexpected object", "object", tombstone.Obj)
					return
				}
			}

			// Clean up namespace config from the store when namespace is deleted
			logger.Infow("Namespace deleted, cleaning up config", "namespace", ns.Name)
			config.PrunerConfigStore.DeleteNamespaceConfig(ctx, ns.Name)
		},
	})
	if err != nil {
		logger.Fatal("Failed to add Namespace event handler", zap.Error(err))
	}

	return impl
}
