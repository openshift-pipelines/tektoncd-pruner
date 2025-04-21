package config

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
)

// mockResource implements metav1.Object for testing
type mockResource struct {
	metav1.ObjectMeta
	completed  bool
	successful bool
	failed     bool
}

// mockResourceFuncs implements HistoryLimiterResourceFuncs for testing
type mockResourceFuncs struct {
	resources       map[string][]metav1.Object
	successLimit    *int32
	failedLimit     *int32
	enforceLevel    EnforcedConfigLevel
	defaultLabelKey string
}

func (m *mockResourceFuncs) Type() string { return "MockResource" }

func (m *mockResourceFuncs) Get(_ context.Context, namespace, name string) (metav1.Object, error) {
	for _, res := range m.resources[namespace] {
		if res.GetName() == name {
			return res, nil
		}
	}
	return nil, nil
}

func (m *mockResourceFuncs) Update(_ context.Context, _ metav1.Object) error      { return nil }
func (m *mockResourceFuncs) Patch(_ context.Context, _, _ string, _ []byte) error { return nil }

func (m *mockResourceFuncs) Delete(_ context.Context, namespace, name string) error {
	resources := m.resources[namespace]
	for i, res := range resources {
		if res.GetName() == name {
			m.resources[namespace] = append(resources[:i], resources[i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockResourceFuncs) List(_ context.Context, namespace, _ string) ([]metav1.Object, error) {
	return m.resources[namespace], nil
}

func (m *mockResourceFuncs) GetSuccessHistoryLimitCount(_, _ string, _ SelectorSpec) (*int32, string) {
	return m.successLimit, "identified_by_global"
}

func (m *mockResourceFuncs) GetFailedHistoryLimitCount(_, _ string, _ SelectorSpec) (*int32, string) {
	return m.failedLimit, "identified_by_global"
}

func (m *mockResourceFuncs) IsSuccessful(resource metav1.Object) bool {
	if mr, ok := resource.(*mockResource); ok {
		return mr.successful
	}
	return false
}

func (m *mockResourceFuncs) IsFailed(resource metav1.Object) bool {
	if mr, ok := resource.(*mockResource); ok {
		return mr.failed
	}
	return false
}

func (m *mockResourceFuncs) IsCompleted(resource metav1.Object) bool {
	if mr, ok := resource.(*mockResource); ok {
		return mr.completed
	}
	return false
}

func (m *mockResourceFuncs) GetDefaultLabelKey() string { return m.defaultLabelKey }

func (m *mockResourceFuncs) GetEnforcedConfigLevel(_, _ string, _ SelectorSpec) EnforcedConfigLevel {
	return m.enforceLevel
}

func TestNewHistoryLimiter(t *testing.T) {
	tests := []struct {
		name       string
		resourceFn HistoryLimiterResourceFuncs
		wantErr    bool
	}{
		{
			name:       "nil resource funcs",
			resourceFn: nil,
			wantErr:    true,
		},
		{
			name:       "valid resource funcs",
			resourceFn: &mockResourceFuncs{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hl, err := NewHistoryLimiter(tt.resourceFn)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, hl)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, hl)
			}
		})
	}
}

func TestProcessEvent(t *testing.T) {
	tests := []struct {
		name          string
		resource      *mockResource
		successLimit  *int32
		failedLimit   *int32
		enforceLevel  EnforcedConfigLevel
		wantProcessed bool
	}{
		{
			name: "resource in deletion",
			resource: &mockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-1",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			},
			wantProcessed: false,
		},
		{
			name: "already processed resource",
			resource: &mockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-2",
					Annotations: map[string]string{
						AnnotationHistoryLimitCheckProcessed: time.Now().Format(time.RFC3339),
					},
				},
			},
			wantProcessed: true,
		},
		{
			name: "incomplete resource",
			resource: &mockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-3",
				},
				completed: false,
			},
			wantProcessed: false,
		},
		{
			name: "successful resource with limit",
			resource: &mockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-4",
				},
				completed:  true,
				successful: true,
			},
			successLimit:  ptr.Int32(2),
			enforceLevel:  EnforcedConfigLevelGlobal,
			wantProcessed: true,
		},
		{
			name: "failed resource with limit",
			resource: &mockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-5",
				},
				completed: true,
				failed:    true,
			},
			failedLimit:   ptr.Int32(1),
			enforceLevel:  EnforcedConfigLevelGlobal,
			wantProcessed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())

			mockFuncs := &mockResourceFuncs{
				resources: map[string][]metav1.Object{
					"default": {tt.resource},
				},
				successLimit:    tt.successLimit,
				failedLimit:     tt.failedLimit,
				enforceLevel:    tt.enforceLevel,
				defaultLabelKey: "test.label/name",
			}

			hl, err := NewHistoryLimiter(mockFuncs)
			assert.NoError(t, err)

			err = hl.ProcessEvent(ctx, tt.resource)
			assert.NoError(t, err)

			processed := hl.isProcessed(tt.resource)
			assert.Equal(t, tt.wantProcessed, processed)
		})
	}
}

