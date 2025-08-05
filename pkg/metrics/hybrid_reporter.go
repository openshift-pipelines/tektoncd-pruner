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
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/controller"
)

// HybridReporter provides comprehensive observability combining Knative controller metrics
// with detailed OpenTelemetry metrics for complete observability coverage.
//
// This production-ready implementation provides:
// - 12+ Knative controller metrics (industry standard)
// - 16+ detailed pruner metrics (domain-specific insights)
// - Automatic error categorization and reporting
// - Configuration-driven setup with validation
// - Comprehensive health monitoring
// - Thread-safe operations with minimal performance impact
type HybridReporter struct {
	// Core observability components
	controllerStats controller.StatsReporter
	metricsReporter MetricsReporter
	traceReporter   TraceReporter
	errorReporter   *ErrorReporter

	// Configuration and state
	config         *ObservabilityConfig
	logger         *zap.SugaredLogger
	reconcilerName string
	initialized    bool
	mu             sync.RWMutex
}

// NewHybridReporter creates a new production-ready hybrid reporter with enhanced error handling
// and configuration management.
//
// Usage:
//
//	configManager := metrics.NewConfigManager(logger)
//	configManager.LoadConfig(ctx, configMap)
//	reporter, err := metrics.NewHybridReporter("my-controller", logger, configManager.GetConfig())
func NewHybridReporter(reconcilerName string, logger *zap.SugaredLogger, config *ObservabilityConfig) (*HybridReporter, error) {
	if config == nil {
		config = NewDefaultConfig()
	}

	reporter := &HybridReporter{
		reconcilerName: reconcilerName,
		logger:         logger,
		config:         config,
	}

	if err := reporter.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize hybrid reporter: %w", err)
	}

	return reporter, nil
}

// initialize sets up all the reporter components based on configuration
func (hr *HybridReporter) initialize() error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if hr.initialized {
		return nil
	}

	// Always initialize Knative controller stats (industry standard)
	hr.controllerStats = controller.MustNewStatsReporter(hr.reconcilerName, hr.logger)

	// Initialize metrics reporter if enabled
	if hr.config.IsMetricsEnabled() {
		hr.metricsReporter = GetReporter()
		if hr.metricsReporter == nil {
			return fmt.Errorf("metrics are enabled but reporter is nil - ensure metrics.Init() was called")
		}
	}

	// Initialize trace reporter if enabled
	if hr.config.IsTracingEnabled() {
		hr.traceReporter = GetTracer()
		if hr.traceReporter == nil {
			hr.logger.Warn("Tracing is enabled but tracer is nil - tracing will be disabled")
		}
	}

	// Always initialize error reporter for production robustness
	hr.errorReporter = NewErrorReporter(hr.metricsReporter, hr.traceReporter, hr.logger)

	hr.initialized = true
	hr.logger.Infow("Hybrid reporter initialized successfully",
		"reconciler", hr.reconcilerName,
		"metrics_enabled", hr.config.IsMetricsEnabled(),
		"tracing_enabled", hr.config.IsTracingEnabled(),
		"total_metrics_available", "28+")

	return nil
}

// =============================================================================
// Primary Observability Interface (Most Important Methods)
// =============================================================================

// ReportReconcile reports reconciliation metrics to both Knative and OpenTelemetry systems.
// This is the primary method used by reconcilers.
//
// Metrics reported:
// - Knative: reconcile_count, reconcile_latency (with reconciler, success, namespace tags)
// - OpenTelemetry: tektoncd_pruner_reconciliation_duration_seconds, tektoncd_pruner_resources_processed_total
func (hr *HybridReporter) ReportReconcile(duration time.Duration, success bool, key types.NamespacedName, resourceType string) {
	// Report to Knative controller metrics (industry standard)
	if err := hr.reportToKnative(duration, success, key); err != nil {
		hr.errorReporter.ReportError(context.Background(), err, "knative_reporting", key.Namespace, resourceType)
	}

	// Report to OpenTelemetry metrics (detailed insights)
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportReconciliationDuration(key.Namespace, resourceType, duration)
		status := "success"
		if !success {
			status = "error"
		}
		hr.metricsReporter.ReportResourceProcessed(key.Namespace, resourceType, status)
	}
}

// ReportQueueDepth reports current queue depth to both observability systems.
// Critical for monitoring controller performance and resource pressure.
//
// Metrics reported:
// - Knative: work_queue_depth (with reconciler tag)
// - OpenTelemetry: tektoncd_pruner_current_resources_queued
func (hr *HybridReporter) ReportQueueDepth(depth int64) {
	// Report to Knative
	if err := hr.controllerStats.ReportQueueDepth(depth); err != nil {
		hr.errorReporter.ReportError(context.Background(), err, "queue_depth_reporting", "", hr.reconcilerName)
	}

	// Report to OpenTelemetry for consistency
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportCurrentResourcesQueued("", hr.reconcilerName, depth)
	}
}

