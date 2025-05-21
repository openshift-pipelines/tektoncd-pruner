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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clocktest "k8s.io/utils/clock/testing"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/logging"
)

func TestPrFuncs_IsCompleted(t *testing.T) {
	tests := []struct {
		name     string
		pr       *pipelinev1.PipelineRun
		want     bool
		wantLogs []string
	}{
		{
			name: "Completed with CompletionTime",
			pr: &pipelinev1.PipelineRun{
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						StartTime:      &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
			want: true,
		},
		{
			name: "Completed with Succeeded condition",
			pr: &pipelinev1.PipelineRun{
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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
			pr: &pipelinev1.PipelineRun{
				Status: pipelinev1.PipelineRunStatus{},
			},
			want: false,
		},
		{
			name: "Not completed - pending",
			pr: &pipelinev1.PipelineRun{
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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

			prFuncs := &PrFuncs{client: fakepipelineclientset.NewSimpleClientset()}
			if got := prFuncs.IsCompleted(tt.pr); got != tt.want {
				t.Errorf("PrFuncs.IsCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrFuncs_Ignore(t *testing.T) {
	tests := []struct {
		name        string
		pr          *pipelinev1.PipelineRun
		want        bool
		description string
	}{
		{
			name: "Has TTL annotation",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
			},
			want:        false,
			description: "PipelineRun with TTL annotation should not be ignored",
		},
		{
			name: "No annotations",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want:        true,
			description: "PipelineRun with no annotations should be ignored",
		},
		{
			name: "Empty TTL annotation",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "",
					},
				},
			},
			want:        true,
			description: "PipelineRun with empty TTL annotation should be ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			prFuncs := &PrFuncs{client: fakepipelineclientset.NewSimpleClientset()}
			if got := prFuncs.Ignore(tt.pr); got != tt.want {
				t.Errorf("PrFuncs.Ignore() = %v, want %v - %s", got, tt.want, tt.description)
			}
		})
	}
}

func TestReconciler_ProcessPipelineRun(t *testing.T) {
	fakeClock := clocktest.NewFakeClock(time.Now())

	tests := []struct {
		name            string
		pr              *pipelinev1.PipelineRun
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
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pr-2",
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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
			description:  "PipelineRun exceeding TTL should be deleted",
		},
		{
			name: "Success history limit exceeded",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pr-3",
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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
				// Add older successful PipelineRuns
				for i := 0; i < 2; i++ {
					_, _ = client.TektonV1().PipelineRuns("default").Create(context.Background(), &pipelinev1.PipelineRun{
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("old-success-%d", i),
							Annotations: map[string]string{
								config.AnnotationTTLSecondsAfterFinished: "3600",
							},
						},
						Status: pipelinev1.PipelineRunStatus{
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
			description: "PipelineRun exceeding success history limit should be deleted",
		},
		{
			name: "Failure history limit exceeded",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pr-4",
					Annotations: map[string]string{
						config.AnnotationTTLSecondsAfterFinished: "3600",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
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
				// Add older failed PipelineRuns
				for i := 0; i < 2; i++ {
					_, _ = client.TektonV1().PipelineRuns("default").Create(context.Background(), &pipelinev1.PipelineRun{
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("old-failure-%d", i),
							Annotations: map[string]string{
								config.AnnotationTTLSecondsAfterFinished: "3600",
							},
						},
						Status: pipelinev1.PipelineRunStatus{
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
			description: "PipelineRun exceeding failure history limit should be deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zaptest.NewLogger(t).Sugar()
			ctx = logging.WithLogger(ctx, logger)

			// Setup fake clients
			pipelineClient := fakepipelineclientset.NewSimpleClientset(tt.pr)
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
			prFuncs := &PrFuncs{client: pipelineClient}
			ttlHandler, err := config.NewTTLHandler(fakeClock, prFuncs)
			if err != nil {
				t.Fatalf("Failed to create TTLHandler: %v", err)
			}
			historyLimiter, err := config.NewHistoryLimiter(prFuncs)
			if err != nil {
				t.Fatalf("Failed to create HistoryLimiter: %v", err)
			}

			r := &Reconciler{
				kubeclient:     kubeClient,
				ttlHandler:     ttlHandler,
				historyLimiter: historyLimiter,
			}

			// Execute reconciliation
			err = r.ReconcileKind(ctx, tt.pr)
			if (err != nil) != tt.wantError {
				t.Errorf("ReconcileKind() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// Check if the PipelineRun was deleted as expected
			_, err = pipelineClient.TektonV1().PipelineRuns("default").Get(ctx, tt.pr.Name, metav1.GetOptions{})
			isDeleted := errors.IsNotFound(err)

			if isDeleted != tt.wantDelete {
				t.Errorf("PipelineRun deletion state = %v, want %v - %s", isDeleted, tt.wantDelete, tt.description)
			}

			// Run additional validation if provided
			if tt.validateResults != nil {
				tt.validateResults(t, pipelineClient)
			}
		})
	}
}
