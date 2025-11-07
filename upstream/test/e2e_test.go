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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"knative.dev/pkg/apis"
)

const (
	prunerConfigName = "tekton-pruner-default-spec"
	prunerNamespace  = "tekton-pipelines"
	testNamespace    = "pruner-test"
	waitForDeletion  = 5 * time.Minute
	pollingInterval  = 5 * time.Second
)

// getGlobalConfigLabels returns the required labels for global pruner ConfigMaps
func getGlobalConfigLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":     "tekton-pruner",
		"pruner.tekton.dev/config-type": "global",
		"pruner.tekton.dev/release":     "devel",
	}
}

// getNamespaceConfigLabels returns the required labels for namespace-specific pruner ConfigMaps
func getNamespaceConfigLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":     "tekton-pruner",
		"pruner.tekton.dev/config-type": "namespace",
	}
}

// Helper function to create a simple TaskRun for testing
func createTestTaskRun(name, namespace string) *v1.TaskRun {
	return &v1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.TaskRunSpec{
			TaskSpec: &v1.TaskSpec{
				Steps: []v1.Step{{
					Name:    "echo",
					Image:   "busybox",
					Command: []string{"echo", "hello"},
				}},
			},
		},
	}
}

// Helper function to create a TaskRun with labels
func createTestTaskRunWithLabels(name, namespace string, labels map[string]string) *v1.TaskRun {
	tr := createTestTaskRun(name, namespace)
	tr.Labels = labels
	return tr
}

// Helper function to create a simple PipelineRun for testing
func createTestPipelineRun(name, namespace string) *v1.PipelineRun {
	return &v1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PipelineRunSpec{
			PipelineSpec: &v1.PipelineSpec{
				Tasks: []v1.PipelineTask{{
					Name: "test-task",
					TaskSpec: &v1.EmbeddedTask{
						TaskSpec: v1.TaskSpec{
							Steps: []v1.Step{{
								Name:    "echo",
								Image:   "busybox",
								Command: []string{"echo", "hello"},
							}},
						},
					},
				}},
			},
		},
	}
}

// Helper function to create a PipelineRun with labels
func createTestPipelineRunWithLabels(name, namespace string, labels map[string]string) *v1.PipelineRun {
	pr := createTestPipelineRun(name, namespace)
	pr.Labels = labels
	return pr
}

// Helper function to create a global ConfigMap
func createGlobalConfigMap(name, namespace, configData string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    getGlobalConfigLabels(),
		},
		Data: map[string]string{
			"global-config": configData,
		},
	}
}

// Helper function to create a namespace ConfigMap
func createNamespaceConfigMap(name, namespace, configData string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    getNamespaceConfigLabels(),
		},
		Data: map[string]string{
			"ns-config": configData,
		},
	}
}

// updateOrCreateConfigMap updates an existing ConfigMap or creates it if it doesn't exist
func updateOrCreateConfigMap(ctx context.Context, kubeClient *kubernetes.Clientset, cm *corev1.ConfigMap) error {
	_, err := kubeClient.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if errors.IsNotFound(err) {
		_, err = kubeClient.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	}
	return err
}

// ensureClusterReady verifies that all required components are ready before running tests
func ensureClusterReady(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	t.Log("=== Verifying cluster readiness ===")

	// 1. Check Tekton Pipeline Controller
	t.Log("Checking Tekton Pipeline controller...")
	if err := waitForDeploymentReady(ctx, kubeClient, "tekton-pipelines", "tekton-pipelines-controller", 60*time.Second); err != nil {
		t.Fatalf("Tekton Pipeline controller not ready: %v", err)
	}
	t.Log("Tekton Pipeline controller is ready")

	// 2. Check Tekton Pipeline Webhook
	t.Log("Checking Tekton Pipeline webhook...")
	if err := waitForDeploymentReady(ctx, kubeClient, "tekton-pipelines", "tekton-pipelines-webhook", 60*time.Second); err != nil {
		t.Fatalf("Tekton Pipeline webhook not ready: %v", err)
	}
	t.Log("Tekton Pipeline webhook is ready")

	// 3. Check Pruner Controller
	t.Log("Checking Pruner controller...")
	if err := waitForDeploymentReady(ctx, kubeClient, prunerNamespace, "tekton-pruner-controller", 60*time.Second); err != nil {
		t.Fatalf("Pruner controller not ready: %v", err)
	}
	t.Log("Pruner controller is ready")

	// 4. Check Pruner Webhook
	t.Log("Checking Pruner webhook...")
	if err := waitForDeploymentReady(ctx, kubeClient, prunerNamespace, "tekton-pruner-webhook", 60*time.Second); err != nil {
		t.Fatalf("Pruner webhook not ready: %v", err)
	}
	t.Log("Pruner webhook is ready")

	// 5. Additional grace period for all components to be fully operational
	t.Log("Waiting for components to stabilize...")
	time.Sleep(5 * time.Second)

	t.Log("=== All cluster components are ready ===")
}

