package taskrun

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	fakepipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clocktest "k8s.io/utils/clock/testing"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/logging"
	rtesting "knative.dev/pkg/reconciler/testing"
)

func TestNewController(t *testing.T) {
	ctx, _ := rtesting.SetupFakeContext(t)
	c := NewController(ctx, configmap.NewStaticWatcher())
	if c == nil {
		t.Error("Expected NewController to return a non-nil value")
	}
}

func TestReconcileTaskRun(t *testing.T) {
	fakeClock := clocktest.NewFakeClock(time.Now())

	tests := []struct {
		name string
		tr   *pipelinev1.TaskRun
		ttl  int32
		want bool // true if TaskRun should be deleted
	}{
		{
			name: "Standalone TaskRun with TTL exceeded",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tr-1",
					Namespace: "default",
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-2 * time.Hour)},
					},
					Status: duckv1.Status{
						Conditions: []apis.Condition{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			ttl:  3600, // 1 hour
			want: true,
		},
		{
			name: "TaskRun within TTL",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tr-2",
					Namespace: "default",
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-30 * time.Minute)},
					},
					Status: duckv1.Status{
						Conditions: []apis.Condition{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			ttl:  3600,
			want: false,
		},
		{
			name: "Non-standalone TaskRun",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tr-3",
					Namespace: "default",
					Labels: map[string]string{
						"tekton.dev/pipelineRun": "parent-pipeline",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-2 * time.Hour)},
					},
				},
			},
			ttl:  3600,
			want: false, // Should not be deleted as it's part of a PipelineRun
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			var err error

			// Create fake clients
			pipelineClient := fakepipelineclientset.NewSimpleClientset(tt.tr)
			kubeClient := fake.NewSimpleClientset()

			trFuncs := &TrFuncs{client: pipelineClient}

			// Create reconciler
			r := &Reconciler{
				kubeclient: kubeClient,
			}

			// Initialize TTLHandler and HistoryLimiter properly
			var ttlHandler *config.TTLHandler
			ttlHandler, err = config.NewTTLHandler(fakeClock, trFuncs)
			if err != nil {
				t.Fatalf("Failed to create TTLHandler: %v", err)
			}
			r.ttlHandler = ttlHandler

			var historyLimiter *config.HistoryLimiter
			historyLimiter, err = config.NewHistoryLimiter(trFuncs)
			if err != nil {
				t.Fatalf("Failed to create HistoryLimiter: %v", err)
			}
			r.historyLimiter = historyLimiter

			// Create config map with TTL setting
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.PrunerConfigMapName,
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					"global-config": fmt.Sprintf(`enforcedConfigLevel: global
ttlSecondsAfterFinished: %d`, tt.ttl),
				},
			}

			// Load config
			if err := config.PrunerConfigStore.LoadGlobalConfig(ctx, cm); err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Reconcile
			if err := r.ReconcileKind(ctx, tt.tr); err != nil {
				t.Errorf("ReconcileKind() error = %v", err)
			}

			// Check if TaskRun was deleted
			_, err = pipelineClient.TektonV1().TaskRuns(tt.tr.Namespace).Get(ctx, tt.tr.Name, metav1.GetOptions{})
			if tt.want {
				if err == nil {
					t.Error("Expected TaskRun to be deleted")
				}
			} else {
				if err != nil {
					t.Errorf("Expected TaskRun to exist, got error: %v", err)
				}
			}
		})
	}
}

