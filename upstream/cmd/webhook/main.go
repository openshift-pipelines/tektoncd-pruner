/*
Copyright 2025 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"

	"github.com/tektoncd/pruner/pkg/webhook"
	"k8s.io/client-go/tools/cache"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	secretinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/secret"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/signals"
	"knative.dev/pkg/system"
	pkgwebhook "knative.dev/pkg/webhook"
	"knative.dev/pkg/webhook/certificates"
)

func main() {
	// Create signal context
	ctx := signals.NewContext()

	// Disable leader election for stateless webhook
	ctx = sharedmain.WithHADisabled(ctx)

	ctx = pkgwebhook.WithOptions(ctx, pkgwebhook.Options{
		ServiceName: "tekton-pruner-webhook",
		Port:        pkgwebhook.PortFromEnv(8443),
		SecretName:  pkgwebhook.SecretNameFromEnv("tekton-pruner-webhook-certs"),
	})

	// Start webhook server with certificate controller
	// The certificate controller ensures the webhook-certs secret exists and injects the CA bundle into webhook configurations
	sharedmain.MainWithContext(ctx, "pruner-webhook",
		certificates.NewController,
		NewConfigMapValidationWebhook,
	)
}

// create the webhook admission controller
func NewConfigMapValidationWebhook(ctx context.Context, _ configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)
	logger.Info("Setting up Pruner ConfigMap validation webhook")

	// Get webhook options for secret name
	opts := pkgwebhook.GetOptions(ctx)
	client := kubeclient.Get(ctx)
	secretInformer := secretinformer.Get(ctx)

	webhookName := "validation.webhook.pruner.tekton.dev"

	// Create the admission controller with client and configuration
	wh := &webhook.ValidateConfigMap{
		Client:      client,
		SecretName:  opts.SecretName,
		WebhookName: webhookName,
	}

	// Create the controller
	c := controller.NewContext(ctx, wh, controller.ControllerOptions{
		Logger:        logger,
		WorkQueueName: "configmap-validation",
	})

	// Watch the secret for changes to trigger reconciliation
	if _, err := secretInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterWithNameAndNamespace(system.Namespace(), opts.SecretName),
		Handler:    controller.HandleAll(c.Enqueue),
	}); err != nil {
		logger.Fatalw("Failed to add event handler for secret informer", "error", err)
	}

	return c
}
