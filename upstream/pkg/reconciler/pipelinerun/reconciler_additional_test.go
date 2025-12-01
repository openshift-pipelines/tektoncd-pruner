package pipelinerun

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	fakepipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
)

func TestNewPrFuncs(t *testing.T) {
	client := fakepipelineclientset.NewSimpleClientset()
	prFuncs := NewPrFuncs(client)

	assert.NotNil(t, prFuncs)
	assert.NotNil(t, prFuncs.client)
	assert.Equal(t, client, prFuncs.client)
}

func TestListByLabels(t *testing.T) {
	tests := []struct {
		name          string
		prs           []*pipelinev1.PipelineRun
		namespace     string
		labels        map[string]string
		expectedCount int
	}{
		{
			name:      "No PipelineRuns",
			prs:       []*pipelinev1.PipelineRun{},
			namespace: "default",
			labels: map[string]string{
				"app": "test",
			},
			expectedCount: 0,
		},
		{
			name: "Single matching PipelineRun",
			prs: []*pipelinev1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-1",
						Namespace: "default",
						Labels: map[string]string{
							"app": "test",
						},
					},
				},
			},
			namespace: "default",
			labels: map[string]string{
				"app": "test",
			},
			expectedCount: 1,
		},
		{
			name: "Multiple matching PipelineRuns",
			prs: []*pipelinev1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-1",
						Namespace: "default",
						Labels: map[string]string{
							"app": "test",
							"env": "prod",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-2",
						Namespace: "default",
						Labels: map[string]string{
							"app": "test",
							"env": "prod",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-3",
						Namespace: "default",
						Labels: map[string]string{
							"app": "other",
						},
					},
				},
			},
			namespace: "default",
			labels: map[string]string{
				"app": "test",
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake client with PipelineRuns
			var runtimeObjs []runtime.Object
			for _, pr := range tt.prs {
				runtimeObjs = append(runtimeObjs, pr)
			}
			client := fakepipelineclientset.NewSimpleClientset(runtimeObjs...)
			prFuncs := NewPrFuncs(client)

			// Call ListByLabels
			result, err := prFuncs.ListByLabels(ctx, tt.namespace, tt.labels)

			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestListByAnnotations(t *testing.T) {
	tests := []struct {
		name          string
		prs           []*pipelinev1.PipelineRun
		namespace     string
		annotations   map[string]string
		expectedCount int
	}{
		{
			name:      "No PipelineRuns",
			prs:       []*pipelinev1.PipelineRun{},
			namespace: "default",
			annotations: map[string]string{
				"owner": "team-a",
			},
			expectedCount: 0,
		},
		{
			name: "Single matching PipelineRun",
			prs: []*pipelinev1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-1",
						Namespace: "default",
						Annotations: map[string]string{
							"owner": "team-a",
						},
					},
				},
			},
			namespace: "default",
			annotations: map[string]string{
				"owner": "team-a",
			},
			expectedCount: 1,
		},
		{
			name: "Multiple matching PipelineRuns",
			prs: []*pipelinev1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-1",
						Namespace: "default",
						Annotations: map[string]string{
							"owner":   "team-a",
							"project": "myproject",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-2",
						Namespace: "default",
						Annotations: map[string]string{
							"owner":   "team-a",
							"project": "myproject",
						},
					},
				},
			},
			namespace: "default",
			annotations: map[string]string{
				"owner": "team-a",
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake client with PipelineRuns
			var runtimeObjs []runtime.Object
			for _, pr := range tt.prs {
				runtimeObjs = append(runtimeObjs, pr)
			}
			client := fakepipelineclientset.NewSimpleClientset(runtimeObjs...)
			prFuncs := NewPrFuncs(client)

			// Call ListByAnnotations
			result, err := prFuncs.ListByAnnotations(ctx, tt.namespace, tt.annotations)

			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestListByNamespaces(t *testing.T) {
	tests := []struct {
		name               string
		prs                []*pipelinev1.PipelineRun
		namespaces         []string
		expectedNamespaces int
	}{
		{
			name:               "No namespaces",
			prs:                []*pipelinev1.PipelineRun{},
			namespaces:         []string{},
			expectedNamespaces: 0,
		},
		{
			name: "Single namespace with PipelineRuns",
			prs: []*pipelinev1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-1",
						Namespace: "ns1",
					},
				},
			},
			namespaces:         []string{"ns1"},
			expectedNamespaces: 1,
		},
		{
			name: "Multiple namespaces",
			prs: []*pipelinev1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-1",
						Namespace: "ns1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-2",
						Namespace: "ns2",
					},
				},
			},
			namespaces:         []string{"ns1", "ns2"},
			expectedNamespaces: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake client with PipelineRuns
			var runtimeObjs []runtime.Object
			for _, pr := range tt.prs {
				runtimeObjs = append(runtimeObjs, pr)
			}
			client := fakepipelineclientset.NewSimpleClientset(runtimeObjs...)
			prFuncs := NewPrFuncs(client)

			// Call ListByNamespaces
			result, err := prFuncs.ListByNamespaces(ctx, tt.namespaces)

			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedNamespaces)
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name        string
		pr          *pipelinev1.PipelineRun
		updateFunc  func(*pipelinev1.PipelineRun)
		expectError bool
	}{
		{
			name: "Successful update with annotation",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-1",
					Namespace: "default",
				},
			},
			updateFunc: func(pr *pipelinev1.PipelineRun) {
				if pr.Annotations == nil {
					pr.Annotations = make(map[string]string)
				}
				pr.Annotations["test-annotation"] = "true"
			},
			expectError: false,
		},
		{
			name: "Update with label",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-2",
					Namespace: "default",
				},
			},
			updateFunc: func(pr *pipelinev1.PipelineRun) {
				if pr.Labels == nil {
					pr.Labels = make(map[string]string)
				}
				pr.Labels["updated"] = "true"
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake client with PipelineRun
			client := fakepipelineclientset.NewSimpleClientset(tt.pr)
			prFuncs := NewPrFuncs(client)

			// Apply update function
			if tt.updateFunc != nil {
				tt.updateFunc(tt.pr)
			}

			// Call Update
			err := prFuncs.Update(ctx, tt.pr)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify the update
				updated, err := client.TektonV1().PipelineRuns(tt.pr.Namespace).Get(ctx, tt.pr.Name, metav1.GetOptions{})
				assert.NoError(t, err)
				assert.Equal(t, tt.pr.Annotations, updated.Annotations)
				assert.Equal(t, tt.pr.Labels, updated.Labels)
			}
		})
	}
}