func TestHistoryLimitReconciliation(t *testing.T) {
	tests := []struct {
		name            string
		trs             []*pipelinev1.TaskRun
		successLimit    int32
		failureLimit    int32
		wantDeleteCount int
	}{
		{
			name: "Exceed success history limit",
			trs: []*pipelinev1.TaskRun{
				createTestTR("tr-1", true, time.Now().Add(-3*time.Hour)),
				createTestTR("tr-2", true, time.Now().Add(-2*time.Hour)),
				createTestTR("tr-3", true, time.Now().Add(-1*time.Hour)),
			},
			successLimit:    2,
			failureLimit:    1,
			wantDeleteCount: 1, // Oldest successful TR should be deleted
		},
		{
			name: "Exceed failure history limit",
			trs: []*pipelinev1.TaskRun{
				createTestTR("tr-1", false, time.Now().Add(-3*time.Hour)),
				createTestTR("tr-2", false, time.Now().Add(-2*time.Hour)),
				createTestTR("tr-3", true, time.Now().Add(-1*time.Hour)),
			},
			successLimit:    2,
			failureLimit:    1,
			wantDeleteCount: 1, // Oldest failed TR should be deleted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			// Create fake clients with TaskRuns
			pipelineClient := fakepipelineclientset.NewSimpleClientset()
			for _, tr := range tt.trs {
				_, err := pipelineClient.TektonV1().TaskRuns(tr.Namespace).Create(ctx, tr, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create TaskRun: %v", err)
				}
			}

			trFuncs := &TrFuncs{client: pipelineClient}

			// Create reconciler
			r := &Reconciler{
				kubeclient: fake.NewSimpleClientset(),
			}

			// Initialize HistoryLimiter properly
			historyLimiter, err := config.NewHistoryLimiter(trFuncs)
			if err != nil {
				t.Fatalf("Failed to create HistoryLimiter: %v", err)
			}
			r.historyLimiter = historyLimiter

			// Create config map with history limits
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.PrunerConfigMapName,
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					"global-config": fmt.Sprintf(`enforcedConfigLevel: global
successfulHistoryLimit: %d
failedHistoryLimit: %d`, tt.successLimit, tt.failureLimit),
				},
			}

			// Load config
			if err := config.PrunerConfigStore.LoadGlobalConfig(ctx, cm); err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Reconcile each TaskRun
			for _, tr := range tt.trs {
				if err := r.ReconcileKind(ctx, tr); err != nil {
					t.Errorf("ReconcileKind() error = %v", err)
				}
			}

			// Count remaining TaskRuns
			trList, err := pipelineClient.TektonV1().TaskRuns("default").List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("Failed to list TaskRuns: %v", err)
			}

			deletedCount := len(tt.trs) - len(trList.Items)
			if deletedCount != tt.wantDeleteCount {
				t.Errorf("Got %d deleted TaskRuns, want %d", deletedCount, tt.wantDeleteCount)
			}
		})
	}
}

func TestTaskRunFilter(t *testing.T) {
	tests := []struct {
		name string
		tr   *pipelinev1.TaskRun
		want bool // true if TaskRun should be processed
	}{
		{
			name: "Standalone TaskRun",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standalone-tr",
				},
			},
			want: true,
		},
		{
			name: "TaskRun with PipelineRun label",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pipeline-tr",
					Labels: map[string]string{
						"tekton.dev/pipelineRun": "parent-pipeline",
					},
				},
			},
			want: false,
		},
		{
			name: "TaskRun with PipelineRun owner",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "owned-tr",
					OwnerReferences: []metav1.OwnerReference{{
						Kind: "PipelineRun",
					}},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStandaloneTaskRun(tt.tr)
			if got != tt.want {
				t.Errorf("isStandaloneTaskRun() = %v, want %v", got, tt.want)
			}
		})
	}
}

func createTestTR(name string, successful bool, completionTime time.Time) *pipelinev1.TaskRun {
	status := corev1.ConditionTrue
	if !successful {
		status = corev1.ConditionFalse
	}

	return &pipelinev1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Status: pipelinev1.TaskRunStatus{
			TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
				CompletionTime: &metav1.Time{Time: completionTime},
			},
			Status: duckv1.Status{
				Conditions: []apis.Condition{{
					Type:   apis.ConditionSucceeded,
					Status: status,
				}},
			},
		},
	}
}
