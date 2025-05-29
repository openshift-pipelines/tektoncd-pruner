package taskrun

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/config"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	fakepipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clocktest "k8s.io/utils/clock/testing"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/logging"
)

func TestTrFuncs_IsCompleted(t *testing.T) {
	tests := []struct {
		name     string
		tr       *pipelinev1.TaskRun
		want     bool
		wantLogs []string
	}{
		{
			name: "Completed with CompletionTime",
			tr: &pipelinev1.TaskRun{
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime:      &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			want: true,
		},
		{
			name: "Completed with Succeeded condition",
			tr: &pipelinev1.TaskRun{
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					},
					Status: duckv1.Status{
						Conditions: []apis.Condition{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			want: true,
		},
		{
			name: "Not completed - no start time",
			tr: &pipelinev1.TaskRun{
				Status: pipelinev1.TaskRunStatus{},
			},
			want: false,
		},
		{
			name: "Not completed - pending",
			tr: &pipelinev1.TaskRun{
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime: &metav1.Time{Time: time.Now()},
					},
					Status: duckv1.Status{
						Conditions: []apis.Condition{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionUnknown,
						}},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trFuncs := &TrFuncs{client: fakepipelineclientset.NewSimpleClientset()}
			if got := trFuncs.IsCompleted(tt.tr); got != tt.want {
				t.Errorf("TrFuncs.IsCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrFuncs_Ignore(t *testing.T) {
	tests := []struct {
		name        string
		tr          *pipelinev1.TaskRun
		want        bool
		description string
	}{
		{
			name: "Has TTL annotation",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
			},
			want:        false,
			description: "TaskRun with TTL annotation should not be ignored",
		},
		{
			name: "No annotations",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want:        true,
			description: "TaskRun with no annotations should be ignored",
		},
		{
			name: "Empty TTL annotation",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "",
					},
				},
			},
			want:        true,
			description: "TaskRun with empty TTL annotation should be ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			trFuncs := &TrFuncs{client: fakepipelineclientset.NewSimpleClientset()}
			if got := trFuncs.Ignore(tt.tr); got != tt.want {
				t.Errorf("TrFuncs.Ignore() = %v, want %v - %s", got, tt.want, tt.description)
			}
		})
	}
}

func TestReconciler_ProcessTaskRun(t *testing.T) {
	fakeClock := clocktest.NewFakeClock(time.Now())

	tests := []struct {
		name            string
		tr              *pipelinev1.TaskRun
		ttl             int32
		successLimit    int32
		failureLimit    int32
		wantDelete      bool
		wantError       bool
		description     string
		setupAdditional func(client *fakepipelineclientset.Clientset)
		validateResults func(t *testing.T, client *fakepipelineclientset.Clientset)
	}{
		{
			name: "TTL exceeded",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tr-2",
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime:      &metav1.Time{Time: fakeClock.Now().Add(-2 * time.Hour)},
						CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-90 * time.Minute)},
					},
					Status: duckv1.Status{
						Conditions: []apis.Condition{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			ttl:          3600,
			successLimit: 5,
			failureLimit: 2,
			wantDelete:   true,
			description:  "TaskRun exceeding TTL should be deleted",
			validateResults: func(t *testing.T, client *fakepipelineclientset.Clientset) {
				// Verify TaskRun deletion due to TTL
				trs, err := client.TektonV1().TaskRuns("default").List(context.Background(), metav1.ListOptions{})
				if err != nil {
					t.Errorf("Failed to list TaskRuns: %v", err)
					return
				}
				t.Logf("Found %d TaskRuns after reconciliation", len(trs.Items))
			},
		},
		{
			name: "Success history limit exceeded",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-3",
					Namespace: "default",
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime:      &metav1.Time{Time: fakeClock.Now().Add(-30 * time.Minute)},
						CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-15 * time.Minute)},
					},
					Status: duckv1.Status{
						Conditions: []apis.Condition{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			ttl:          3600,
			successLimit: 1,
			failureLimit: 2,
			wantDelete:   true,
			setupAdditional: func(client *fakepipelineclientset.Clientset) {
				// Add older successful TaskRuns with proper completion times
				for i := 0; i < 2; i++ {
					_, _ = client.TektonV1().TaskRuns("default").Create(context.Background(), &pipelinev1.TaskRun{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("old-success-%d", i),
							Namespace: "default",
							Annotations: map[string]string{
								config.AnnotationTTLSecondsAfterFinished: "3600",
							},
						},
						Status: pipelinev1.TaskRunStatus{
							TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
								StartTime:      &metav1.Time{Time: fakeClock.Now().Add(-2 * time.Hour)},
								CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-100 * time.Minute)},
							},
							Status: duckv1.Status{
								Conditions: []apis.Condition{{
									Type:   apis.ConditionSucceeded,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					}, metav1.CreateOptions{})
				}
			},
			description: "TaskRun exceeding success history limit should be deleted",
		},
		{
			name: "Failure history limit exceeded",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-4",
					Namespace: "default",
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime:      &metav1.Time{Time: fakeClock.Now().Add(-30 * time.Minute)},
						CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-15 * time.Minute)},
					},
					Status: duckv1.Status{
						Conditions: []apis.Condition{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionFalse,
						}},
					},
				},
			},
			ttl:          3600,
			successLimit: 5,
			failureLimit: 1,
			wantDelete:   true,
			setupAdditional: func(client *fakepipelineclientset.Clientset) {
				// Add older failed TaskRuns with proper completion times
				for i := 0; i < 2; i++ {
					_, _ = client.TektonV1().TaskRuns("default").Create(context.Background(), &pipelinev1.TaskRun{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("old-failure-%d", i),
							Namespace: "default",
							Annotations: map[string]string{
								config.AnnotationTTLSecondsAfterFinished: "3600",
							},
						},
						Status: pipelinev1.TaskRunStatus{
							TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
								StartTime:      &metav1.Time{Time: fakeClock.Now().Add(-2 * time.Hour)},
								CompletionTime: &metav1.Time{Time: fakeClock.Now().Add(-100 * time.Minute)},
							},
							Status: duckv1.Status{
								Conditions: []apis.Condition{{
									Type:   apis.ConditionSucceeded,
									Status: corev1.ConditionFalse,
								}},
							},
						},
					}, metav1.CreateOptions{})
				}
			},
			description: "TaskRun exceeding failure history limit should be deleted",
			validateResults: func(t *testing.T, client *fakepipelineclientset.Clientset) {
				// List all TaskRuns in the namespace to verify the state
				trs, err := client.TektonV1().TaskRuns("default").List(context.Background(), metav1.ListOptions{})
				if err != nil {
					t.Errorf("Failed to list TaskRuns: %v", err)
					return
				}

				// Count failed TaskRuns
				failedCount := 0
				for _, tr := range trs.Items {
					for _, cond := range tr.Status.Conditions {
						if cond.Type == apis.ConditionSucceeded && cond.Status == corev1.ConditionFalse {
							failedCount++
							t.Logf("Found failed TaskRun: %s with completion time %v", tr.Name, tr.Status.CompletionTime)
						}
					}
				}
				t.Logf("Found %d failed TaskRuns", failedCount)

				// We should have exactly 1 failed TaskRun (the failureLimit)
				if failedCount > 1 {
					t.Errorf("Expected at most 1 failed TaskRun, but got %d", failedCount)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			// Setup fake clients
			pipelineClient := fakepipelineclientset.NewSimpleClientset(tt.tr)
			if tt.setupAdditional != nil {
				tt.setupAdditional(pipelineClient)
			}
			kubeClient := fake.NewSimpleClientset()

			// Setup config
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.PrunerConfigMapName,
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					"global-config": fmt.Sprintf(`
enforcedConfigLevel: global
ttlSecondsAfterFinished: %d
successfulHistoryLimit: %d
failedHistoryLimit: %d`, tt.ttl, tt.successLimit, tt.failureLimit),
				},
			}

			if err := config.PrunerConfigStore.LoadGlobalConfig(ctx, cm); err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Create reconciler with handlers
			trFuncs := &TrFuncs{client: pipelineClient}
			ttlHandler, err := config.NewTTLHandler(fakeClock, trFuncs)
			if err != nil {
				t.Fatalf("Failed to create TTLHandler: %v", err)
			}
			historyLimiter, err := config.NewHistoryLimiter(trFuncs)
			if err != nil {
				t.Fatalf("Failed to create HistoryLimiter: %v", err)
			}

			r := &Reconciler{
				kubeclient:     kubeClient,
				ttlHandler:     ttlHandler,
				historyLimiter: historyLimiter,
			}

			// List TaskRuns before reconciliation
			trsBefore, err := pipelineClient.TektonV1().TaskRuns("default").List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Errorf("Failed to list TaskRuns before reconciliation: %v", err)
				return
			}
			t.Logf("[%s] Before reconciliation: Found %d TaskRuns", tt.name, len(trsBefore.Items))
			for _, tr := range trsBefore.Items {
				for _, cond := range tr.Status.Conditions {
					if cond.Type == apis.ConditionSucceeded {
						t.Logf("[%s] TaskRun %s: success=%v completion=%v", tt.name, tr.Name, cond.Status == corev1.ConditionTrue, tr.Status.CompletionTime)
					}
				}
			}

			// Execute reconciliation
			err = r.ReconcileKind(ctx, tt.tr)
			if err != nil {
				// Skip requeue errors - they're expected when TTL hasn't expired yet
				if !strings.Contains(err.Error(), "requeue after:") && tt.wantError != true {
					t.Errorf("ReconcileKind() error = %v, wantError %v", err, tt.wantError)
					return
				}
			} else if tt.wantError {
				t.Errorf("ReconcileKind() expected error but got none")
				return
			}

			// List TaskRuns after reconciliation
			trsAfter, err := pipelineClient.TektonV1().TaskRuns("default").List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Errorf("Failed to list TaskRuns after reconciliation: %v", err)
				return
			}
			t.Logf("[%s] After reconciliation: Found %d TaskRuns", tt.name, len(trsAfter.Items))
			for _, tr := range trsAfter.Items {
				for _, cond := range tr.Status.Conditions {
					if cond.Type == apis.ConditionSucceeded {
						t.Logf("[%s] TaskRun %s: success=%v completion=%v", tt.name, tr.Name, cond.Status == corev1.ConditionTrue, tr.Status.CompletionTime)
					}
				}
			}

			// Check if the TaskRun was deleted as expected
			_, err = pipelineClient.TektonV1().TaskRuns("default").Get(ctx, tt.tr.Name, metav1.GetOptions{})
			isDeleted := errors.IsNotFound(err)

			if isDeleted != tt.wantDelete {
				t.Errorf("TaskRun deletion state = %v, want %v - %s", isDeleted, tt.wantDelete, tt.description)
			}

			// Run additional validation if provided
			if tt.validateResults != nil {
				tt.validateResults(t, pipelineClient)
			}
		})
	}
}
