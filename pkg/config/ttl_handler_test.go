package config

import (
	"context"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clocktest "k8s.io/utils/clock/testing"
	"knative.dev/pkg/ptr"
)

// ttlMockResource implements metav1.Object for testing
type ttlMockResource struct {
	metav1.ObjectMeta
	completed       bool
	completion_time *metav1.Time
}

// mockTTLFuncs implements TTLResourceFuncs for testing
type mockTTLFuncs struct {
	resources           map[string]*ttlMockResource
	enforcedConfigLevel EnforcedConfigLevel
	ttl                 *int32
}

func newMockTTLFuncs() *mockTTLFuncs {
	return &mockTTLFuncs{
		resources: make(map[string]*ttlMockResource),
	}
}

func (m *mockTTLFuncs) Type() string { return "MockResource" }

func (m *mockTTLFuncs) Get(_ context.Context, namespace, name string) (metav1.Object, error) {
	key := namespace + "/" + name
	if res, ok := m.resources[key]; ok {
		return res, nil
	}
	return nil, errors.NewNotFound(schema.GroupResource{Group: "test", Resource: "mock"}, name)
}

func (m *mockTTLFuncs) Delete(_ context.Context, namespace, name string) error {
	key := namespace + "/" + name
	if _, ok := m.resources[key]; ok {
		delete(m.resources, key)
		return nil
	}
	return errors.NewNotFound(schema.GroupResource{Group: "test", Resource: "mock"}, name)
}

func (m *mockTTLFuncs) Patch(_ context.Context, namespace, name string, _ []byte) error {
	key := namespace + "/" + name
	if res, ok := m.resources[key]; ok {
		if res.Annotations == nil {
			res.Annotations = make(map[string]string)
		}
		res.Annotations[AnnotationTTLSecondsAfterFinished] = "60" // Default test TTL
		return nil
	}
	return errors.NewNotFound(schema.GroupResource{Group: "test", Resource: "mock"}, name)
}

func (m *mockTTLFuncs) Update(_ context.Context, resource metav1.Object) error {
	key := resource.GetNamespace() + "/" + resource.GetName()
	if _, ok := m.resources[key]; ok {
		m.resources[key] = resource.(*ttlMockResource)
		return nil
	}
	return errors.NewNotFound(schema.GroupResource{Group: "test", Resource: "mock"}, resource.GetName())
}

func (m *mockTTLFuncs) IsCompleted(resource metav1.Object) bool {
	if mr, ok := resource.(*ttlMockResource); ok {
		return mr.completed
	}
	return false
}

func (m *mockTTLFuncs) GetCompletionTime(resource metav1.Object) (metav1.Time, error) {
	if mr, ok := resource.(*ttlMockResource); ok && mr.completion_time != nil {
		return *mr.completion_time, nil
	}
	return metav1.Time{}, fmt.Errorf("completion time not set")
}

func (m *mockTTLFuncs) Ignore(resource metav1.Object) bool { return false }

func (m *mockTTLFuncs) GetTTLSecondsAfterFinished(_, _ string, _ SelectorSpec) (*int32, string) {
	ttl := int32(60) // Default test TTL
	return &ttl, "test"
}

func (m *mockTTLFuncs) GetDefaultLabelKey() string { return "test.mock/resource" }

func (m *mockTTLFuncs) GetEnforcedConfigLevel(_, _ string, _ SelectorSpec) EnforcedConfigLevel {
	return m.enforcedConfigLevel
}

func TestNewTTLHandlerFunc(t *testing.T) {
	mockFuncs := newMockTTLFuncs()
	fakeClock := clocktest.NewFakeClock(time.Now())

	tests := []struct {
		name       string
		resourceFn TTLResourceFuncs
		wantErr    bool
		errMessage string
	}{
		{
			name:       "Valid constructor",
			resourceFn: mockFuncs,
			wantErr:    false,
		},
		{
			name:       "Nil resource functions",
			resourceFn: nil,
			wantErr:    true,
			errMessage: "resourceFunc interface can not be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewTTLHandler(fakeClock, tt.resourceFn)
			if tt.wantErr {
				if err == nil {
					t.Error("NewTTLHandler() error = nil, want error")
				} else if err.Error() != tt.errMessage {
					t.Errorf("NewTTLHandler() error = %v, want %v", err, tt.errMessage)
				}
				return
			}
			if err != nil {
				t.Errorf("NewTTLHandler() unexpected error = %v", err)
			}
			if handler == nil {
				t.Error("NewTTLHandler() handler is nil, want non-nil")
			}
		})
	}
}