// waitForDeploymentReady waits for a deployment to have at least one ready replica
func waitForDeploymentReady(ctx context.Context, kubeClient *kubernetes.Clientset, namespace, deploymentName string, timeout time.Duration) error {
	return wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		deployment, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil // Deployment not found yet, keep waiting
			}
			return false, err
		}

		// Check if deployment has ready replicas
		if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
			return true, nil
		}

		return false, nil
	})
}

// waitForConfigSync waits for the webhook's informer to sync ConfigMap changes
// This is a simple sleep since ensureClusterReady() already verified webhook is running
func waitForConfigSync() {
	// Brief pause to allow webhook informers to sync ConfigMap updates
	// This is needed because Kubernetes informers may have a slight delay
	// before reflecting the latest ConfigMap state in their cache
	time.Sleep(2 * time.Second)
}

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

	// Ensure all required cluster components are ready before running tests
	ensureClusterReady(ctx, t, kubeClient)

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
	t.Run("TestTTLBasedPruning", func(t *testing.T) {
		testTTLBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	t.Run("TestPipelineRunTTLBasedPruning", func(t *testing.T) {
		testPipelineRunTTLBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	t.Run("TestHistoryBasedPruning", func(t *testing.T) {
		testHistoryBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	t.Run("TestPipelineRunHistoryBasedPruning", func(t *testing.T) {
		testPipelineRunHistoryBasedPruning(ctx, t, kubeClient, tektonClient)
	})

	t.Run("TestConfigurationOverrides", func(t *testing.T) {
		testConfigurationOverrides(ctx, t, kubeClient, tektonClient)
	})

	t.Run("TestPipelineRunConfigurationOverrides", func(t *testing.T) {
		testPipelineRunConfigurationOverrides(ctx, t, kubeClient, tektonClient)
	})

	t.Run("TestWebhookValidation_ValidGlobalConfig", func(t *testing.T) {
		testWebhookValidGlobalConfig(ctx, t, kubeClient)
	})

	t.Run("TestWebhookValidation_InvalidGlobalConfig", func(t *testing.T) {
		testWebhookInvalidGlobalConfig(ctx, t, kubeClient)
	})

	t.Run("TestWebhookValidation_NamespaceConfigWithinLimits", func(t *testing.T) {
		testWebhookNamespaceConfigWithinLimits(ctx, t, kubeClient)
	})

	t.Run("TestWebhookValidation_NamespaceConfigExceedsLimits", func(t *testing.T) {
		testWebhookNamespaceConfigExceedsLimits(ctx, t, kubeClient)
	})

	t.Run("TestWebhookValidation_UpdateGlobalConfig", func(t *testing.T) {
		testWebhookUpdateGlobalConfig(ctx, t, kubeClient)
	})

	t.Run("TestWebhookValidation_UpdateNamespaceConfig", func(t *testing.T) {
		testWebhookUpdateNamespaceConfig(ctx, t, kubeClient)
	})

	t.Run("TestWebhookValidation_SystemMaximumEnforcement", func(t *testing.T) {
		testWebhookSystemMaximumEnforcement(ctx, t, kubeClient)
	})

	t.Run("TestWebhookValidation_ForbiddenNamespaces", func(t *testing.T) {
		testWebhookForbiddenNamespaces(ctx, t, kubeClient)
	})
}

func testTTLBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Set up TTL configuration
	configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, configMap); err != nil {
		t.Fatalf("Failed to configure pruner with TTL settings: %v", err)
	}

	// Create a TaskRun
	tr := createTestTaskRun("test-taskrun-ttl", testNamespace)

	tr, err := tektonClient.TektonV1().TaskRuns(testNamespace).Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test TaskRun %q in namespace %q: %v", tr.Name, testNamespace, err)
	}

	// Add cleanup to ensure test doesn't leave resources behind
	t.Cleanup(func() {
		// Best effort cleanup - ignore errors as resource may already be deleted
		tektonClient.TektonV1().TaskRuns(testNamespace).Delete(context.Background(), tr.Name, metav1.DeleteOptions{})
	})

	t.Logf("Created TaskRun %q, waiting for TTL-based deletion...", tr.Name)

	// Wait for deletion
	if err := waitForTaskRunDeletion(ctx, tektonClient, tr.Name, tr.Namespace); err != nil {
		t.Errorf("TaskRun %q in namespace %q was not deleted by TTL after %v: %v",
			tr.Name, tr.Namespace, waitForDeletion, err)
	} else {
		t.Logf("TaskRun %q successfully deleted by TTL", tr.Name)
	}
}

func testPipelineRunTTLBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Set up TTL configuration
	configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
ttlSecondsAfterFinished: 60`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, configMap); err != nil {
		t.Fatalf("Failed to configure pruner with TTL settings: %v", err)
	}

	// Create a PipelineRun
	pr := createTestPipelineRun("test-pipelinerun-ttl", testNamespace)

	pr, err := tektonClient.TektonV1().PipelineRuns(testNamespace).Create(ctx, pr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test PipelineRun %q in namespace %q: %v", pr.Name, testNamespace, err)
	}

	// Add cleanup
	t.Cleanup(func() {
		tektonClient.TektonV1().PipelineRuns(testNamespace).Delete(context.Background(), pr.Name, metav1.DeleteOptions{})
	})

	t.Logf("Created PipelineRun %q, waiting for TTL-based deletion...", pr.Name)

	// Wait for deletion
	if err := waitForPipelineRunDeletion(ctx, tektonClient, pr.Name, pr.Namespace); err != nil {
		t.Errorf("PipelineRun %q in namespace %q was not deleted by TTL after %v: %v",
			pr.Name, pr.Namespace, waitForDeletion, err)
	} else {
		t.Logf("PipelineRun %q successfully deleted by TTL", pr.Name)
	}
}

func testHistoryBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Configure history limits
	configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
successfulHistoryLimit: 2
failedHistoryLimit: 1`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, configMap); err != nil {
		t.Fatalf("Failed to configure history limits: %v", err)
	}

	taskLabel := "test-task-history"
	labels := map[string]string{"tekton.dev/task": taskLabel}

	// Add cleanup to remove all test TaskRuns
	t.Cleanup(func() {
		tektonClient.TektonV1().TaskRuns(testNamespace).DeleteCollection(
			context.Background(),
			metav1.DeleteOptions{},
			metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/task=%s", taskLabel)},
		)
	})

	// Create multiple successful TaskRuns
	t.Logf("Creating 3 successful TaskRuns...")
	for i := 0; i < 3; i++ {
		tr := createTestTaskRunWithLabels(fmt.Sprintf("test-taskrun-success-%d", i), testNamespace, labels)
		_, err := tektonClient.TektonV1().TaskRuns(testNamespace).Create(ctx, tr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create successful TaskRun %q: %v", tr.Name, err)
		}
		t.Logf("Created successful TaskRun: %s", tr.Name)
	}

	// Create failed TaskRuns
	t.Logf("Creating 2 failed TaskRuns...")
	for i := 0; i < 2; i++ {
		tr := createTestTaskRunWithLabels(fmt.Sprintf("test-taskrun-failed-%d", i), testNamespace, labels)
		// Override the command to fail
		tr.Spec.TaskSpec.Steps[0].Command = []string{"false"}
		_, err := tektonClient.TektonV1().TaskRuns(testNamespace).Create(ctx, tr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create failed TaskRun %q: %v", tr.Name, err)
		}
		t.Logf("Created failed TaskRun: %s", tr.Name)
	}

	// Wait for pruning with proper polling
	t.Logf("Waiting up to 150 seconds for history-based pruning...")
	time.Sleep(150 * time.Second) // TODO: Replace with proper wait condition when pruner provides status

	// Check TaskRuns after pruning
	trs, err := tektonClient.TektonV1().TaskRuns(testNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tekton.dev/task=%s", taskLabel),
	})
	if err != nil {
		t.Fatalf("Failed to list TaskRuns: %v", err)
	}

	successCount := 0
	failedCount := 0
	for _, tr := range trs.Items {
		cond := tr.Status.GetCondition(apis.ConditionSucceeded)
		if cond != nil {
			if cond.IsTrue() {
				successCount++
				t.Logf("Found successful TaskRun: %s", tr.Name)
			} else if cond.IsFalse() {
				failedCount++
				t.Logf("Found failed TaskRun: %s", tr.Name)
			}
		}
	}

	t.Logf("After pruning: %d successful TaskRuns, %d failed TaskRuns (expected: ≤2 successful, ≤1 failed)",
		successCount, failedCount)

	if successCount > 2 {
		t.Errorf("Expected at most 2 successful TaskRuns after pruning, got %d", successCount)
	}
	if failedCount > 1 {
		t.Errorf("Expected at most 1 failed TaskRun after pruning, got %d", failedCount)
	}
}

