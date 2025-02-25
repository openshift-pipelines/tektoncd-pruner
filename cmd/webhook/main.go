package main

import (
	"context"
	"fmt"
	"os"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/metrics"
	"knative.dev/pkg/signals"
	"knative.dev/pkg/webhook"
	"knative.dev/pkg/webhook/certificates"
	"knative.dev/pkg/webhook/configmaps"
)

func newConfigValidationController(name string) func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		return configmaps.NewAdmissionController(ctx,

			// Name of the configmap webhook, it is based on the value of the environment variable WEBHOOK_ADMISSION_CONTROLLER_NAME
			// default is "config.webhook.pruner.tekton.dev"
			fmt.Sprintf("config.%s", name),

			// The path on which to serve the webhook.
			"/config-validation",

			// The configmaps to validate.
			configmap.Constructors{
				logging.ConfigMapName(): logging.NewConfigFromConfigMap,
				metrics.ConfigMapName(): metrics.NewObservabilityConfigFromConfigMap,
			},
		)
	}
}

func main() {

	serviceName := os.Getenv("WEBHOOK_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "tekton-pruner-webhook"
	}

	secretName := os.Getenv("WEBHOOK_SECRET_NAME")
	if secretName == "" {
		secretName = "tekton-pruner-webhook-certs"
	}

	webhookName := os.Getenv("WEBHOOK_ADMISSION_CONTROLLER_NAME")
	if webhookName == "" {
		webhookName = "webhook.pruner.tekton.dev"
	}

	ctx := webhook.WithOptions(signals.NewContext(), webhook.Options{
		ServiceName: serviceName,
		SecretName:  secretName,
		Port:        webhook.PortFromEnv(8443),
	})

	sharedmain.MainWithContext(ctx, serviceName,
		certificates.NewController,
		newConfigValidationController(webhookName),
	)
}