func TestHandleProcessEvent(t *testing.T) {
	mockFuncs := newMockTTLFuncs()
	fakeClock := clocktest.NewFakeClock(time.Now())
	handler, _ := NewTTLHandler(fakeClock, mockFuncs)

	tests := []struct {
		name        string
		resource    *ttlMockResource
		wantErr     bool
		wantDeleted bool
	}{
		{
			name: "Resource not completed",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test1",
					Namespace: "default",
				},
				completed: false,
			},
			wantErr:     false,
			wantDeleted: false,
		},
		{
			name: "Resource completed but TTL not expired",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test2",
					Namespace: "default",
				},
				completed:       true,
				completion_time: &metav1.Time{Time: fakeClock.Now()},
			},
			wantErr:     false,
			wantDeleted: false,
		},
		{
			name: "Resource completed and TTL expired",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test3",
					Namespace: "default",
				},
				completed:       true,
				completion_time: &metav1.Time{Time: fakeClock.Now().Add(-2 * time.Hour)},
			},
			wantErr:     false,
			wantDeleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add resource to mock storage
			key := tt.resource.GetNamespace() + "/" + tt.resource.GetName()
			mockFuncs.resources[key] = tt.resource

			err := handler.ProcessEvent(context.Background(), tt.resource)
			if tt.wantErr && err == nil {
				t.Error("ProcessEvent() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ProcessEvent() unexpected error = %v", err)
			}

			// Check if resource was deleted as expected
			_, exists := mockFuncs.resources[key]
			if tt.wantDeleted && exists {
				t.Error("Resource should have been deleted but still exists")
			}
			if !tt.wantDeleted && !exists {
				t.Error("Resource should exist but was deleted")
			}
		})
	}
}

func TestResourceNeedsCleanup(t *testing.T) {
	mockFuncs := newMockTTLFuncs()
	fakeClock := clocktest.NewFakeClock(time.Now())
	handler, _ := NewTTLHandler(fakeClock, mockFuncs)

	tests := []struct {
		name         string
		resource     *ttlMockResource
		enforceLevel EnforcedConfigLevel
		configTTL    *int32
		want         bool
	}{
		{
			name: "No annotations",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test1",
					Namespace: "default",
				},
				completed: true,
			},
			enforceLevel: EnforcedConfigLevelGlobal,
			want:         false,
		},
		{
			name: "Has TTL annotation but not completed",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test2",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationTTLSecondsAfterFinished: "60",
					},
				},
				completed: false,
			},
			enforceLevel: EnforcedConfigLevelGlobal,
			want:         false,
		},
		{
			name: "Has TTL annotation and completed - global enforcement",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test3",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationTTLSecondsAfterFinished: "60",
					},
				},
				completed: true,
			},
			enforceLevel: EnforcedConfigLevelGlobal,
			configTTL:    ptr.Int32(60),
			want:         true,
		},
		{
			name: "Has TTL annotation but doesn't match config - global enforcement",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test4",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationTTLSecondsAfterFinished: "120",
					},
				},
				completed: true,
			},
			enforceLevel: EnforcedConfigLevelGlobal,
			configTTL:    ptr.Int32(60),
			want:         false,
		},
		{
			name: "Has TTL=-1 annotation - resource enforcement",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test5",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationTTLSecondsAfterFinished: "-1",
					},
				},
				completed: true,
			},
			enforceLevel: EnforcedConfigLevelResource,
			want:         false,
		},
		{
			name: "Has TTL annotation matching config - resource enforcement",
			resource: &ttlMockResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test6",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationTTLSecondsAfterFinished: "60",
					},
				},
				completed: true,
			},
			enforceLevel: EnforcedConfigLevelResource,
			configTTL:    ptr.Int32(60),
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.configTTL != nil {
				mockFuncs.enforcedConfigLevel = tt.enforceLevel
				mockFuncs.ttl = tt.configTTL
			}
			if got := handler.needsCleanup(tt.resource); got != tt.want {
				t.Errorf("needsCleanup() = %v, want %v", got, tt.want)
			}
		})
	}
}