func testPipelineRunHistoryBasedPruning(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
successfulHistoryLimit: 2
failedHistoryLimit: 1`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, configMap); err != nil {
		t.Fatalf("Failed to configure history limits: %v", err)
	}

	pipelineLabel := "test-pipeline-history"
	labels := map[string]string{"tekton.dev/pipeline": pipelineLabel}

	t.Cleanup(func() {
		tektonClient.TektonV1().PipelineRuns(testNamespace).DeleteCollection(
			context.Background(),
			metav1.DeleteOptions{},
			metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/pipeline=%s", pipelineLabel)},
		)
	})

	t.Logf("Creating 3 successful PipelineRuns...")
	for i := 0; i < 3; i++ {
		pr := createTestPipelineRunWithLabels(fmt.Sprintf("test-pipelinerun-success-%d", i), testNamespace, labels)
		_, err := tektonClient.TektonV1().PipelineRuns(testNamespace).Create(ctx, pr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create successful PipelineRun %q: %v", pr.Name, err)
		}
		t.Logf("Created successful PipelineRun: %s", pr.Name)
	}

	t.Logf("Creating 2 failed PipelineRuns...")
	for i := 0; i < 2; i++ {
		pr := createTestPipelineRunWithLabels(fmt.Sprintf("test-pipelinerun-failed-%d", i), testNamespace, labels)
		pr.Spec.PipelineSpec.Tasks[0].TaskSpec.TaskSpec.Steps[0].Command = []string{"false"}
		_, err := tektonClient.TektonV1().PipelineRuns(testNamespace).Create(ctx, pr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create failed PipelineRun %q: %v", pr.Name, err)
		}
		t.Logf("Created failed PipelineRun: %s", pr.Name)
	}

	t.Logf("Waiting 30 seconds for history-based pruning...")
	time.Sleep(30 * time.Second)

	prs, err := tektonClient.TektonV1().PipelineRuns(testNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tekton.dev/pipeline=%s", pipelineLabel),
	})
	if err != nil {
		t.Fatalf("Failed to list PipelineRuns: %v", err)
	}

	successCount := 0
	failedCount := 0
	for _, pr := range prs.Items {
		cond := pr.Status.GetCondition(apis.ConditionSucceeded)
		if cond != nil {
			if cond.IsTrue() {
				successCount++
				t.Logf("Found successful PipelineRun: %s", pr.Name)
			} else if cond.IsFalse() {
				failedCount++
				t.Logf("Found failed PipelineRun: %s", pr.Name)
			}
		}
	}

	t.Logf("After pruning: %d successful PipelineRuns, %d failed PipelineRuns (expected: ≤2 successful, ≤1 failed)",
		successCount, failedCount)

	if successCount > 2 {
		t.Errorf("Expected at most 2 successful PipelineRuns after pruning, got %d", successCount)
	}
	if failedCount > 1 {
		t.Errorf("Expected at most 1 failed PipelineRun after pruning, got %d", failedCount)
	}
}

func testConfigurationOverrides(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	// Set up configuration with namespace override
	configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 300
namespaces:
  pruner-test:
    ttlSecondsAfterFinished: 60`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, configMap); err != nil {
		t.Fatalf("Failed to configure namespace override: %v", err)
	}

	// Create TaskRuns in different namespaces
	namespaces := []string{testNamespace, "default"}
	createdTaskRuns := make(map[string]string) // namespace -> taskrun name

	for _, ns := range namespaces {
		trName := fmt.Sprintf("test-taskrun-override-%s", ns)
		tr := createTestTaskRun(trName, ns)

		tr, err := tektonClient.TektonV1().TaskRuns(ns).Create(ctx, tr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test TaskRun %q in namespace %q: %v", trName, ns, err)
		}
		createdTaskRuns[ns] = tr.Name
		t.Logf("Created TaskRun %q in namespace %q", tr.Name, ns)
	}

	// Add cleanup
	t.Cleanup(func() {
		for ns, name := range createdTaskRuns {
			tektonClient.TektonV1().TaskRuns(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
		}
	})

	// TaskRun in testNamespace should be deleted faster (60s TTL)
	t.Logf("Waiting for TaskRun in %q to be deleted by 60s TTL...", testNamespace)
	if err := waitForTaskRunDeletion(ctx, tektonClient, createdTaskRuns[testNamespace], testNamespace); err != nil {
		t.Errorf("TaskRun %q in test namespace %q was not deleted as expected: %v",
			createdTaskRuns[testNamespace], testNamespace, err)
	} else {
		t.Logf("✓ TaskRun in test namespace deleted successfully by namespace-specific TTL")
	}

	// TaskRun in default namespace should still exist (300s TTL not reached yet)
	t.Logf("Verifying TaskRun in default namespace still exists (300s TTL not reached)...")
	_, err := tektonClient.TektonV1().TaskRuns("default").Get(ctx, createdTaskRuns["default"], metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Errorf("TaskRun %q in default namespace was prematurely deleted when it should still exist",
			createdTaskRuns["default"])
	} else if err != nil {
		t.Errorf("Error checking TaskRun in default namespace: %v", err)
	} else {
		t.Logf("✓ TaskRun in default namespace still exists as expected")
	}
}

