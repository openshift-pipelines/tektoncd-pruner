package tektonpruner

import (
	"context"

	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/logging"
)

// Reconciler includes the kubernetes client to interact with the cluster
type Reconciler struct {
	kubeclient kubernetes.Interface
}

// Reconcile is the method that will be called when resources change
func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)

	// Example logic: log the key of the changed resource (you can replace this with your custom logic)
	logger.Infof("Reconcile called for key: %s", key)

	// Nothing to reconcile at the moment. We are just updating the configstore

	return nil // Return nil to indicate successful reconciliation
}
