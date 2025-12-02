package taskrun

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

func TestNewTrFuncs(t *testing.T) {
	client := fakepipelineclientset.NewSimpleClientset()
	trFuncs := NewTrFuncs(client)

	assert.NotNil(t, trFuncs)
	assert.NotNil(t, trFuncs.client)
	assert.Equal(t, client, trFuncs.client)
}

func TestTaskRun_ListByLabels(t *testing.T) {
	tests := []struct {
		name          string
		trs           []*pipelinev1.TaskRun
		namespace     string
		labels        map[string]string
		expectedCount int
	}{
		{
			name:      "No TaskRuns",
			trs:       []*pipelinev1.TaskRun{},
			namespace: "default",
			labels: map[string]string{
				"app": "test",
			},
			expectedCount: 0,
		},
		{
			name: "Single matching TaskRun",
			trs: []*pipelinev1.TaskRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-1",
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
			name: "Multiple matching TaskRuns",
			trs: []*pipelinev1.TaskRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-1",
						Namespace: "default",
						Labels: map[string]string{
							"app": "test",
							"env": "prod",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-2",
						Namespace: "default",
						Labels: map[string]string{
							"app": "test",
							"env": "dev",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-3",
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
		{
			name: "No matching TaskRuns",
			trs: []*pipelinev1.TaskRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-1",
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
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake client with TaskRuns
			var runtimeObjs []runtime.Object
			for _, tr := range tt.trs {
				runtimeObjs = append(runtimeObjs, tr)
			}
			client := fakepipelineclientset.NewSimpleClientset(runtimeObjs...)
			trFuncs := NewTrFuncs(client)

			// Call ListByLabels
			result, err := trFuncs.ListByLabels(ctx, tt.namespace, tt.labels)

			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestTaskRun_ListByAnnotations(t *testing.T) {
	tests := []struct {
		name          string
		trs           []*pipelinev1.TaskRun
		namespace     string
		annotations   map[string]string
		expectedCount int
	}{
		{
			name:      "No TaskRuns",
			trs:       []*pipelinev1.TaskRun{},
			namespace: "default",
			annotations: map[string]string{
				"owner": "team-a",
			},
			expectedCount: 0,
		},
		{
			name: "Single matching TaskRun",
			trs: []*pipelinev1.TaskRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-1",
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
			name: "Multiple matching TaskRuns",
			trs: []*pipelinev1.TaskRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-1",
						Namespace: "default",
						Annotations: map[string]string{
							"owner":   "team-a",
							"project": "myproject",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-2",
						Namespace: "default",
						Annotations: map[string]string{
							"owner":   "team-a",
							"project": "otherproject",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-3",
						Namespace: "default",
						Annotations: map[string]string{
							"owner": "team-b",
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

			// Create fake client with TaskRuns
			var runtimeObjs []runtime.Object
			for _, tr := range tt.trs {
				runtimeObjs = append(runtimeObjs, tr)
			}
			client := fakepipelineclientset.NewSimpleClientset(runtimeObjs...)
			trFuncs := NewTrFuncs(client)

			// Call ListByAnnotations
			result, err := trFuncs.ListByAnnotations(ctx, tt.namespace, tt.annotations)

			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestTaskRun_ListByNamespaces(t *testing.T) {
	tests := []struct {
		name               string
		trs                []*pipelinev1.TaskRun
		namespaces         []string
		expectedNamespaces int
	}{
		{
			name:               "No namespaces",
			trs:                []*pipelinev1.TaskRun{},
			namespaces:         []string{},
			expectedNamespaces: 0,
		},
		{
			name: "Single namespace with TaskRuns",
			trs: []*pipelinev1.TaskRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-1",
						Namespace: "ns1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-2",
						Namespace: "ns1",
					},
				},
			},
			namespaces:         []string{"ns1"},
			expectedNamespaces: 1,
		},
		{
			name: "Multiple namespaces",
			trs: []*pipelinev1.TaskRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-1",
						Namespace: "ns1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-2",
						Namespace: "ns2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tr-3",
						Namespace: "ns3",
					},
				},
			},
			namespaces:         []string{"ns1", "ns2", "ns3"},
			expectedNamespaces: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake client with TaskRuns
			var runtimeObjs []runtime.Object
			for _, tr := range tt.trs {
				runtimeObjs = append(runtimeObjs, tr)
			}
			client := fakepipelineclientset.NewSimpleClientset(runtimeObjs...)
			trFuncs := NewTrFuncs(client)

			// Call ListByNamespaces
			result, err := trFuncs.ListByNamespaces(ctx, tt.namespaces)

			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedNamespaces)

			// Verify each namespace has a result
			for _, ns := range tt.namespaces {
				_, exists := result[ns]
				assert.True(t, exists, "Namespace %s should exist in results", ns)
			}
		})
	}
}

func TestTaskRun_Update(t *testing.T) {
	tests := []struct {
		name        string
		tr          *pipelinev1.TaskRun
		updateFunc  func(*pipelinev1.TaskRun)
		expectError bool
	}{
		{
			name: "Successful update with annotation",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-1",
					Namespace: "default",
				},
			},
			updateFunc: func(tr *pipelinev1.TaskRun) {
				if tr.Annotations == nil {
					tr.Annotations = make(map[string]string)
				}
				tr.Annotations["test-annotation"] = "true"
			},
			expectError: false,
		},
		{
			name: "Update with label",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-2",
					Namespace: "default",
				},
			},
			updateFunc: func(tr *pipelinev1.TaskRun) {
				if tr.Labels == nil {
					tr.Labels = make(map[string]string)
				}
				tr.Labels["updated"] = "true"
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logtesting.TestLogger(t)
			ctx = logging.WithLogger(ctx, logger)

			// Create fake client with TaskRun
			client := fakepipelineclientset.NewSimpleClientset(tt.tr)
			trFuncs := NewTrFuncs(client)

			// Apply update function
			if tt.updateFunc != nil {
				tt.updateFunc(tt.tr)
			}

			// Call Update
			err := trFuncs.Update(ctx, tt.tr)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify the update
				updated, err := client.TektonV1().TaskRuns(tt.tr.Namespace).Get(ctx, tt.tr.Name, metav1.GetOptions{})
				assert.NoError(t, err)
				assert.Equal(t, tt.tr.Annotations, updated.Annotations)
				assert.Equal(t, tt.tr.Labels, updated.Labels)
			}
		})
	}
}