func testPipelineRunConfigurationOverrides(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset, tektonClient *clientset.Clientset) {
	configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 300
namespaces:
  pruner-test:
    ttlSecondsAfterFinished: 60`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, configMap); err != nil {
		t.Fatalf("Failed to configure namespace override: %v", err)
	}

	namespaces := []string{testNamespace, "default"}
	createdPipelineRuns := make(map[string]string)

	for _, ns := range namespaces {
		prName := fmt.Sprintf("test-pipelinerun-override-%s", ns)
		pr := createTestPipelineRun(prName, ns)

		pr, err := tektonClient.TektonV1().PipelineRuns(ns).Create(ctx, pr, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test PipelineRun %q in namespace %q: %v", prName, ns, err)
		}
		createdPipelineRuns[ns] = pr.Name
		t.Logf("Created PipelineRun %q in namespace %q", pr.Name, ns)
	}

	t.Cleanup(func() {
		for ns, name := range createdPipelineRuns {
			tektonClient.TektonV1().PipelineRuns(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
		}
	})

	t.Logf("Waiting for PipelineRun in %q to be deleted by 60s TTL...", testNamespace)
	if err := waitForPipelineRunDeletion(ctx, tektonClient, createdPipelineRuns[testNamespace], testNamespace); err != nil {
		t.Errorf("PipelineRun %q in test namespace %q was not deleted as expected: %v",
			createdPipelineRuns[testNamespace], testNamespace, err)
	} else {
		t.Logf("✓ PipelineRun in test namespace deleted successfully by namespace-specific TTL")
	}

	t.Logf("Verifying PipelineRun in default namespace still exists (300s TTL not reached)...")
	_, err := tektonClient.TektonV1().PipelineRuns("default").Get(ctx, createdPipelineRuns["default"], metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Errorf("PipelineRun %q in default namespace was prematurely deleted when it should still exist",
			createdPipelineRuns["default"])
	} else if err != nil {
		t.Errorf("Error checking PipelineRun in default namespace: %v", err)
	} else {
		t.Logf("✓ PipelineRun in default namespace still exists as expected")
	}
}

func testWebhookValidGlobalConfig(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
ttlSecondsAfterFinished: 300
successfulHistoryLimit: 5
failedHistoryLimit: 3`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, configMap); err != nil {
		t.Errorf("Valid global config was rejected by webhook: %v", err)
	} else {
		t.Logf("✓ Webhook correctly accepted valid global config")
	}
}

