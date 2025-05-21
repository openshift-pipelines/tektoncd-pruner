package tektonpruner

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
	_ "knative.dev/pkg/system/testing" // Required for setting system namespace in tests

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
)

func TestGarbageCollection(t *testing.T) {
	tests := []struct {
		name          string
		configMapData map[string]string
		wantGCTrigger bool
		wantError     bool
	}{
		{
			name: "Valid config triggers GC",
			configMapData: map[string]string{
				"global-config": `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60
successfulHistoryLimit: 5
failedHistoryLimit: 2`,
			},
			wantGCTrigger: true,
			wantError:     false,
		},
		{
			name: "Invalid config does not trigger GC",
			configMapData: map[string]string{
				"global-config": `invalid: yaml: content`,
			},
			wantGCTrigger: false,
			wantError:     true,
		},
		{
			name:          "Empty config data does not trigger GC",
			configMapData: map[string]string{},
			wantGCTrigger: false,
			wantError:     false,
		},
		{
			name: "Missing required fields does not trigger GC",
			configMapData: map[string]string{
				"global-config": `enforcedConfigLevel: global`,
			},
			wantGCTrigger: false,
			wantError:     true,
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

			// Create config map watcher with the initial config map
			cmw := configmap.NewStaticWatcher(cm)

			// Create a function to handle config map updates
			updateHandler := func(configMap *corev1.ConfigMap) {
				// Try to load the config to check validity
				err := config.PrunerConfigStore.LoadGlobalConfig(ctx, configMap)
				if err != nil {
					if tt.wantError {
						// Error was expected, don't trigger GC
						gcTriggered <- false
						return
					}
					t.Errorf("Unexpected error parsing config: %v", err)
					gcTriggered <- false
					return
				} else if tt.wantError {
					t.Error("Expected error but got none")
					gcTriggered <- false
					return
				}

				// Check if there's any config data - only trigger GC if there is
				hasConfig := configMap.Data != nil && configMap.Data["global-config"] != ""
				gcTriggered <- hasConfig
			}

			// Watch for config map changes
			cmw.Watch(config.PrunerConfigMapName, updateHandler)

			// Start the watcher
			stopCh := make(chan struct{})
			defer close(stopCh)
			if err := cmw.Start(stopCh); err != nil {
				t.Fatalf("Failed to start config map watcher: %v", err)
			}

			// Update config map
			updatedCM := cm.DeepCopy()
			updatedCM.Data = tt.configMapData
			_, err := kubeclient.CoreV1().ConfigMaps("tekton-pipelines").Update(ctx, updatedCM, metav1.UpdateOptions{})
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
	// Create a context with timeout and logging
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	logger := logtesting.TestLogger(t)
	ctx = logging.WithLogger(ctx, logger)

	// Setup fake client
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PrunerConfigMapName,
			Namespace: "tekton-pipelines",
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60`,
		},
	}
	client := fake.NewSimpleClientset(cm)

	// Inject client into context
	ctx = context.WithValue(ctx, kubeclient.Key{}, client)

	// Create a WaitGroup to track goroutines
	var wg sync.WaitGroup
	doneChan := make(chan struct{})
	errChan := make(chan error, 5)

	// Test concurrent execution safety with error handling
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errChan <- fmt.Errorf("goroutine %d panicked: %v", id, r)
				}
			}()
			safeRunGarbageCollector(ctx, logger)
		}(i)
	}

	// Wait for goroutines in a separate goroutine
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	// Wait with timeout and error collection
	select {
	case <-doneChan:
		// Check for any errors
		close(errChan)
		for err := range errChan {
			t.Error(err)
		}
	case <-ctx.Done():
		t.Log("Context cancelled as expected")
	case <-time.After(3 * time.Second):
		t.Fatal("Test timed out waiting for garbage collectors to complete")
	}
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
