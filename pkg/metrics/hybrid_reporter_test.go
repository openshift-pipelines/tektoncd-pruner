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
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
)

// Mock implementations for testing
type mockMetricsReporter struct {
	reconciliationDurations []time.Duration
	resourceProcessed       []string
	resourceDeleted         []string
	resourceErrors          []string
}

func (m *mockMetricsReporter) ReportResourceProcessed(namespace, resourceType, status string) {
	m.resourceProcessed = append(m.resourceProcessed, status)
}

func (m *mockMetricsReporter) ReportResourceDeleted(namespace, resourceType, reason string) {
	m.resourceDeleted = append(m.resourceDeleted, reason)
}

func (m *mockMetricsReporter) ReportResourceError(namespace, resourceType, reason string) {
	m.resourceErrors = append(m.resourceErrors, reason)
}

func (m *mockMetricsReporter) ReportResourceSkipped(namespace, resourceType, reason string) {}

func (m *mockMetricsReporter) ReportReconciliationDuration(namespace, resourceType string, duration time.Duration) {
	m.reconciliationDurations = append(m.reconciliationDurations, duration)
}

func (m *mockMetricsReporter) ReportTTLProcessingDuration(namespace, resourceType string, duration time.Duration) {
}
func (m *mockMetricsReporter) ReportHistoryProcessingDuration(namespace, resourceType string, duration time.Duration) {
}
func (m *mockMetricsReporter) ReportResourceDeletionDuration(namespace, resourceType string, duration time.Duration) {
}
func (m *mockMetricsReporter) ReportResourceQueued(namespace, resourceType string) {}
func (m *mockMetricsReporter) ReportActiveResourcesCount(namespace, resourceType string, count int64) {
}
func (m *mockMetricsReporter) ReportCurrentResourcesQueued(namespace, resourceType string, count int64) {
}
func (m *mockMetricsReporter) ReportConfigurationReload(configLevel string) {}
func (m *mockMetricsReporter) ReportConfigurationError(configLevel string)  {}
func (m *mockMetricsReporter) ReportGarbageCollectionDuration(duration time.Duration, namespacesCount int) {
}
func (m *mockMetricsReporter) ReportResourceAgeAtDeletion(namespace, resourceType string, age time.Duration) {
}
func (m *mockMetricsReporter) ReportTTLAnnotationUpdate(namespace, resourceType string)      {}
func (m *mockMetricsReporter) ReportTTLExpirationEvent(namespace, resourceType string)       {}
func (m *mockMetricsReporter) ReportHistoryLimitEvent(namespace, resourceType string)        {}
func (m *mockMetricsReporter) ReportResourceCleanedByHistory(namespace, resourceType string) {}

type mockTraceReporter struct {
	enabled bool
	traces  []string
}

func (m *mockTraceReporter) StartSpan(ctx context.Context, operationName string) (context.Context, trace.Span) {
	return ctx, nil
}

func (m *mockTraceReporter) StartSpanWithAttributes(ctx context.Context, operationName string, attrs map[string]interface{}) (context.Context, trace.Span) {
	return ctx, nil
}

func (m *mockTraceReporter) EndSpan(span trace.Span) {}

func (m *mockTraceReporter) TraceReconcile(ctx context.Context, resourceType, namespace, name string) (context.Context, trace.Span) {
	m.traces = append(m.traces, "reconcile")
	return ctx, nil
}

func (m *mockTraceReporter) TraceResourceProcessing(ctx context.Context, operation, resourceType, namespace, name string) (context.Context, trace.Span) {
	m.traces = append(m.traces, operation)
	return ctx, nil
}

func (m *mockTraceReporter) TraceError(ctx context.Context, err error, message string) {
	m.traces = append(m.traces, "error")
}

func (m *mockTraceReporter) Enable()         { m.enabled = true }
func (m *mockTraceReporter) Disable()        { m.enabled = false }
func (m *mockTraceReporter) IsEnabled() bool { return m.enabled }

func TestNewHybridReporter(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	reporter, err := NewHybridReporter("test-controller", logger, config)
	if err != nil {
		t.Fatalf("Failed to create hybrid reporter: %v", err)
	}

	if reporter == nil {
		t.Fatal("Reporter should not be nil")
	}

	if reporter.reconcilerName != "test-controller" {
		t.Errorf("Expected reconciler name 'test-controller', got '%s'", reporter.reconcilerName)
	}
}