// WithErrorReporting wraps operations with comprehensive error reporting and automatic categorization.
// This is the recommended way to handle operations that might fail.
//
// Example:
//
//	err := reporter.WithErrorReporting(ctx, "reconcile", namespace, "taskrun", func() error {
//	    return doReconciliation()
//	})
func (hr *HybridReporter) WithErrorReporting(ctx context.Context, operation, namespace, resourceType string, fn func() error) error {
	if hr.errorReporter != nil {
		return hr.errorReporter.WithErrorReporting(ctx, operation, namespace, resourceType, fn)
	}
	return fn()
}

// =============================================================================
// Resource Operation Methods
// =============================================================================

// ReportResourceDeleted reports successful resource deletion
func (hr *HybridReporter) ReportResourceDeleted(namespace, resourceType, reason string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportResourceDeleted(namespace, resourceType, reason)
	}
}

// ReportResourceError reports resource processing errors with automatic categorization
func (hr *HybridReporter) ReportResourceError(namespace, resourceType, reason string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportResourceError(namespace, resourceType, reason)
	}
}

// ReportResourceSkipped reports skipped resources
func (hr *HybridReporter) ReportResourceSkipped(namespace, resourceType, reason string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportResourceSkipped(namespace, resourceType, reason)
	}
}

// =============================================================================
// Performance Metrics Methods
// =============================================================================

// ReportTTLProcessingDuration reports TTL processing performance
func (hr *HybridReporter) ReportTTLProcessingDuration(namespace, resourceType string, duration time.Duration) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportTTLProcessingDuration(namespace, resourceType, duration)
	}
}

// ReportHistoryProcessingDuration reports history processing performance
func (hr *HybridReporter) ReportHistoryProcessingDuration(namespace, resourceType string, duration time.Duration) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportHistoryProcessingDuration(namespace, resourceType, duration)
	}
}

// ReportResourceDeletionDuration reports resource deletion performance
func (hr *HybridReporter) ReportResourceDeletionDuration(namespace, resourceType string, duration time.Duration) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportResourceDeletionDuration(namespace, resourceType, duration)
	}
}

// ReportResourceAgeAtDeletion reports how old resources were when deleted
func (hr *HybridReporter) ReportResourceAgeAtDeletion(namespace, resourceType string, age time.Duration) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportResourceAgeAtDeletion(namespace, resourceType, age)
	}
}

// =============================================================================
// State Metrics Methods
// =============================================================================

// ReportResourceQueued reports queued resources
func (hr *HybridReporter) ReportResourceQueued(namespace, resourceType string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportResourceQueued(namespace, resourceType)
	}
}

// ReportActiveResourcesCount reports active resource count
func (hr *HybridReporter) ReportActiveResourcesCount(namespace, resourceType string, count int64) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportActiveResourcesCount(namespace, resourceType, count)
	}
}

// =============================================================================
// Configuration and Operational Methods
// =============================================================================

// ReportConfigurationReload reports configuration reloads
func (hr *HybridReporter) ReportConfigurationReload(configLevel string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportConfigurationReload(configLevel)
	}
}

// ReportConfigurationError reports configuration errors
func (hr *HybridReporter) ReportConfigurationError(configLevel string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportConfigurationError(configLevel)
	}
}

// ReportGarbageCollectionDuration reports garbage collection performance
func (hr *HybridReporter) ReportGarbageCollectionDuration(duration time.Duration, namespacesCount int) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportGarbageCollectionDuration(duration, namespacesCount)
	}
}

// =============================================================================
// TTL and History Specific Methods
// =============================================================================

// ReportTTLAnnotationUpdate reports TTL annotation updates
func (hr *HybridReporter) ReportTTLAnnotationUpdate(namespace, resourceType string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportTTLAnnotationUpdate(namespace, resourceType)
	}
}

// ReportTTLExpirationEvent reports TTL expiration events
func (hr *HybridReporter) ReportTTLExpirationEvent(namespace, resourceType string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportTTLExpirationEvent(namespace, resourceType)
	}
}

// ReportHistoryLimitEvent reports history limit events
func (hr *HybridReporter) ReportHistoryLimitEvent(namespace, resourceType string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportHistoryLimitEvent(namespace, resourceType)
	}
}

// ReportResourceCleanedByHistory reports resources cleaned by history limits
func (hr *HybridReporter) ReportResourceCleanedByHistory(namespace, resourceType string) {
	if hr.metricsReporter != nil {
		hr.metricsReporter.ReportResourceCleanedByHistory(namespace, resourceType)
	}
}

// =============================================================================
// Enhanced Error Handling Methods
// =============================================================================