func TestDoResourceCleanup(t *testing.T) {
	tests := []struct {
		name          string
		resources     []metav1.Object
		successLimit  *int32
		failedLimit   *int32
		annotations   map[string]string
		enforceLevel  EnforcedConfigLevel
		wantRemaining int
	}{
		{
			name: "cleanup successful resources - global enforcement",
			resources: []metav1.Object{
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-1",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
					},
					completed:  true,
					successful: true,
				},
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-2",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
					completed:  true,
					successful: true,
				},
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-3",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					},
					completed:  true,
					successful: true,
				},
			},
			successLimit:  ptr.Int32(2),
			enforceLevel:  EnforcedConfigLevelGlobal,
			wantRemaining: 2,
		},
		{
			name: "cleanup with annotation matching config - resource enforcement",
			resources: []metav1.Object{
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-1",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
						Annotations: map[string]string{
							AnnotationSuccessfulHistoryLimit: "2",
						},
					},
					completed:  true,
					successful: true,
				},
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-2",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
						Annotations: map[string]string{
							AnnotationSuccessfulHistoryLimit: "2",
						},
					},
					completed:  true,
					successful: true,
				},
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-3",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
						Annotations: map[string]string{
							AnnotationSuccessfulHistoryLimit: "2",
						},
					},
					completed:  true,
					successful: true,
				},
			},
			successLimit:  ptr.Int32(2),
			enforceLevel:  EnforcedConfigLevelResource,
			wantRemaining: 2,
		},
		{
			name: "cleanup with annotation not matching config - resource enforcement",
			resources: []metav1.Object{
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-1",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
						Annotations: map[string]string{
							AnnotationSuccessfulHistoryLimit: "3",
						},
					},
					completed:  true,
					successful: true,
				},
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-2",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
						Annotations: map[string]string{
							AnnotationSuccessfulHistoryLimit: "3",
						},
					},
					completed:  true,
					successful: true,
				},
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-3",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
						Annotations: map[string]string{
							AnnotationSuccessfulHistoryLimit: "3",
						},
					},
					completed:  true,
					successful: true,
				},
			},
			successLimit:  ptr.Int32(2),
			enforceLevel:  EnforcedConfigLevelResource,
			wantRemaining: 3, // Should not clean up since annotation doesn't match config
		},
		{
			name: "cleanup failed resources - namespace enforcement",
			resources: []metav1.Object{
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-1",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
					},
					completed: true,
					failed:    true,
				},
				&mockResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-2",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
					completed: true,
					failed:    true,
				},
			},
			failedLimit:   ptr.Int32(1),
			enforceLevel:  EnforcedConfigLevelNamespace,
			wantRemaining: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := logging.WithLogger(context.Background(), zaptest.NewLogger(t).Sugar())

			mockFuncs := &mockResourceFuncs{
				resources: map[string][]metav1.Object{
					"default": tt.resources,
				},
				successLimit:    tt.successLimit,
				failedLimit:     tt.failedLimit,
				enforceLevel:    tt.enforceLevel,
				defaultLabelKey: "test.label/name",
			}

			hl, err := NewHistoryLimiter(mockFuncs)
			assert.NoError(t, err)

			// Process each resource
			for _, res := range tt.resources {
				err = hl.ProcessEvent(ctx, res)
				assert.NoError(t, err)
			}

			// Check remaining resources
			remaining, err := mockFuncs.List(ctx, "default", "")
			assert.NoError(t, err)
			assert.Len(t, remaining, tt.wantRemaining)
		})
	}
}