func TestHybridReporter_ReportReconcile(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	// Create reporter with mock metrics
	reporter := &HybridReporter{
		reconcilerName: "test-controller",
		logger:         logger,
		config:         config,
		initialized:    true,
	}

	mockMetrics := &mockMetricsReporter{}
	reporter.metricsReporter = mockMetrics

	// Test successful reconciliation
	key := types.NamespacedName{Namespace: "default", Name: "test-resource"}
	duration := 100 * time.Millisecond

	reporter.ReportReconcile(duration, true, key, "taskrun")

	// Verify metrics were reported
	if len(mockMetrics.reconciliationDurations) != 1 {
		t.Errorf("Expected 1 reconciliation duration, got %d", len(mockMetrics.reconciliationDurations))
	}

	if mockMetrics.reconciliationDurations[0] != duration {
		t.Errorf("Expected duration %v, got %v", duration, mockMetrics.reconciliationDurations[0])
	}

	if len(mockMetrics.resourceProcessed) != 1 {
		t.Errorf("Expected 1 resource processed event, got %d", len(mockMetrics.resourceProcessed))
	}

	if mockMetrics.resourceProcessed[0] != "success" {
		t.Errorf("Expected status 'success', got '%s'", mockMetrics.resourceProcessed[0])
	}
}

func TestHybridReporter_ErrorReporting(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	reporter := &HybridReporter{
		reconcilerName: "test-controller",
		logger:         logger,
		config:         config,
		initialized:    true,
	}

	mockMetrics := &mockMetricsReporter{}
	mockTrace := &mockTraceReporter{enabled: true}
	reporter.metricsReporter = mockMetrics
	reporter.errorReporter = NewErrorReporter(mockMetrics, mockTrace, logger)

	// Test error reporting
	testErr := errors.New("test error")
	ctx := context.Background()

	reporter.ReportReconcileError(ctx, testErr, "default", "taskrun", "processing")

	// Verify error was categorized and reported
	if len(mockMetrics.resourceErrors) != 1 {
		t.Errorf("Expected 1 resource error, got %d", len(mockMetrics.resourceErrors))
	}

	if len(mockTrace.traces) != 1 {
		t.Errorf("Expected 1 trace event, got %d", len(mockTrace.traces))
	}

	if mockTrace.traces[0] != "error" {
		t.Errorf("Expected trace event 'error', got '%s'", mockTrace.traces[0])
	}
}

func TestHybridReporter_Configuration(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	reporter, err := NewHybridReporter("test-controller", logger, config)
	if err != nil {
		t.Fatalf("Failed to create hybrid reporter: %v", err)
	}

	// Test configuration update
	newConfig := NewDefaultConfig()
	newConfig.MetricsEnabled = false

	err = reporter.UpdateConfig(newConfig)
	if err != nil {
		t.Errorf("Failed to update configuration: %v", err)
	}

	if reporter.config.IsMetricsEnabled() {
		t.Error("Expected metrics to be disabled after config update")
	}
}

func TestHybridReporter_HealthStatus(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	reporter, err := NewHybridReporter("test-controller", logger, config)
	if err != nil {
		t.Fatalf("Failed to create hybrid reporter: %v", err)
	}

	status := reporter.GetHealthStatus()

	// Verify health status fields
	if status["reconciler_name"] != "test-controller" {
		t.Errorf("Expected reconciler name 'test-controller', got '%v'", status["reconciler_name"])
	}

	if status["initialized"] != true {
		t.Error("Expected initialized to be true")
	}

	if status["metrics_enabled"] != true {
		t.Error("Expected metrics to be enabled")
	}

	if status["tracing_enabled"] != true {
		t.Error("Expected tracing to be enabled")
	}
}

