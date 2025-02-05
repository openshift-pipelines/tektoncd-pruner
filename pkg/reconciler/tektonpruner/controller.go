package tektonpruner

import (
	"context"

	"go.uber.org/zap"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	tektonprunerinformer "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/injection/informers/tektonpruner/v1alpha1/tektonpruner"
	tektonprunerreconciler "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/injection/reconciler/tektonpruner/v1alpha1/tektonpruner"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/version"
	corev1 "k8s.io/api/core/v1"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	// Obtain an informer to both the main and child resources. These will be started by
	// the injection framework automatically. They'll keep a cached representation of the
	// cluster's state of the respective resource at all times.
	tektonPrunerInformer := tektonprunerinformer.Get(ctx)

	logger := logging.FromContext(ctx)
	ver := version.Get()
	// print version details
	logger.Infow("pruner version details",
		"version", ver.Version, "arch", ver.Arch, "platform", ver.Platform,
		"goVersion", ver.GoLang, "buildDate", ver.BuildDate, "gitCommit", ver.GitCommit,
	)

	r := &Reconciler{
		// The client will be needed to create/delete Pods via the API.
		kubeclient: kubeclient.Get(ctx),
	}

	impl := tektonprunerreconciler.NewImpl(ctx, r)

	// Listen for events on the main resource and enqueue themselves.
	tektonPrunerInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	// call
	cmw.Watch(helper.PrunerConfigMapName, onConfigChange(ctx))

	return impl
}

func onConfigChange(ctx context.Context) configmap.Observer {
	logger := logging.FromContext(ctx)
	return func(configMap *corev1.ConfigMap) {
		logger.Info("updating pruner global config map with pruner config store",
			"newGlobalConfig", configMap.Data[helper.PrunerGlobalConfigKey],
		)
		err := helper.PrunerConfigStore.LoadGlobalConfig(configMap)
		if err != nil {
			logger.Error("error on getting pruner global config", zap.Error(err))
		}
	}
}
