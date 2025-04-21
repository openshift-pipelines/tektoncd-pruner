package tektonpruner

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
)

func TestNewControllerBasic(t *testing.T) {
	ctx := context.Background()
	logger := logtesting.TestLogger(t)
	ctx = logging.WithLogger(ctx, logger)

	c := NewController(ctx, configmap.NewStaticWatcher())
	if c == nil {
		t.Error("Expected NewController to return a non-nil value")
	}
}

func TestGarbageCollection(t *testing.T) {
	tests := []struct {
		name          string
		configMapData map[string]string
		wantGCTrigger bool
	}{
		{
			name: "Valid config triggers GC",
			configMapData: map[string]string{
				"global-config": `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60`,
			},
			wantGCTrigger: true,
		},
		{
			name: "Invalid config does not trigger GC",
			configMapData: map[string]string{
				"global-config": `invalid: yaml: content`,
			},
			wantGCTrigger: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake config map
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.PrunerConfigMapName,
					Namespace: "tekton-pipelines",
				},
				Data: tt.configMapData,
			}

			// Create fake k8s client
			kubeclient := fake.NewSimpleClientset(cm)

			// Create a channel to track if GC was triggered
			gcTriggered := make(chan bool, 1)
			defer close(gcTriggered)

			// Create config map watcher
			cmw := configmap.NewStaticWatcher()

			// Watch for config map changes
			cmw.Watch(config.PrunerConfigMapName, func(configMap *corev1.ConfigMap) {
				gcTriggered <- true
			})

			// Start the watcher
			if err := cmw.Start(ctx.Done()); err != nil {
				t.Fatalf("Failed to start config map watcher: %v", err)
			}

			// Update config map
			_, err := kubeclient.CoreV1().ConfigMaps("tekton-pipelines").Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				t.Fatalf("Failed to update config map: %v", err)
			}

			// Wait for GC trigger or timeout
			select {
			case triggered := <-gcTriggered:
				if triggered != tt.wantGCTrigger {
					t.Errorf("GC trigger = %v, want %v", triggered, tt.wantGCTrigger)
				}
			case <-time.After(time.Second):
				if tt.wantGCTrigger {
					t.Error("Timed out waiting for GC trigger")
				}
			}
		})
	}
}

func TestSafeRunGarbageCollector(t *testing.T) {
	ctx := context.Background()
	logger := logtesting.TestLogger(t)

	// Test concurrent execution safety
	for i := 0; i < 5; i++ {
		go safeRunGarbageCollector(ctx, logger)
	}

	// Allow some time for potential race conditions
	time.Sleep(100 * time.Millisecond)
}

func TestGetFilteredNamespaces(t *testing.T) {
	tests := []struct {
		name         string
		namespaces   []string
		wantFiltered []string
	}{
		{
			name: "Filter kube- and openshift- namespaces",
			namespaces: []string{
				"default",
				"kube-system",
				"openshift-test",
				"test-namespace",
			},
			wantFiltered: []string{
				"default",
				"test-namespace",
			},
		},
		{
			name: "No namespaces to filter",
			namespaces: []string{
				"test1",
				"test2",
			},
			wantFiltered: []string{
				"test1",
				"test2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create fake namespaces
			var namespaceObjects []runtime.Object
			for _, ns := range tt.namespaces {
				namespaceObjects = append(namespaceObjects, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: ns,
					},
				})
			}

			// Create fake client with namespaces
			client := fake.NewSimpleClientset(namespaceObjects...)

			filtered, err := getFilteredNamespaces(ctx, client)
			if err != nil {
				t.Fatalf("getFilteredNamespaces() error = %v", err)
			}

			// Compare results
			if len(filtered) != len(tt.wantFiltered) {
				t.Errorf("got %d namespaces, want %d", len(filtered), len(tt.wantFiltered))
			}

			for i, ns := range filtered {
				if ns != tt.wantFiltered[i] {
					t.Errorf("namespace[%d] = %s, want %s", i, ns, tt.wantFiltered[i])
				}
			}
		})
	}
}
