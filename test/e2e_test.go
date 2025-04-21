package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"knative.dev/pkg/apis"
)

const (
	prunerConfigName = "tekton-pruner-default-spec"
	prunerNamespace  = "tekton-pipelines"
	testNamespace    = "pruner-test" //avoid creating test namespaces prefixed with tekton- as they are reserved for tekton components"
	waitForDeletion  = 2 * time.Minute
	pollingInterval  = 5 * time.Second
)

func TestPrunerE2E(t *testing.T) {
	ctx := context.Background()

	// Create kubernetes client
	kubeClient, err := kubernetes.NewForConfig(getConfig())
	if err != nil {
		t.Fatalf("Failed to create kubernetes client: %v", err)
	}

	// Create tekton client
	tektonClient, err := clientset.NewForConfig(getConfig())
	if err != nil {
		t.Fatalf("Failed to create tekton client: %v", err)
	}

	// Create test namespace
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create test namespace: %v", err)
	}

	// Run subtests

	// TestTTLBasedPruning
	// Tests the time-based pruning of TaskRuns
	// - Configures a TTL of 60 seconds after completion
	// - Creates a TaskRun that completes successfully
	// - Verifies that TaskRuns are deleted after the TTL period
	t.Run("TestTTLBasedPruning", func(t *testing.T) {
		testTTLBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	// TestPipelineRunTTLBasedPruning
	// Tests the time-based pruning of PipelineRuns
	// - Configures a TTL of 60 seconds after completion
	// - Creates a PipelineRun that completes successfully
	// - Verifies that PipelineRuns are deleted after the TTL period
	t.Run("TestPipelineRunTTLBasedPruning", func(t *testing.T) {
		testPipelineRunTTLBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	// TestHistoryBasedPruning
	// Tests history-based pruning of TaskRuns
	// - Configures limits: keep 2 successful and 1 failed TaskRuns
	// - Creates multiple TaskRuns (3 successful, 2 failed)
	// - Verifies that only the configured number of TaskRuns are retained
	// - Checks that older TaskRuns are pruned while keeping the most recent ones
	t.Run("TestHistoryBasedPruning", func(t *testing.T) {
		testHistoryBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	// TestPipelineRunHistoryBasedPruning
	// Tests history-based pruning of PipelineRuns
	// - Configures limits: keep 2 successful and 1 failed PipelineRuns
	// - Creates multiple PipelineRuns (3 successful, 2 failed)
	// - Verifies that only the configured number of PipelineRuns are retained
	// - Checks that older PipelineRuns are pruned while keeping the most recent ones
	t.Run("TestPipelineRunHistoryBasedPruning", func(t *testing.T) {
		testPipelineRunHistoryBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	// TestConfigurationOverrides
	// Tests namespace-specific configuration overrides for TaskRuns
	// - Sets global TTL to 300 seconds but overrides to 60 seconds for test namespace
	// - Creates TaskRuns in different namespaces
	// - Verifies that TaskRuns in the test namespace are deleted faster
	// - Confirms that TaskRuns in other namespaces follow the global TTL
	t.Run("TestConfigurationOverrides", func(t *testing.T) {
		testConfigurationOverrides(ctx, t, kubeClient, tektonClient)
	})

	// TestPipelineRunConfigurationOverrides
	// Tests namespace-specific configuration overrides for PipelineRuns
	// - Sets global TTL to 300 seconds but overrides to 60 seconds for test namespace
	// - Creates PipelineRuns in different namespaces
	// - Verifies that PipelineRuns in the test namespace are deleted faster
	// - Confirms that PipelineRuns in other namespaces follow the global TTL
	t.Run("TestPipelineRunConfigurationOverrides", func(t *testing.T) {
		testPipelineRunConfigurationOverrides(ctx, t, kubeClient, tektonClient)
	})
}

func testTTLBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Set up TTL configuration
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerConfigName,
			Namespace: prunerNamespace,
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60`,
		},
	}

	// Update or create config
	_, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if errors.IsNotFound(err) {
		_, err = kubeClient.CoreV1().ConfigMaps(prunerNamespace).Create(ctx, configMap, metav1.CreateOptions{})
	}
	if err != nil {
		t.Fatalf("Failed to configure pruner: %v", err)
	}

	// Create a completed TaskRun
	tr := &v1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-taskrun-ttl",
			Namespace: testNamespace,
		},
		Spec: v1.TaskRunSpec{
			TaskSpec: &v1.TaskSpec{
				Steps: []v1.Step{{
					Name:    "echo",
					Image:   "ubuntu",
					Command: []string{"echo", "hello"},
				}},
			},
		},
	}

	tr, err = tektonClient.TektonV1().TaskRuns(testNamespace).Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test TaskRun: %v", err)
	}

	// Wait for deletion
	if err := waitForTaskRunDeletion(ctx, tektonClient, tr.Name, tr.Namespace); err != nil {
		t.Errorf("TaskRun was not deleted by TTL: %v", err)
	}
}

func testPipelineRunTTLBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Set up TTL configuration
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerConfigName,
			Namespace: prunerNamespace,
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60`,
		},
	}

	// Update or create config
	_, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if errors.IsNotFound(err) {
		_, err = kubeClient.CoreV1().ConfigMaps(prunerNamespace).Create(ctx, configMap, metav1.CreateOptions{})
	}
	if err != nil {
		t.Fatalf("Failed to configure pruner: %v", err)
	}

	// Create a PipelineRun
	pr := &v1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipelinerun-ttl",
			Namespace: testNamespace,
		},
		Spec: v1.PipelineRunSpec{
			PipelineSpec: &v1.PipelineSpec{
				Tasks: []v1.PipelineTask{{
					Name: "test-task",
					TaskSpec: &v1.EmbeddedTask{
						TaskSpec: v1.TaskSpec{
							Steps: []v1.Step{{
								Name:    "echo",
								Image:   "ubuntu",
								Command: []string{"echo", "hello"},
							}},
						},
					},
				}},
			},
		},
	}

	pr, err = tektonClient.TektonV1().PipelineRuns(testNamespace).Create(ctx, pr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test PipelineRun: %v", err)
	}

	// Wait for deletion
	if err := waitForPipelineRunDeletion(ctx, tektonClient, pr.Name, pr.Namespace); err != nil {
		t.Errorf("PipelineRun was not deleted by TTL: %v", err)
	}
}

func testHistoryBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Configure history limits
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerConfigName,
			Namespace: prunerNamespace,
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: global
successfulHistoryLimit: 2
failedHistoryLimit: 1`,
		},
	}

	_, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to configure history limits: %v", err)
	}

	// Create multiple TaskRuns with different statuses
	for i := 0; i < 3; i++ {
		tr := &v1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-taskrun-success-%d", i),
				Namespace: testNamespace,
				Labels: map[string]string{
					"tekton.dev/task": "test-task",
				},
			},
			Spec: v1.TaskRunSpec{
				TaskSpec: &v1.TaskSpec{
					Steps: []v1.Step{{
						Name:    "echo",
						Image:   "ubuntu",
						Command: []string{"echo", "hello"},
					}},
				},
			},
		}

		tr, err = tektonClient.TektonV1().TaskRuns(testNamespace).Create(ctx, tr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test TaskRun: %v", err)
		}
	}

	// Create failed TaskRuns
	for i := 0; i < 2; i++ {
		tr := &v1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-taskrun-failed-%d", i),
				Namespace: testNamespace,
				Labels: map[string]string{
					"tekton.dev/task": "test-task",
				},
			},
			Spec: v1.TaskRunSpec{
				TaskSpec: &v1.TaskSpec{
					Steps: []v1.Step{{
						Name:    "fail",
						Image:   "ubuntu",
						Command: []string{"false"},
					}},
				},
			},
		}

		tr, err = tektonClient.TektonV1().TaskRuns(testNamespace).Create(ctx, tr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test TaskRun: %v", err)
		}
	}

	// Wait and verify limits
	time.Sleep(30 * time.Second) // Wait for pruner to process

	// Check successful TaskRuns
	trs, err := tektonClient.TektonV1().TaskRuns(testNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "tekton.dev/task=test-task",
	})
	if err != nil {
		t.Fatalf("Failed to list TaskRuns: %v", err)
	}

	successCount := 0
	failedCount := 0
	for _, tr := range trs.Items {
		if tr.Status.GetCondition(apis.ConditionSucceeded).IsTrue() {
			successCount++
		} else if tr.Status.GetCondition(apis.ConditionSucceeded).IsFalse() {
			failedCount++
		}
	}

	if successCount > 2 {
		t.Errorf("Expected at most 2 successful TaskRuns, got %d", successCount)
	}
	if failedCount > 1 {
		t.Errorf("Expected at most 1 failed TaskRun, got %d", failedCount)
	}
}

func testPipelineRunHistoryBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Configure history limits
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerConfigName,
			Namespace: prunerNamespace,
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: global
successfulHistoryLimit: 2
failedHistoryLimit: 1`,
		},
	}

	_, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to configure history limits: %v", err)
	}

	// Create multiple successful PipelineRuns
	for i := 0; i < 3; i++ {
		pr := &v1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-pipelinerun-success-%d", i),
				Namespace: testNamespace,
				Labels: map[string]string{
					"tekton.dev/pipeline": "test-pipeline",
				},
			},
			Spec: v1.PipelineRunSpec{
				PipelineSpec: &v1.PipelineSpec{
					Tasks: []v1.PipelineTask{{
						Name: "test-task",
						TaskSpec: &v1.EmbeddedTask{
							TaskSpec: v1.TaskSpec{
								Steps: []v1.Step{{
									Name:    "echo",
									Image:   "ubuntu",
									Command: []string{"echo", "hello"},
								}},
							},
						},
					}},
				},
			},
		}

		pr, err = tektonClient.TektonV1().PipelineRuns(testNamespace).Create(ctx, pr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test PipelineRun: %v", err)
		}
	}

	// Create failed PipelineRuns
	for i := 0; i < 2; i++ {
		pr := &v1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-pipelinerun-failed-%d", i),
				Namespace: testNamespace,
				Labels: map[string]string{
					"tekton.dev/pipeline": "test-pipeline",
				},
			},
			Spec: v1.PipelineRunSpec{
				PipelineSpec: &v1.PipelineSpec{
					Tasks: []v1.PipelineTask{{
						Name: "test-task",
						TaskSpec: &v1.EmbeddedTask{
							TaskSpec: v1.TaskSpec{
								Steps: []v1.Step{{
									Name:    "fail",
									Image:   "ubuntu",
									Command: []string{"false"},
								}},
							},
						},
					}},
				},
			},
		}

		pr, err = tektonClient.TektonV1().PipelineRuns(testNamespace).Create(ctx, pr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test PipelineRun: %v", err)
		}
	}

	// Wait and verify limits
	time.Sleep(30 * time.Second)

	// Check PipelineRuns
	prs, err := tektonClient.TektonV1().PipelineRuns(testNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "tekton.dev/pipeline=test-pipeline",
	})
	if err != nil {
		t.Fatalf("Failed to list PipelineRuns: %v", err)
	}

	successCount := 0
	failedCount := 0
	for _, pr := range prs.Items {
		if pr.Status.GetCondition(apis.ConditionSucceeded).IsTrue() {
			successCount++
		} else if pr.Status.GetCondition(apis.ConditionSucceeded).IsFalse() {
			failedCount++
		}
	}

	if successCount > 2 {
		t.Errorf("Expected at most 2 successful PipelineRuns, got %d", successCount)
	}
	if failedCount > 1 {
		t.Errorf("Expected at most 1 failed PipelineRun, got %d", failedCount)
	}
}