func testWebhookInvalidGlobalConfig(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	testCases := []struct {
		name   string
		config string
	}{
		{
			name: "negative TTL",
			config: `enforcedConfigLevel: global
ttlSecondsAfterFinished: -100`,
		},
		{
			name: "negative history limit",
			config: `enforcedConfigLevel: global
successfulHistoryLimit: -5`,
		},
		{
			name: "invalid enforcedConfigLevel",
			config: `enforcedConfigLevel: invalid
ttlSecondsAfterFinished: 300`,
		},
		{
			name: "invalid YAML structure",
			config: `enforcedConfigLevel: global
  ttlSecondsAfterFinished: 60
 invalid indentation here`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configMap := createGlobalConfigMap(prunerConfigName, prunerNamespace, tc.config)

			original, _ := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Get(ctx, prunerConfigName, metav1.GetOptions{})

			_, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
			if err == nil {
				t.Errorf("Invalid global config was accepted by webhook when it should have been rejected")
				if original != nil {
					kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, original, metav1.UpdateOptions{})
				}
			} else {
				t.Logf("✓ Webhook correctly rejected invalid config: %v", err)
			}
		})
	}
}

func testWebhookNamespaceConfigWithinLimits(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	// Ensure global config exists with limits
	globalConfig := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
ttlSecondsAfterFinished: 500
successfulHistoryLimit: 10
failedHistoryLimit: 5`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, globalConfig); err != nil {
		t.Fatalf("Failed to set up global config: %v", err)
	}

	// Wait for webhook's informer to sync the global config
	waitForConfigSync()

	// Create namespace config within limits
	namespaceConfig := createNamespaceConfigMap("tekton-pruner-namespace-spec", testNamespace, `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 300
successfulHistoryLimit: 5
failedHistoryLimit: 3`)

	// Add cleanup
	t.Cleanup(func() {
		kubeClient.CoreV1().ConfigMaps(testNamespace).Delete(context.Background(),
			namespaceConfig.Name, metav1.DeleteOptions{})
	})

	// Should succeed
	_, err := kubeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx, namespaceConfig, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Errorf("Valid namespace config within limits was rejected by webhook: %v", err)
	} else {
		t.Logf("Webhook correctly accepted namespace config within limits")
	}
}

func testWebhookNamespaceConfigExceedsLimits(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	// Ensure global config exists with limits
	globalConfig := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
ttlSecondsAfterFinished: 500
successfulHistoryLimit: 10
failedHistoryLimit: 5`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, globalConfig); err != nil {
		t.Fatalf("Failed to set up global config: %v", err)
	}

	// Wait for webhook's informer to sync the global config
	waitForConfigSync()

	testCases := []struct {
		name        string
		config      string
		description string
	}{
		{
			name: "TTL exceeds global",
			config: `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 1000`,
			description: "TTL of 1000 exceeds global limit of 500",
		},
		{
			name: "successfulHistoryLimit exceeds global",
			config: `enforcedConfigLevel: namespace
successfulHistoryLimit: 20`,
			description: "successfulHistoryLimit of 20 exceeds global limit of 10",
		},
		{
			name: "failedHistoryLimit exceeds global",
			config: `enforcedConfigLevel: namespace
failedHistoryLimit: 10`,
			description: "failedHistoryLimit of 10 exceeds global limit of 5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			namespaceConfig := createNamespaceConfigMap("tekton-pruner-namespace-spec", testNamespace, tc.config)

			// Add cleanup in case it gets created
			t.Cleanup(func() {
				kubeClient.CoreV1().ConfigMaps(testNamespace).Delete(context.Background(),
					namespaceConfig.Name, metav1.DeleteOptions{})
			})

			t.Logf("Testing: %s", tc.description)

			// Should fail
			_, err := kubeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx, namespaceConfig, metav1.CreateOptions{})
			if err == nil {
				t.Errorf("Namespace config exceeding limits was accepted by webhook when it should have been rejected (case: %s)",
					tc.description)
			} else {
				t.Logf("✓ Webhook correctly rejected namespace config: %v", err)
			}
		})
	}
}