func TestHybridReporter_WithErrorReporting(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	reporter := &HybridReporter{
		reconcilerName: "test-controller",
		logger:         logger,
		config:         config,
		initialized:    true,
	}

	mockMetrics := &mockMetricsReporter{}
	mockTrace := &mockTraceReporter{enabled: true}
	reporter.metricsReporter = mockMetrics
	reporter.errorReporter = NewErrorReporter(mockMetrics, mockTrace, logger)

	ctx := context.Background()

	// Test successful operation
	err := reporter.WithErrorReporting(ctx, "test-operation", "default", "taskrun", func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test operation with error
	testErr := errors.New("operation failed")
	err = reporter.WithErrorReporting(ctx, "test-operation", "default", "taskrun", func() error {
		return testErr
	})

	if err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}

	// Verify error was reported
	if len(mockMetrics.resourceErrors) != 1 {
		t.Errorf("Expected 1 resource error, got %d", len(mockMetrics.resourceErrors))
	}
}

func TestHybridReporter_MetricsSummary(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	reporter, err := NewHybridReporter("test-controller", logger, config)
	if err != nil {
		t.Fatalf("Failed to create hybrid reporter: %v", err)
	}

	summary := reporter.GetMetricsSummary()

	// Verify summary structure
	if summary["reconciler_name"] != "test-controller" {
		t.Errorf("Expected reconciler name 'test-controller', got '%v'", summary["reconciler_name"])
	}

	configSection, exists := summary["configuration"].(map[string]interface{})
	if !exists {
		t.Fatal("Expected configuration section to exist")
	}

	if configSection["metrics_enabled"] != true {
		t.Error("Expected metrics to be enabled in summary")
	}

	if configSection["tracing_enabled"] != true {
		t.Error("Expected tracing to be enabled in summary")
	}

	// Verify metrics lists exist
	if _, exists := summary["knative_controller_metrics"]; !exists {
		t.Error("Expected knative_controller_metrics to exist in summary")
	}

	if _, exists := summary["comprehensive_pruner_metrics"]; !exists {
		t.Error("Expected comprehensive_pruner_metrics to exist in summary")
	}
}

func TestObservabilityConfig_Validation(t *testing.T) {
	config := NewDefaultConfig()

	// Test valid configuration
	err := config.Validate()
	if err != nil {
		t.Errorf("Expected valid config to pass validation, got error: %v", err)
	}

	// Test invalid metrics port
	config.MetricsPort = -1
	err = config.Validate()
	if err == nil {
		t.Error("Expected invalid metrics port to fail validation")
	}

	// Test invalid tracing sample rate
	config.MetricsPort = 9090 // Reset to valid
	config.TracingSampleRate = 1.5
	err = config.Validate()
	if err == nil {
		t.Error("Expected invalid sample rate to fail validation")
	}
}

func TestErrorReporter_Categorization(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockMetrics := &mockMetricsReporter{}
	mockTrace := &mockTraceReporter{enabled: true}

	errorReporter := NewErrorReporter(mockMetrics, mockTrace, logger)

	testCases := []struct {
		error    string
		expected string
	}{
		{"resource not found", "not_found"},
		{"403 forbidden", "permission_denied"},
		{"request timeout", "timeout"},
		{"connection refused", "network_error"},
		{"conflict detected", "conflict"},
		{"rate limit exceeded", "rate_limited"},
		{"internal server error", "server_error"},
		{"bad request format", "validation_error"},
		{"quota exceeded", "quota_exceeded"},
		{"unknown error type", "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.error, func(t *testing.T) {
			category := errorReporter.categorizeError(errors.New(tc.error), "test")
			if category != tc.expected {
				t.Errorf("Expected category '%s' for error '%s', got '%s'", tc.expected, tc.error, category)
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkHybridReporter_ReportReconcile(b *testing.B) {
	logger := zap.NewNop().Sugar()
	config := NewDefaultConfig()

	reporter := &HybridReporter{
		reconcilerName:  "benchmark-controller",
		logger:          logger,
		config:          config,
		metricsReporter: &mockMetricsReporter{},
		initialized:     true,
	}

	key := types.NamespacedName{Namespace: "default", Name: "test-resource"}
	duration := 100 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reporter.ReportReconcile(duration, true, key, "taskrun")
	}
}

func BenchmarkErrorReporter_ReportError(b *testing.B) {
	logger := zap.NewNop().Sugar()
	mockMetrics := &mockMetricsReporter{}
	mockTrace := &mockTraceReporter{enabled: true}

	errorReporter := NewErrorReporter(mockMetrics, mockTrace, logger)
	ctx := context.Background()
	testErr := errors.New("benchmark error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errorReporter.ReportError(ctx, testErr, "benchmark", "default", "taskrun")
	}
}