func TestGetCompletionTime(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name         string
		pr           *pipelinev1.PipelineRun
		expectedTime *metav1.Time
	}{
		{
			name: "PipelineRun with completion time",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-1",
					Namespace: "default",
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						CompletionTime: &now,
					},
				},
			},
			expectedTime: &now,
		},
		{
			name: "PipelineRun without completion time",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-2",
					Namespace: "default",
				},
			},
			expectedTime: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			client := fakepipelineclientset.NewSimpleClientset(tt.pr)
			prFuncs := NewPrFuncs(client)

			// Call GetCompletionTime
			completionTime, err := prFuncs.GetCompletionTime(tt.pr)

			if tt.expectedTime == nil {
				// When no completion time, expect error and zero time
				assert.Error(t, err)
				assert.True(t, completionTime.IsZero())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTime.Time, completionTime.Time)
			}
		})
	}
}

func TestIgnore(t *testing.T) {
	now := metav1.Now()
	earlier := metav1.NewTime(now.Add(-1 * time.Hour))

	tests := []struct {
		name   string
		pr     *pipelinev1.PipelineRun
		expect bool
	}{
		{
			name: "Completed PipelineRun should not be ignored",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-1",
					Namespace: "default",
					Labels: map[string]string{
						"app": "test",
					},
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						StartTime:      &earlier,
						CompletionTime: &now,
					},
				},
			},
			expect: false,
		},
		{
			name: "Running PipelineRun should be ignored",
			pr: &pipelinev1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-2",
					Namespace: "default",
				},
				Status: pipelinev1.PipelineRunStatus{
					PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
						StartTime: &earlier,
					},
				},
			},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			client := fakepipelineclientset.NewSimpleClientset(tt.pr)
			prFuncs := NewPrFuncs(client)

			// Call Ignore (cast to metav1.Object)
			result := prFuncs.Ignore(metav1.Object(tt.pr))

			assert.Equal(t, tt.expect, result)
		})
	}
}