func testWebhookUpdateGlobalConfig(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	validConfig := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
ttlSecondsAfterFinished: 300
successfulHistoryLimit: 5`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, validConfig); err != nil {
		t.Fatalf("Failed to create initial global config: %v", err)
	}

	currentConfig, err := kubeClient.CoreV1().ConfigMaps(prunerNamespace).Get(ctx, prunerConfigName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get current config: %v", err)
	}

	currentConfig.Data["global-config"] = `enforcedConfigLevel: global
ttlSecondsAfterFinished: -100`

	t.Logf("Attempting to update global config with invalid negative TTL...")
	_, err = kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, currentConfig, metav1.UpdateOptions{})
	if err == nil {
		t.Errorf("Invalid global config update was accepted by webhook when it should have been rejected")
		kubeClient.CoreV1().ConfigMaps(prunerNamespace).Update(ctx, validConfig, metav1.UpdateOptions{})
	} else {
		t.Logf("✓ Webhook correctly rejected invalid global config update: %v", err)
	}
}

func testWebhookUpdateNamespaceConfig(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	// Ensure global config exists with limits
	globalConfig := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: global
ttlSecondsAfterFinished: 500
successfulHistoryLimit: 10`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, globalConfig); err != nil {
		t.Fatalf("Failed to set up global config: %v", err)
	}

	// Wait for webhook's informer to sync global config
	waitForConfigSync()

	// Create valid namespace config
	validNamespaceConfig := createNamespaceConfigMap("tekton-pruner-namespace-spec", testNamespace, `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 300
successfulHistoryLimit: 5`)

	// Add cleanup
	t.Cleanup(func() {
		kubeClient.CoreV1().ConfigMaps(testNamespace).Delete(context.Background(),
			validNamespaceConfig.Name, metav1.DeleteOptions{})
	})

	_, err := kubeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx, validNamespaceConfig, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create initial namespace config: %v", err)
	}

	// Get current namespace config
	currentConfig, err := kubeClient.CoreV1().ConfigMaps(testNamespace).Get(ctx, validNamespaceConfig.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get current namespace config: %v", err)
	}

	// Attempt to update with values exceeding global limits
	currentConfig.Data["ns-config"] = `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 1000
successfulHistoryLimit: 20`

	t.Logf("Attempting to update namespace config with values exceeding global limits...")
	_, err = kubeClient.CoreV1().ConfigMaps(testNamespace).Update(ctx, currentConfig, metav1.UpdateOptions{})
	if err == nil {
		t.Errorf("Namespace config update exceeding limits was accepted by webhook when it should have been rejected")
	} else {
		t.Logf("✓ Webhook correctly rejected namespace config update exceeding limits: %v", err)
	}
}

