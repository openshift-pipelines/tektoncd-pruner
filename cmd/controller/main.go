package main

import (
	"flag"
	"strings"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/pipelinerun"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/taskrun"
	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/tektonpruner"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/signals"
)

// main function of the program
func main() {
	// Define command-line flags
	flag.IntVar(&controller.DefaultThreadsPerController, "threads-per-controller", controller.DefaultThreadsPerController, "Threads (goroutines) to create per controller")
	namespace := flag.String("namespace", corev1.NamespaceAll, "Namespace to restrict informer to. Optional, defaults to all namespaces.")
	disableHighAvailability := flag.Bool("disable-ha", true, "Whether to disable high-availability functionality for this component.")
	flag.Parse()

	// Parse and get REST config
	cfg := injection.ParseAndGetRESTConfigOrDie()

	// Set QPS and Burst settings
	if cfg.QPS == 0 {
		cfg.QPS = 2 * rest.DefaultQPS
	}
	if cfg.Burst == 0 {
		cfg.Burst = rest.DefaultBurst
	}

	// Multiply by 2 for number of controllers
	cfg.QPS = 2 * cfg.QPS
	cfg.Burst = 2 * cfg.Burst

	// Set up logging
	ctx := signals.NewContext()
	logger := logging.FromContext(ctx)

	// Add namespaces
	var namespaces []string
	if *namespace != "" {
		namespaces = strings.Split(strings.ReplaceAll(*namespace, " ", ""), ",")
		logger.Infof("controller is scoped to the following namespaces: %s\n", namespaces)
	}

	// Add High Availability flag
	if *disableHighAvailability {
		ctx = sharedmain.WithHADisabled(ctx)
	}

	// Use sharedmain to handle controller lifecycle
	sharedmain.MainWithConfig(ctx, "tekton-pruner-controller", cfg,
		tektonpruner.NewController,
		pipelinerun.NewController,
		taskrun.NewController,
	)
}
