package pipelinerun

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

func TestReconcilePipelineRun(t *testing.T) {
	fakeClock := clocktest.NewFakeClock(time.Now())

	tests := []struct {
		name string
		pr   *pipelinev1.PipelineRun
		ttl  int32
		want bool // true if PipelineRun should be deleted
	}{
		{
			name: "Completed PipelineRun with TTL exceeded",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-1",
					Namespace: "default",
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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
			name: "Completed PipelineRun within TTL",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-2",
					Namespace: "default",
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			var err error

			// Create fake clients
			pipelineClient := fakepipelineclientset.NewSimpleClientset(tt.pr)
			kubeClient := fake.NewSimpleClientset()

			prFuncs := &PrFuncs{client: pipelineClient}

			// Create reconciler
			r := &Reconciler{
				kubeclient: kubeClient,
			}

			// Initialize TTLHandler and HistoryLimiter properly
			var ttlHandler *config.TTLHandler
			ttlHandler, err = config.NewTTLHandler(fakeClock, prFuncs)
			if err != nil {
				t.Fatalf("Failed to create TTLHandler: %v", err)
			}
			r.ttlHandler = ttlHandler

			var historyLimiter *config.HistoryLimiter
			historyLimiter, err = config.NewHistoryLimiter(prFuncs)
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
			if err := r.ReconcileKind(ctx, tt.pr); err != nil {
				t.Errorf("ReconcileKind() error = %v", err)
			}

			// Check if PipelineRun was deleted
			_, err = pipelineClient.TektonV1().PipelineRuns(tt.pr.Namespace).Get(ctx, tt.pr.Name, metav1.GetOptions{})
			if tt.want {
				if err == nil {
					t.Error("Expected PipelineRun to be deleted")
				}
			} else {
				if err != nil {
					t.Errorf("Expected PipelineRun to exist, got error: %v", err)
				}
			}
		})
	}
}

func TestHistoryLimitReconciliation(t *testing.T) {
	tests := []struct {
		name            string
		prs             []*pipelinev1.PipelineRun
		successLimit    int32
		failureLimit    int32
		wantDeleteCount int
	}{
		{
			name: "Exceed success history limit",
			prs: []*pipelinev1.PipelineRun{
				createTestPR("pr-1", true, time.Now().Add(-3*time.Hour)),
				createTestPR("pr-2", true, time.Now().Add(-2*time.Hour)),
				createTestPR("pr-3", true, time.Now().Add(-1*time.Hour)),
			},
			successLimit:    2,
			failureLimit:    1,
			wantDeleteCount: 1, // Oldest successful PR should be deleted
		},
		{
			name: "Exceed failure history limit",
			prs: []*pipelinev1.PipelineRun{
				createTestPR("pr-1", false, time.Now().Add(-3*time.Hour)),
				createTestPR("pr-2", false, time.Now().Add(-2*time.Hour)),
				createTestPR("pr-3", true, time.Now().Add(-1*time.Hour)),
			},
			successLimit:    2,
			failureLimit:    1,
			wantDeleteCount: 1, // Oldest failed PR should be deleted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			// Create fake clients with PipelineRuns
			pipelineClient := fakepipelineclientset.NewSimpleClientset()
			for _, pr := range tt.prs {
				_, err := pipelineClient.TektonV1().PipelineRuns(pr.Namespace).Create(ctx, pr, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create PipelineRun: %v", err)
				}
			}

			prFuncs := &PrFuncs{client: pipelineClient}

			// Create reconciler
			r := &Reconciler{
				kubeclient: fake.NewSimpleClientset(),
			}

			// Initialize HistoryLimiter properly
			historyLimiter, err := config.NewHistoryLimiter(prFuncs)
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

			// Reconcile each PipelineRun
			for _, pr := range tt.prs {
				if err := r.ReconcileKind(ctx, pr); err != nil {
					t.Errorf("ReconcileKind() error = %v", err)
				}
			}

			// Count remaining PipelineRuns
			prList, err := pipelineClient.TektonV1().PipelineRuns("default").List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("Failed to list PipelineRuns: %v", err)
			}

			deletedCount := len(tt.prs) - len(prList.Items)
			if deletedCount != tt.wantDeleteCount {
				t.Errorf("Got %d deleted PipelineRuns, want %d", deletedCount, tt.wantDeleteCount)
			}
		})
	}
}

func createTestPR(name string, successful bool, completionTime time.Time) *pipelinev1.PipelineRun {
	status := corev1.ConditionTrue
	if !successful {
		status = corev1.ConditionFalse
	}

	return &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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