func testWebhookSystemMaximumEnforcement(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	// First, ensure there's no global config with explicit limits
	// Create a minimal global config without limits
	minimalGlobalConfig := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: namespace`)

	if err := updateOrCreateConfigMap(ctx, kubeClient, minimalGlobalConfig); err != nil {
		t.Fatalf("Failed to set up minimal global config: %v", err)
	}

	// Wait for webhook's informer to sync the global config
	waitForConfigSync()

	testCases := []struct {
		name        string
		configType  string // "global" or "namespace"
		config      string
		shouldFail  bool
		description string
	}{
		// Global config tests
		{
			name:        "global config exceeds system maximum TTL",
			configType:  "global",
			config:      `ttlSecondsAfterFinished: 2592001`,
			shouldFail:  true,
			description: "TTL of 2592001 seconds exceeds system maximum of 2592000 seconds (30 days)",
		},
		{
			name:        "global config at system maximum TTL",
			configType:  "global",
			config:      `ttlSecondsAfterFinished: 2592000`,
			shouldFail:  false,
			description: "TTL of 2592000 seconds is at system maximum",
		},
		{
			name:        "global config exceeds system maximum successfulHistoryLimit",
			configType:  "global",
			config:      `successfulHistoryLimit: 101`,
			shouldFail:  true,
			description: "successfulHistoryLimit of 101 exceeds system maximum of 100",
		},
		{
			name:        "global config at system maximum successfulHistoryLimit",
			configType:  "global",
			config:      `successfulHistoryLimit: 100`,
			shouldFail:  false,
			description: "successfulHistoryLimit of 100 is at system maximum",
		},
		{
			name:        "global config exceeds system maximum failedHistoryLimit",
			configType:  "global",
			config:      `failedHistoryLimit: 150`,
			shouldFail:  true,
			description: "failedHistoryLimit of 150 exceeds system maximum of 100",
		},
		{
			name:        "global config exceeds system maximum historyLimit",
			configType:  "global",
			config:      `historyLimit: 200`,
			shouldFail:  true,
			description: "historyLimit of 200 exceeds system maximum of 100",
		},
		// Namespace config tests (without global limits)
		{
			name:        "namespace config exceeds system maximum TTL without global limit",
			configType:  "namespace",
			config:      `ttlSecondsAfterFinished: 3000000`,
			shouldFail:  true,
			description: "namespace TTL of 3000000 seconds exceeds system maximum without global limit",
		},
		{
			name:        "namespace config within system maximum TTL without global limit",
			configType:  "namespace",
			config:      `ttlSecondsAfterFinished: 2592000`,
			shouldFail:  false,
			description: "namespace TTL of 2592000 seconds is within system maximum",
		},
		{
			name:        "namespace config exceeds system maximum successfulHistoryLimit without global limit",
			configType:  "namespace",
			config:      `successfulHistoryLimit: 150`,
			shouldFail:  true,
			description: "namespace successfulHistoryLimit of 150 exceeds system maximum without global limit",
		},
		{
			name:        "namespace config within system maximum successfulHistoryLimit without global limit",
			configType:  "namespace",
			config:      `successfulHistoryLimit: 100`,
			shouldFail:  false,
			description: "namespace successfulHistoryLimit of 100 is within system maximum",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cm *corev1.ConfigMap
			var isUpdate bool

			if tc.configType == "global" {
				// For global config, we update the existing one
				cm = createGlobalConfigMap(prunerConfigName, prunerNamespace, tc.config)
				isUpdate = true
			} else {
				// For namespace config, we create new ones
				cm = createNamespaceConfigMap("tekton-pruner-namespace-spec", testNamespace, tc.config)
				isUpdate = false

				// Add cleanup
				t.Cleanup(func() {
					kubeClient.CoreV1().ConfigMaps(testNamespace).Delete(context.Background(),
						cm.Name, metav1.DeleteOptions{})
				})
			}

			t.Logf("Testing: %s", tc.description)

			var err error
			if isUpdate {
				err = updateOrCreateConfigMap(ctx, kubeClient, cm)
			} else {
				_, err = kubeClient.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
			}

			if tc.shouldFail {
				if err == nil {
					t.Errorf("Config exceeding system maximum was accepted by webhook when it should have been rejected")
				} else {
					t.Logf("✓ Webhook correctly rejected config exceeding system maximum: %v", err)
				}
			} else {
				if err != nil && !errors.IsAlreadyExists(err) {
					t.Errorf("Valid config within system maximum was rejected by webhook: %v", err)
				} else {
					t.Logf("✓ Webhook correctly accepted config within system maximum")
				}
			}
		})
	}

	// Restore original global config if needed
	t.Cleanup(func() {
		// Reset to a reasonable global config for other tests
		resetConfig := createGlobalConfigMap(prunerConfigName, prunerNamespace, `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10
failedHistoryLimit: 10`)
		updateOrCreateConfigMap(context.Background(), kubeClient, resetConfig)
	})
}

func testWebhookForbiddenNamespaces(ctx context.Context, t *testing.T, kubeClient *kubernetes.Clientset) {
	// ensureClusterReady() already verified webhook is ready, just wait for informer sync
	waitForConfigSync()

	// Test that namespace-level configs are rejected in forbidden namespaces
	// Note: The webhook namespaceSelector already excludes kube-system, kube-public,
	// kube-node-lease, tekton-pipelines, and default from webhook processing.
	// This test focuses on namespaces that match forbidden patterns but aren't
	// explicitly excluded by the selector - these should be caught by our code validation.
	testCases := []struct {
		name               string
		namespace          string
		shouldCreateNS     bool
		expectedRejection  bool
		expectedErrMessage string
	}{
		{
			name:               "kube-custom namespace - should be rejected by code validation",
			namespace:          "kube-custom-test",
			shouldCreateNS:     true,
			expectedRejection:  true,
			expectedErrMessage: "kube-* namespaces",
		},
		{
			name:               "kube-monitoring namespace - should be rejected by code validation",
			namespace:          "kube-monitoring",
			shouldCreateNS:     true,
			expectedRejection:  true,
			expectedErrMessage: "kube-* namespaces",
		},
		{
			name:               "openshift-pipelines namespace - should be rejected by code validation",
			namespace:          "openshift-pipelines-test",
			shouldCreateNS:     true,
			expectedRejection:  true,
			expectedErrMessage: "openshift-* namespaces",
		},
		{
			name:               "openshift-operators namespace - should be rejected by code validation",
			namespace:          "openshift-operators",
			shouldCreateNS:     true,
			expectedRejection:  true,
			expectedErrMessage: "openshift-* namespaces",
		},
		{
			name:               "tekton-custom namespace - should be rejected by code validation",
			namespace:          "tekton-custom-test",
			shouldCreateNS:     true,
			expectedRejection:  true,
			expectedErrMessage: "tekton-* namespaces",
		},
		{
			name:               "tekton-operator namespace - should be rejected by code validation",
			namespace:          "tekton-operator",
			shouldCreateNS:     true,
			expectedRejection:  true,
			expectedErrMessage: "tekton-* namespaces",
		},
		{
			name:              "user namespace - should be allowed",
			namespace:         testNamespace,
			shouldCreateNS:    false, // testNamespace is already created in setup
			expectedRejection: false,
		},
		{
			name:              "dev namespace - should be allowed",
			namespace:         "dev-test-ns",
			shouldCreateNS:    true,
			expectedRejection: false,
		},
		{
			name:              "production namespace - should be allowed",
			namespace:         "production-test-ns",
			shouldCreateNS:    true,
			expectedRejection: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the namespace if needed
			if tc.shouldCreateNS {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: tc.namespace,
					},
				}
				_, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
				if err != nil && !errors.IsAlreadyExists(err) {
					t.Fatalf("Failed to create test namespace %s: %v", tc.namespace, err)
				}

				// Cleanup namespace after test
				t.Cleanup(func() {
					kubeClient.CoreV1().Namespaces().Delete(context.Background(), tc.namespace, metav1.DeleteOptions{})
				})
			}

			// Create a namespace-level config in this namespace
			namespaceConfig := createNamespaceConfigMap("tekton-pruner-namespace-spec", tc.namespace, `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10`)

			// Add cleanup in case it gets created
			t.Cleanup(func() {
				kubeClient.CoreV1().ConfigMaps(tc.namespace).Delete(context.Background(),
					namespaceConfig.Name, metav1.DeleteOptions{})
			})

			// Try to create the ConfigMap
			_, err := kubeClient.CoreV1().ConfigMaps(tc.namespace).Create(ctx, namespaceConfig, metav1.CreateOptions{})

			if tc.expectedRejection {
				if err == nil {
					t.Errorf("Namespace config in forbidden namespace %s was accepted by webhook when it should have been rejected", tc.namespace)
				} else {
					// Check if the error message contains the expected pattern
					if tc.expectedErrMessage != "" {
						errMsg := err.Error()
						if len(errMsg) > 0 && len(tc.expectedErrMessage) > 0 {
							found := false
							for i := 0; i <= len(errMsg)-len(tc.expectedErrMessage); i++ {
								if errMsg[i:i+len(tc.expectedErrMessage)] == tc.expectedErrMessage {
									found = true
									break
								}
							}
							if !found {
								t.Logf("Warning: Error message '%s' does not contain expected pattern '%s'", errMsg, tc.expectedErrMessage)
							}
						}
					}
					t.Logf("✓ Webhook correctly rejected namespace config in forbidden namespace %s: %v", tc.namespace, err)
				}
			} else {
				if err != nil {
					t.Errorf("Valid namespace config in allowed namespace %s was rejected by webhook: %v", tc.namespace, err)
				} else {
					t.Logf("✓ Webhook correctly accepted namespace config in allowed namespace %s", tc.namespace)
				}
			}
		})
	}
}

// Helper functions for waiting on resource deletion

// waitForTaskRunDeletion polls for a TaskRun to be deleted within the configured timeout
func waitForTaskRunDeletion(ctx context.Context, client *clientset.Clientset, name, namespace string) error {
	return wait.PollImmediate(pollingInterval, waitForDeletion, func() (bool, error) {
		_, err := client.TektonV1().TaskRuns(namespace).Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil // Resource deleted, we're done
		}
		if err != nil {
			return false, err // Unexpected error
		}
		return false, nil // Resource still exists, keep polling
	})
}

// waitForPipelineRunDeletion polls for a PipelineRun to be deleted within the configured timeout
func waitForPipelineRunDeletion(ctx context.Context, client *clientset.Clientset, name, namespace string) error {
	return wait.PollImmediate(pollingInterval, waitForDeletion, func() (bool, error) {
		_, err := client.TektonV1().PipelineRuns(namespace).Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil // Resource deleted, we're done
		}
		if err != nil {
			return false, err // Unexpected error
		}
		return false, nil // Resource still exists, keep polling
	})
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