func testConfigurationOverrides(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Set up configuration with namespace override
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerConfigName,
			Namespace: prunerNamespace,
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 300
namespaces:
  pruner-test:
    ttlSecondsAfterFinished: 60`,
		},
	}

	_, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to configure namespace override: %v", err)
	}

	// Create TaskRuns in different namespaces
	namespaces := []string{testNamespace, "default"}
	for _, ns := range namespaces {
		tr := &v1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-taskrun-override-%s", ns),
				Namespace: ns,
			},
			Spec: v1.TaskRunSpec{
				TaskSpec: &v1.TaskSpec{
					Steps: []v1.Step{{
						Name:    "echo",
						Image:   "ubuntu",
						Command: []string{"echo", "hello"},
					}},
				},
			},
		}

		tr, err = tektonClient.TektonV1().TaskRuns(ns).Create(ctx, tr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test TaskRun in namespace %s: %v", ns, err)
		}
	}

	// TaskRun in testNamespace should be deleted faster
	if err := waitForTaskRunDeletion(ctx, tektonClient, fmt.Sprintf("test-taskrun-override-%s", testNamespace), testNamespace); err != nil {
		t.Errorf("TaskRun in test namespace was not deleted as expected: %v", err)
	}

	// TaskRun in default namespace should still exist
	_, err = tektonClient.TektonV1().TaskRuns("default").Get(ctx, fmt.Sprintf("test-taskrun-override-default"), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Error("TaskRun in default namespace was deleted when it should still exist")
	}
}

func testPipelineRunConfigurationOverrides(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerConfigName,
			Namespace: prunerNamespace,
		},
		Data: map[string]string{
			"global-config": `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 300
namespaces:
  pruner-test:
    ttlSecondsAfterFinished: 60`,
		},
	}

	_, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to configure namespace override: %v", err)
	}

	// Create PipelineRuns in different namespaces
	namespaces := []string{testNamespace, "default"}
	for _, ns := range namespaces {
		pr := &v1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-pipelinerun-override-%s", ns),
				Namespace: ns,
			},
			Spec: v1.PipelineRunSpec{
				PipelineSpec: &v1.PipelineSpec{
					Tasks: []v1.PipelineTask{{
						Name: "test-task",
						TaskSpec: &v1.EmbeddedTask{
							TaskSpec: v1.TaskSpec{
								Steps: []v1.Step{{
									Name:    "echo",
									Image:   "ubuntu",
									Command: []string{"echo", "hello"},
								}},
							},
						},
					}},
				},
			},
		}

		pr, err = tektonClient.TektonV1().PipelineRuns(ns).Create(ctx, pr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test PipelineRun in namespace %s: %v", ns, err)
		}
	}

	// PipelineRun in testNamespace should be deleted faster
	if err := waitForPipelineRunDeletion(ctx, tektonClient, fmt.Sprintf("test-pipelinerun-override-%s", testNamespace), testNamespace); err != nil {
		t.Errorf("PipelineRun in test namespace was not deleted as expected: %v", err)
	}

	// PipelineRun in default namespace should still exist
	_, err = tektonClient.TektonV1().PipelineRuns("default").Get(ctx, fmt.Sprintf("test-pipelinerun-override-default"), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Error("PipelineRun in default namespace was deleted when it should still exist")
	}
}

func waitForTaskRunDeletion(ctx context.Context, client *clientset.Clientset, name, namespace string) error {
	timeout := time.After(waitForDeletion)
	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for TaskRun deletion")
		case <-ticker.C:
			_, err := client.TektonV1().TaskRuns(namespace).Get(ctx, name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
		}
	}
}

func waitForPipelineRunDeletion(ctx context.Context, client *clientset.Clientset, name, namespace string) error {
	timeout := time.After(waitForDeletion)
	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for PipelineRun deletion")
		case <-ticker.C:
			_, err := client.TektonV1().PipelineRuns(namespace).Get(ctx, name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
		}
	}
}

// getConfig returns a kubernetes client config for the current context
func getConfig() *rest.Config {
	// Try getting in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config
	}
	// Fall back to kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	kubeConfigArgs, _ := kubeConfig.ClientConfig()
	return kubeConfigArgs
}