func TestTaskRun_GetCompletionTime(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name         string
		tr           *pipelinev1.TaskRun
		expectedTime *metav1.Time
	}{
		{
			name: "TaskRun with completion time",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-1",
					Namespace: "default",
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						CompletionTime: &now,
					},
				},
			},
			expectedTime: &now,
		},
		{
			name: "TaskRun without completion time",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-2",
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

			client := fakepipelineclientset.NewSimpleClientset(tt.tr)
			trFuncs := NewTrFuncs(client)

			// Call GetCompletionTime
			completionTime, err := trFuncs.GetCompletionTime(tt.tr)

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

func TestIsStandaloneTaskRun(t *testing.T) {
	tests := []struct {
		name     string
		tr       metav1.Object
		expected bool
	}{
		{
			name: "Standalone TaskRun without owner reference",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-1",
					Namespace: "default",
				},
			},
			expected: true,
		},
		{
			name: "TaskRun with PipelineRun owner reference",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-2",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "tekton.dev/v1",
							Kind:       "PipelineRun",
							Name:       "pr-1",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "TaskRun with non-PipelineRun owner reference",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-3",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "pod-1",
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStandaloneTaskRun(tt.tr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTaskRun_Ignore(t *testing.T) {
	now := metav1.Now()
	earlier := metav1.NewTime(now.Add(-1 * time.Hour))

	tests := []struct {
		name   string
		tr     *pipelinev1.TaskRun
		expect bool
	}{
		{
			name: "Completed standalone TaskRun should not be ignored",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-1",
					Namespace: "default",
					Labels: map[string]string{
						"app": "test",
					},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime:      &earlier,
						CompletionTime: &now,
					},
				},
			},
			expect: false,
		},
		{
			name: "Running TaskRun should be ignored",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-2",
					Namespace: "default",
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime: &earlier,
					},
				},
			},
			expect: true,
		},
		{
			name: "TaskRun owned by PipelineRun should be ignored",
			tr: &pipelinev1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tr-3",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "tekton.dev/v1",
							Kind:       "PipelineRun",
							Name:       "pr-1",
						},
					},
				},
				Status: pipelinev1.TaskRunStatus{
					TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
						StartTime:      &earlier,
						CompletionTime: &now,
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

			client := fakepipelineclientset.NewSimpleClientset(tt.tr)
			trFuncs := NewTrFuncs(client)

			// Call Ignore (cast to metav1.Object)
			result := trFuncs.Ignore(metav1.Object(tt.tr))

			assert.Equal(t, tt.expect, result)
		})
	}
}
