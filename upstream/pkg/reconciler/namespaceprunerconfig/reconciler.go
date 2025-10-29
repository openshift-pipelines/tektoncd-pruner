package namespaceprunerconfig

import (
	"context"
	"strings"

	"github.com/tektoncd/pruner/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// Reconciler implements controller.Reconciler for NamespacePrunerConfig resources
type Reconciler struct {
	kubeclient kubernetes.Interface
}

// Reconcile is called when a ConfigMap with name "tekton-pruner-namespace-spec" changes
func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)
	logger.Debugf("Reconciling namespace pruner config: %s", key)

	// Parse the key to get namespace and name
	namespace, name, err := parseKey(key)
	if err != nil {
		logger.Errorf("Invalid key: %s", key)
		return err
	}

	// Get the ConfigMap
	cm, err := r.kubeclient.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// If the ConfigMap is not found, it was deleted
		if errors.IsNotFound(err) {
			logger.Infof("Namespace ConfigMap deleted: %s/%s", namespace, name)
			config.PrunerConfigStore.DeleteNamespaceConfig(ctx, namespace)
			return nil
		}
		logger.Errorf("Failed to get ConfigMap %s/%s: %v", namespace, name, err)
		return err
	}

	// Load the namespace config
	logger.Infof("Loading namespace config from ConfigMap: %s/%s", namespace, name)
	if err := config.PrunerConfigStore.LoadNamespaceConfig(ctx, namespace, cm); err != nil {
		logger.Errorf("Failed to load namespace config from ConfigMap %s/%s: %v", namespace, name, err)
		return err
	}

	logger.Infof("Successfully loaded namespace config: %s/%s", namespace, name)
	return nil
}

// parseKey parses the key in the format "namespace/name" and returns namespace and name
func parseKey(key string) (namespace, name string, err error) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return "", "", controller.NewPermanentError(nil)
	}
	return parts[0], parts[1], nil
}