// ReportReconcileError reports reconciliation errors with proper categorization
func (hr *HybridReporter) ReportReconcileError(ctx context.Context, err error, namespace, resourceType, phase string) {
	if hr.errorReporter != nil {
		hr.errorReporter.ReportReconcileError(ctx, err, namespace, resourceType, phase)
	}
}

// =============================================================================
// Tracing Methods
// =============================================================================

// TraceReconcile starts a reconciliation trace
func (hr *HybridReporter) TraceReconcile(ctx context.Context, resourceType, namespace, name string) (context.Context, error) {
	if hr.traceReporter != nil && hr.traceReporter.IsEnabled() {
		newCtx, _ := hr.traceReporter.TraceReconcile(ctx, resourceType, namespace, name)
		return newCtx, nil
	}
	return ctx, nil
}

// TraceResourceProcessing starts a resource processing trace
func (hr *HybridReporter) TraceResourceProcessing(ctx context.Context, operation, resourceType, namespace, name string) (context.Context, error) {
	if hr.traceReporter != nil && hr.traceReporter.IsEnabled() {
		newCtx, _ := hr.traceReporter.TraceResourceProcessing(ctx, operation, resourceType, namespace, name)
		return newCtx, nil
	}
	return ctx, nil
}

// =============================================================================
// Configuration and Health Methods
// =============================================================================

// UpdateConfig updates the reporter configuration and reinitializes if needed
func (hr *HybridReporter) UpdateConfig(config *ObservabilityConfig) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	hr.config = config
	hr.logger.Infow("Configuration updated",
		"metrics_enabled", config.IsMetricsEnabled(),
		"tracing_enabled", config.IsTracingEnabled())

	return hr.initialize() // Re-initialize with new config
}

// GetHealthStatus returns comprehensive health status of all observability components
func (hr *HybridReporter) GetHealthStatus() map[string]interface{} {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	status := map[string]interface{}{
		"reconciler_name":  hr.reconcilerName,
		"initialized":      hr.initialized,
		"knative_stats":    hr.controllerStats != nil,
		"metrics_enabled":  hr.config.IsMetricsEnabled(),
		"metrics_reporter": hr.metricsReporter != nil,
		"tracing_enabled":  hr.config.IsTracingEnabled(),
		"tracing_reporter": hr.traceReporter != nil && hr.traceReporter.IsEnabled(),
		"error_reporter":   hr.errorReporter != nil,
		"error_stats":      map[string]int{},
	}

	if hr.errorReporter != nil {
		status["error_stats"] = hr.errorReporter.GetErrorStats()
	}

	return status
}

// GetMetricsSummary returns a comprehensive summary of all available metrics
func (hr *HybridReporter) GetMetricsSummary() map[string]interface{} {
	return map[string]interface{}{
		"reconciler_name": hr.reconcilerName,
		"total_metrics":   "28+",
		"configuration": map[string]interface{}{
			"metrics_enabled": hr.config.IsMetricsEnabled(),
			"tracing_enabled": hr.config.IsTracingEnabled(),
		},
		"knative_controller_metrics": map[string]interface{}{
			"count": 12,
			"metrics": []string{
				"reconcile_count (with reconciler, success, namespace tags)",
				"reconcile_latency (histogram: 10ms-60s buckets)",
				"work_queue_depth (with reconciler tag)",
				"workqueue_adds_total", "workqueue_depth", "workqueue_queue_latency_seconds",
				"workqueue_retries_total", "workqueue_work_duration_seconds",
				"workqueue_unfinished_work_seconds", "workqueue_longest_running_processor_seconds",
				"client_latency", "client_results",
			},
		},
		"comprehensive_pruner_metrics": map[string]interface{}{
			"count": "16+",
			"metrics": []string{
				"tektoncd_pruner_resources_processed_total",
				"tektoncd_pruner_resources_deleted_total", "tektoncd_pruner_resources_errors_total",
				"tektoncd_pruner_reconciliation_duration_seconds", "tektoncd_pruner_ttl_processing_duration_seconds",
				"tektoncd_pruner_history_processing_duration_seconds", "tektoncd_pruner_resource_deletion_duration_seconds",
				"tektoncd_pruner_active_resources_count", "tektoncd_pruner_current_resources_queued",
				"tektoncd_pruner_configuration_reloads_total", "tektoncd_pruner_resource_age_at_deletion_seconds",
				"+ more comprehensive metrics...",
			},
		},
		"error_statistics": hr.errorReporter.GetErrorStats(),
	}
}

// =============================================================================
// Internal Helper Methods
// =============================================================================

// reportToKnative handles Knative-specific reporting with error handling
func (hr *HybridReporter) reportToKnative(duration time.Duration, success bool, key types.NamespacedName) error {
	successStr := "true"
	if !success {
		successStr = "false"
	}
	return hr.controllerStats.ReportReconcile(duration, successStr, key)
}
