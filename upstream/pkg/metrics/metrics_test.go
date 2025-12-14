/*
Copyright 2025 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// TestGetRecorder verifies singleton pattern for recorder.
func TestGetRecorder(t *testing.T) {
	recorder1 := GetRecorder()
	recorder2 := GetRecorder()

	assert.NotNil(t, recorder1)
	assert.Equal(t, recorder1, recorder2)
}

// TestNewRecorder ensures recorder initialization includes all required metrics.
func TestNewRecorder(t *testing.T) {
	r := newRecorder()

	assert.NotNil(t, r.resourcesProcessed)
	assert.NotNil(t, r.resourcesDeleted)
	assert.NotNil(t, r.resourcesErrors)
	assert.NotNil(t, r.reconciliationDuration)
	assert.NotNil(t, r.seenResources)
	assert.Empty(t, r.seenResources)
}

// TestTimer verifies timer creation and recording operations.
func TestTimer(t *testing.T) {
	r := newRecorder()
	ctx := context.Background()

	timer := r.NewTimer()
	assert.NotNil(t, timer)
	time.Sleep(10 * time.Millisecond)

	assert.NotPanics(t, func() {
		timer.RecordReconciliationDuration(ctx)
	})
	assert.NotPanics(t, func() {
		timer.RecordTTLProcessingDuration(ctx)
	})
	assert.NotPanics(t, func() {
		timer.RecordHistoryProcessingDuration(ctx)
	})
}

// TestRecordReconciliationEvent verifies event recording with various statuses.
func TestRecordReconciliationEvent(t *testing.T) {
	r := newRecorder()
	ctx := context.Background()

	tests := []struct {
		resourceType string
		namespace    string
		status       string
	}{
		{ResourceTypePipelineRun, "default", StatusSuccess},
		{ResourceTypeTaskRun, "test-ns", StatusFailed},
	}

	for _, tt := range tests {
		assert.NotPanics(t, func() {
			r.RecordReconciliationEvent(ctx, tt.resourceType, tt.namespace, tt.status)
		})
	}
}

// TestRecordResourceProcessed verifies duplicate resource detection via UID cache.
func TestRecordResourceProcessed(t *testing.T) {
	r := newRecorder()
	ctx := context.Background()

	uid1 := types.UID("test-uid-1")
	uid2 := types.UID("test-uid-2")

	r.RecordResourceProcessed(ctx, uid1, ResourceTypePipelineRun, "default", StatusSuccess)
	assert.True(t, r.seenResources[uid1])

	r.RecordResourceProcessed(ctx, uid1, ResourceTypePipelineRun, "default", StatusSuccess)
	assert.Len(t, r.seenResources, 1)

	r.RecordResourceProcessed(ctx, uid2, ResourceTypeTaskRun, "test-ns", StatusFailed)
	assert.Len(t, r.seenResources, 2)
}

// TestRecordResourceDeleted verifies deletion tracking with resource age.
func TestRecordResourceDeleted(t *testing.T) {
	r := newRecorder()
	ctx := context.Background()

	assert.NotPanics(t, func() {
		r.RecordResourceDeleted(ctx, ResourceTypePipelineRun, "default", OperationTTL, 1*time.Hour)
	})
}

// TestRecordResourceError verifies error recording with classification.
func TestRecordResourceError(t *testing.T) {
	r := newRecorder()
	ctx := context.Background()

	tests := []struct {
		errorType string
		reason    string
	}{
		{ErrorTypeAPI, "Failed to delete"},
		{ErrorTypeTimeout, "Context deadline exceeded"},
	}

	for _, tt := range tests {
		assert.NotPanics(t, func() {
			r.RecordResourceError(ctx, ResourceTypePipelineRun, "default", tt.errorType, tt.reason)
		})
	}
}

// TestUpdateActiveResourcesCount verifies gauge updates for resource tracking.
func TestUpdateActiveResourcesCount(t *testing.T) {
	r := newRecorder()
	ctx := context.Background()

	assert.NotPanics(t, func() {
		r.UpdateActiveResourcesCount(ctx, ResourceTypePipelineRun, "default", 5)
	})
	assert.NotPanics(t, func() {
		r.UpdateActiveResourcesCount(ctx, ResourceTypeTaskRun, "test-ns", -3)
	})
}

// TestResourceAttributes verifies attribute construction for metrics.
func TestResourceAttributes(t *testing.T) {
	attrs := ResourceAttributes(ResourceTypePipelineRun, "default")

	assert.Len(t, attrs, 2)
	keys := make(map[string]bool)
	for _, attr := range attrs {
		keys[string(attr.Key)] = true
	}
	assert.True(t, keys[LabelResourceType])
	assert.True(t, keys[LabelNamespace])
}

// TestErrorAttributes verifies error attribute construction.
func TestErrorAttributes(t *testing.T) {
	attrs := ErrorAttributes(ResourceTypePipelineRun, "default", ErrorTypeAPI, "Failed")

	assert.Len(t, attrs, 4)
	keys := make(map[string]bool)
	for _, attr := range attrs {
		keys[string(attr.Key)] = true
	}
	assert.True(t, keys[LabelResourceType])
	assert.True(t, keys[LabelNamespace])
	assert.True(t, keys[LabelErrorType])
	assert.True(t, keys[LabelReason])
}

// TestClassifyError verifies error type classification for Kubernetes errors.
func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"Nil error", nil, ""},
		{"Not found", errors.NewNotFound(schema.GroupResource{}, "test"), ErrorTypeNotFound},
		{"Forbidden", errors.NewForbidden(schema.GroupResource{}, "test", nil), ErrorTypePermission},
		{"Unauthorized", errors.NewUnauthorized("test"), ErrorTypePermission},
		{"Bad request", errors.NewBadRequest("test"), ErrorTypeValidation},
		{"Invalid", errors.NewInvalid(schema.GroupKind{}, "test", nil), ErrorTypeValidation},
		{"Timeout", errors.NewTimeoutError("test", 30), ErrorTypeTimeout},
		{"Internal", errors.NewInternalError(errors.NewBadRequest("test")), ErrorTypeAPI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ClassifyError(tt.err))
		})
	}
}

// TestIsAPIError verifies API error detection.
func TestIsAPIError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{errors.NewInternalError(errors.NewBadRequest("test")), true},
		{errors.NewServiceUnavailable("test"), true},
		{errors.NewNotFound(schema.GroupResource{}, "test"), false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, isAPIError(tt.err))
	}
}

// TestIsTimeoutError verifies timeout error detection.
func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{errors.NewTimeoutError("test", 30), true},
		{errors.NewServerTimeout(schema.GroupResource{}, "test", 1), true},
		{errors.NewNotFound(schema.GroupResource{}, "test"), false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, isTimeoutError(tt.err))
	}
}

// TestIsPermissionError verifies permission error detection.
func TestIsPermissionError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{errors.NewForbidden(schema.GroupResource{}, "test", nil), true},
		{errors.NewUnauthorized("test"), true},
		{errors.NewNotFound(schema.GroupResource{}, "test"), false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, isPermissionError(tt.err))
	}
}
