package namespaceprunerconfig

import (
	"context"
	"testing"

	"github.com/tektoncd/pruner/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
)

func TestReconcile_ConfigMapCreated(t *testing.T) {
	// a fake Kubernetes client
	kubeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	logger := logtesting.TestLogger(t)
	ctx = logging.WithLogger(ctx, logger)

	// Create a test ConfigMap with namespace-level config
	testNamespace := "test-namespace"
	testConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PrunerNamespaceConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			config.PrunerNamespaceConfigKey: `
ttlSecondsAfterFinished: 60
successfulHistoryLimit: 5
failedHistoryLimit: 10
`,
		},
	}

	// Create the ConfigMap in the fake cluster
	_, err := kubeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx, testConfigMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test ConfigMap: %v", err)
	}

	// Create reconciler
	reconciler := &Reconciler{
		kubeclient: kubeClient,
	}

	// reconcile the ConfigMap
	key := testNamespace + "/" + config.PrunerNamespaceConfigMapName
	err = reconciler.Reconcile(ctx, key)

	// Verify
	if err != nil {
		t.Errorf("Reconcile() returned error: %v", err)
	}
}

func TestReconcile_ConfigMapDeleted(t *testing.T) {
	// Create a fake Kubernetes client (without the ConfigMap)
	kubeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	logger := logtesting.TestLogger(t)
	ctx = logging.WithLogger(ctx, logger)

	testNamespace := "test-namespace"

	// Create reconciler
	reconciler := &Reconciler{
		kubeclient: kubeClient,
	}

	// try to reconcile a non-existent ConfigMap
	key := testNamespace + "/" + config.PrunerNamespaceConfigMapName
	err := reconciler.Reconcile(ctx, key)

	// Verify
	if err != nil {
		t.Errorf("Reconcile() returned error on deletion: %v", err)
	}
}

// TestReconcile_InvalidKey tests that invalid keys are handled properly
func TestReconcile_InvalidKey(t *testing.T) {
	// Setup
	kubeClient := fake.NewSimpleClientset()
	ctx := context.Background()
	logger := logtesting.TestLogger(t)
	ctx = logging.WithLogger(ctx, logger)

	reconciler := &Reconciler{
		kubeclient: kubeClient,
	}

	// try to reconcile with invalid key (no slash)
	invalidKey := "invalid-key-without-slash"
	err := reconciler.Reconcile(ctx, invalidKey)

	// verify it should return an error
	if err == nil {
		t.Error("Reconcile() should return error for invalid key")
	}
}

func TestParseKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		wantNamespace string
		wantName      string
		wantError     bool
	}{
		{
			name:          "valid key",
			key:           "test-namespace/tekton-pruner-namespace-spec",
			wantNamespace: "test-namespace",
			wantName:      "tekton-pruner-namespace-spec",
			wantError:     false,
		},
		{
			name:          "invalid key - no slash",
			key:           "invalid-key",
			wantNamespace: "",
			wantName:      "",
			wantError:     true,
		},
		{
			name:          "invalid key - empty",
			key:           "",
			wantNamespace: "",
			wantName:      "",
			wantError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, name, err := parseKey(tt.key)

			if tt.wantError {
				if err == nil {
					t.Error("parseKey() should return error")
				}
				return
			}

			if err != nil {
				t.Errorf("parseKey() returned unexpected error: %v", err)
			}

			if namespace != tt.wantNamespace {
				t.Errorf("namespace = %v, want %v", namespace, tt.wantNamespace)
			}

			if name != tt.wantName {
				t.Errorf("name = %v, want %v", name, tt.wantName)
			}
		})
	}
}
