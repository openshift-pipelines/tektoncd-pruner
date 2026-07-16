package tektonpruner

import (
	"context"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
	"knative.dev/pkg/system"
	_ "knative.dev/pkg/system/testing" // Required for setting system namespace in tests

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	pipelinefake "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	"github.com/tektoncd/pruner/pkg/config"
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

// TestSafeRunGarbageCollector verifies that concurrent triggers never produce
// overlapping sweeps: the ConfigMap watcher starts one goroutine per update, so a
// burst of updates must not fan out into concurrent cluster-wide sweeps.
//
// Overlap is observed across the two fake clients on purpose. A fake clientset
// serializes its whole reaction chain under one lock, so a counter kept inside a
// single client can never see more than one caller at a time. Instead a sweep is
// counted as in flight when it lists the namespaces (kube client) and counted out
// when it lists the TaskRuns of the last namespace (pipeline client), and the delay
// sits in the pipeline client. Unserialized sweeps therefore pile up in the counter,
// while serialized ones cannot.
func TestSafeRunGarbageCollector(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := logtesting.TestLogger(t)
	ctx = logging.WithLogger(ctx, logger)

	// The sweep loads the ConfigMap from the system namespace; a ConfigMap anywhere
	// else would make every sweep bail out before doing any work.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PrunerConfigMapName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60`,
		},
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}}

	kubeClient := fake.NewSimpleClientset(cm, ns)
	pipelineClient := pipelinefake.NewSimpleClientset()

	var (
		mu       sync.Mutex
		inFlight int
		maxSeen  int
		sweeps   int
	)

	kubeClient.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		inFlight++
		sweeps++
		if inFlight > maxSeen {
			maxSeen = inFlight
		}
		return false, nil, nil // fall through to the tracker
	})

	pipelineClient.PrependReactor("list", "taskruns", func(k8stesting.Action) (bool, runtime.Object, error) {
		// Linger here, outside the kube client's lock, so any sweep that was not
		// serialized shows up in the counter above while this one is still running.
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		inFlight--
		return true, &pipelinev1.TaskRunList{}, nil
	})

	ctx = context.WithValue(ctx, kubeclient.Key{}, kubeClient)
	ctx = context.WithValue(ctx, pipelineclient.Key{}, pipelineClient)

	const triggers = 5

	var wg sync.WaitGroup
	for i := 0; i < triggers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			safeRunGarbageCollector(ctx, logger)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if maxSeen != 1 {
		t.Errorf("concurrent garbage collection sweeps = %d, want 1", maxSeen)
	}
	if sweeps != triggers {
		t.Errorf("garbage collection sweeps = %d, want %d (one per trigger)", sweeps, triggers)
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
