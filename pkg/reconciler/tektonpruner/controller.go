package tektonpruner

import (
	"context"

	"go.uber.org/zap"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/version"
	corev1 "k8s.io/api/core/v1"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	// Obtain a logger from context
	logger := logging.FromContext(ctx)

	logger.Info("Started Pruner controller")
	// Print version details for the controller
	ver := version.Get()
	logger.Infow("pruner version details",
		"version", ver.Version, "arch", ver.Arch, "platform", ver.Platform,
		"goVersion", ver.GoLang, "buildDate", ver.BuildDate, "gitCommit", ver.GitCommit,
	)

	// Reconciler only needs to watch ConfigMap changes, not Tekton resources
	r := &Reconciler{
		kubeclient: kubeclient.Get(ctx),
	}

	//logger = logger.Named("configmap-watcher")

	impl := controller.NewContext(ctx, r, controller.ControllerOptions{
		Logger:        logger,
		WorkQueueName: "configmap-watcher",
	})
	// Watch for changes to the ConfigMap, and trigger the `onConfigChange` function
	cmw.Watch(config.PrunerConfigMapName, onConfigChange(ctx))

	return impl
}

func onConfigChange(ctx context.Context) configmap.Observer {
	logger := logging.FromContext(ctx)
	logger.Info("Observer triggered")
	return func(configMap *corev1.ConfigMap) {
		logger.Debugw("updating pruner global config map with pruner config store",
			"newGlobalConfig", configMap.Data[config.PrunerGlobalConfigKey],
		)
		err := config.PrunerConfigStore.LoadGlobalConfig(ctx, configMap)
		if err != nil {
			logger.Error("error on getting pruner global config", zap.Error(err))
		}
	}
}
