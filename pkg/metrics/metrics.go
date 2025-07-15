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
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	// Metric domain for the pruner - aligned with tektoncd naming convention
	Domain = "tekton.dev/pruner"

	// Component name
	Component = "tektoncd-pruner"

	// Meter name for OpenTelemetry
	MeterName = "github.com/openshift-pipelines/tektoncd-pruner"
)

// Reporter provides methods to report metrics for the pruner using OpenTelemetry
type Reporter struct {
	meter metric.Meter

	// Resource Processing Metrics
	resourcesProcessedTotal metric.Int64Counter
	resourcesDeletedTotal   metric.Int64Counter
	resourcesErrorsTotal    metric.Int64Counter
	resourcesSkippedTotal   metric.Int64Counter

	// Performance Metrics
	reconciliationDuration    metric.Float64Histogram
	ttlProcessingDuration     metric.Float64Histogram
	historyProcessingDuration metric.Float64Histogram
	resourceDeletionDuration  metric.Float64Histogram

	// State Metrics
	resourcesQueuedTotal   metric.Int64Counter
	currentResourcesQueued metric.Int64UpDownCounter
	activeResourcesCount   metric.Int64UpDownCounter

	// TTL-specific Metrics
	ttlAnnotationUpdatesTotal metric.Int64Counter
	ttlExpirationEventsTotal  metric.Int64Counter

	// History Limit Metrics
	historyLimitEventsTotal   metric.Int64Counter
	resourcesCleanedByHistory metric.Int64Counter

	// Configuration Metrics
	configurationReloadsTotal metric.Int64Counter
	configurationErrorsTotal  metric.Int64Counter

	// Resource Age Metrics
	resourceAgeAtDeletion metric.Float64Histogram

	// Error Breakdown Metrics
	resourceDeleteErrorsTotal metric.Int64Counter
	resourceUpdateErrorsTotal metric.Int64Counter

	// Operational Metrics
	garbageCollectionDuration metric.Float64Histogram
	namespacesProcessedTotal  metric.Int64Counter
	activeWorkersCount        metric.Int64UpDownCounter

	// Internal state for efficient UpDownCounter "set" semantics
	mu                            sync.Mutex
	lastQueuedByKey               map[string]int64
	lastActiveResourcesCountByKey map[string]int64
	lastActiveWorkersCount        int64
}

// NewReporter creates a new OpenTelemetry metrics reporter
func NewReporter(ctx context.Context) (*Reporter, error) {
	meter := otel.Meter(MeterName)

	r := &Reporter{meter: meter,
		lastQueuedByKey:               make(map[string]int64),
		lastActiveResourcesCountByKey: make(map[string]int64),
	}

	// Initialize all metrics
	if err := r.initializeMetrics(); err != nil {
		return nil, err
	}

	return r, nil
}

// initializeMetrics creates all OpenTelemetry metric instruments
func (r *Reporter) initializeMetrics() error {
	var err error

	// Resource Processing Metrics
	r.resourcesProcessedTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_resources_processed_total",
		metric.WithDescription("Total resources processed by the pruner"),
	)
	if err != nil {
		return err
	}

	r.resourcesDeletedTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_resources_deleted_total",
		metric.WithDescription("Total resources deleted by the pruner"),
	)
	if err != nil {
		return err
	}

	r.resourcesErrorsTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_resources_errors_total",
		metric.WithDescription("Total processing errors encountered"),
	)
	if err != nil {
		return err
	}

	r.resourcesSkippedTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_resources_skipped_total",
		metric.WithDescription("Total resources skipped during processing"),
	)
	if err != nil {
		return err
	}

	// Performance Metrics with proper histogram buckets
	latencyBuckets := metric.WithExplicitBucketBoundaries(
		0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 300.0, 600.0,
	)

	r.reconciliationDuration, err = r.meter.Float64Histogram(
		"tektoncd_pruner_reconciliation_duration_seconds",
		metric.WithDescription("Time spent in reconciliation operations"),
		metric.WithUnit("s"),
		latencyBuckets,
	)
	if err != nil {
		return err
	}

	r.ttlProcessingDuration, err = r.meter.Float64Histogram(
		"tektoncd_pruner_ttl_processing_duration_seconds",
		metric.WithDescription("Time spent processing TTL operations"),
		metric.WithUnit("s"),
		latencyBuckets,
	)
	if err != nil {
		return err
	}

	r.historyProcessingDuration, err = r.meter.Float64Histogram(
		"tektoncd_pruner_history_processing_duration_seconds",
		metric.WithDescription("Time spent processing history limit operations"),
		metric.WithUnit("s"),
		latencyBuckets,
	)
	if err != nil {
		return err
	}

	r.resourceDeletionDuration, err = r.meter.Float64Histogram(
		"tektoncd_pruner_resource_deletion_duration_seconds",
		metric.WithDescription("Time spent deleting individual resources"),
		metric.WithUnit("s"),
		latencyBuckets,
	)
	if err != nil {
		return err
	}

	// State Metrics
	r.resourcesQueuedTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_resources_queued_total",
		metric.WithDescription("Total resources queued for processing"),
	)
	if err != nil {
		return err
	}

	r.currentResourcesQueued, err = r.meter.Int64UpDownCounter(
		"tektoncd_pruner_current_resources_queued",
		metric.WithDescription("Current number of resources in processing queue"),
	)
	if err != nil {
		return err
	}

	r.activeResourcesCount, err = r.meter.Int64UpDownCounter(
		"tektoncd_pruner_active_resources_count",
		metric.WithDescription("Current number of active resources being processed"),
	)
	if err != nil {
		return err
	}

	// TTL-specific Metrics
	r.ttlAnnotationUpdatesTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_ttl_annotation_updates_total",
		metric.WithDescription("Total TTL annotation updates performed"),
	)
	if err != nil {
		return err
	}

	r.ttlExpirationEventsTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_ttl_expiration_events_total",
		metric.WithDescription("Total TTL expiration events processed"),
	)
	if err != nil {
		return err
	}

	// History Limit Metrics
	r.historyLimitEventsTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_history_limit_events_total",
		metric.WithDescription("Total history limit events processed"),
	)
	if err != nil {
		return err
	}

	r.resourcesCleanedByHistory, err = r.meter.Int64Counter(
		"tektoncd_pruner_resources_cleaned_by_history",
		metric.WithDescription("Total resources cleaned up by history limits"),
	)
	if err != nil {
		return err
	}

	// Configuration Metrics
	r.configurationReloadsTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_configuration_reloads_total",
		metric.WithDescription("Total configuration reload events"),
	)
	if err != nil {
		return err
	}

	r.configurationErrorsTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_configuration_errors_total",
		metric.WithDescription("Total configuration error events"),
	)
	if err != nil {
		return err
	}

	// Resource Age Metrics with age-appropriate buckets (1 minute to 30 days)
	ageBuckets := metric.WithExplicitBucketBoundaries(
		60, 300, 600, 1800, 3600, 7200, 21600, 43200, 86400, 172800, 432000, 864000, 1728000, 2592000,
	)

	r.resourceAgeAtDeletion, err = r.meter.Float64Histogram(
		"tektoncd_pruner_resource_age_at_deletion_seconds",
		metric.WithDescription("Age of resources at the time of deletion"),
		metric.WithUnit("s"),
		ageBuckets,
	)
	if err != nil {
		return err
	}

	// Error Breakdown Metrics
	r.resourceDeleteErrorsTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_resource_delete_errors_total",
		metric.WithDescription("Total resource deletion errors"),
	)
	if err != nil {
		return err
	}

	r.resourceUpdateErrorsTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_resource_update_errors_total",
		metric.WithDescription("Total resource update errors"),
	)
	if err != nil {
		return err
	}

	// Operational Metrics
	r.garbageCollectionDuration, err = r.meter.Float64Histogram(
		"tektoncd_pruner_garbage_collection_duration_seconds",
		metric.WithDescription("Time taken for complete garbage collection cycles"),
		metric.WithUnit("s"),
		latencyBuckets,
	)
	if err != nil {
		return err
	}

	r.namespacesProcessedTotal, err = r.meter.Int64Counter(
		"tektoncd_pruner_namespaces_processed_total",
		metric.WithDescription("Total namespaces processed during garbage collection"),
	)
	if err != nil {
		return err
	}

	r.activeWorkersCount, err = r.meter.Int64UpDownCounter(
		"tektoncd_pruner_active_workers_count",
		metric.WithDescription("Current number of active worker goroutines"),
	)
	if err != nil {
		return err
	}

	return nil
}

// ============================================================================
// Resource Processing Metrics Methods
// ============================================================================

func (r *Reporter) ReportResourceProcessed(namespace, resourceType, status string) {
	r.resourcesProcessedTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
			attribute.String("status", status),
		),
	)
}

func (r *Reporter) ReportResourceDeleted(namespace, resourceType, reason string) {
	r.resourcesDeletedTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
			attribute.String("reason", reason),
		),
	)
}

func (r *Reporter) ReportResourceError(namespace, resourceType, reason string) {
	r.resourcesErrorsTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
			attribute.String("reason", reason),
		),
	)
}

func (r *Reporter) ReportResourceSkipped(namespace, resourceType, reason string) {
	r.resourcesSkippedTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
			attribute.String("reason", reason),
		),
	)
}

// ============================================================================
// Performance Metrics Methods
// ============================================================================

func (r *Reporter) ReportReconciliationDuration(namespace, resourceType string, duration time.Duration) {
	r.reconciliationDuration.Record(context.Background(), duration.Seconds(),
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

func (r *Reporter) ReportTTLProcessingDuration(namespace, resourceType string, duration time.Duration) {
	r.ttlProcessingDuration.Record(context.Background(), duration.Seconds(),
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

func (r *Reporter) ReportHistoryProcessingDuration(namespace, resourceType string, duration time.Duration) {
	r.historyProcessingDuration.Record(context.Background(), duration.Seconds(),
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

func (r *Reporter) ReportResourceDeletionDuration(namespace, resourceType string, duration time.Duration) {
	r.resourceDeletionDuration.Record(context.Background(), duration.Seconds(),
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

// ============================================================================
// State Metrics Methods
// ============================================================================

func (r *Reporter) ReportResourceQueued(namespace, resourceType string) {
	r.resourcesQueuedTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

func (r *Reporter) ReportCurrentResourcesQueued(namespace, resourceType string, count int64) {
	key := namespace + "|" + resourceType
	r.mu.Lock()
	prev := r.lastQueuedByKey[key]
	r.lastQueuedByKey[key] = count
	delta := count - prev
	r.mu.Unlock()
	if delta != 0 {
		r.currentResourcesQueued.Add(context.Background(), delta,
			metric.WithAttributes(
				attribute.String("namespace", namespace),
				attribute.String("resource_type", resourceType),
			),
		)
	}
}

func (r *Reporter) ReportActiveResourcesCount(namespace, resourceType string, count int64) {
	key := namespace + "|" + resourceType
	r.mu.Lock()
	prev := r.lastActiveResourcesCountByKey[key]
	r.lastActiveResourcesCountByKey[key] = count
	delta := count - prev
	r.mu.Unlock()
	if delta != 0 {
		r.activeResourcesCount.Add(context.Background(), delta,
			metric.WithAttributes(
				attribute.String("namespace", namespace),
				attribute.String("resource_type", resourceType),
			),
		)
	}
}

// ============================================================================
// TTL-specific Metrics Methods
// ============================================================================

func (r *Reporter) ReportTTLAnnotationUpdate(namespace, resourceType string) {
	r.ttlAnnotationUpdatesTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

func (r *Reporter) ReportTTLExpirationEvent(namespace, resourceType string) {
	r.ttlExpirationEventsTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

// ============================================================================
// History Limit Metrics Methods
// ============================================================================

func (r *Reporter) ReportHistoryLimitEvent(namespace, resourceType string) {
	r.historyLimitEventsTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

func (r *Reporter) ReportResourceCleanedByHistory(namespace, resourceType string) {
	r.resourcesCleanedByHistory.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

// ============================================================================
// Configuration Metrics Methods
// ============================================================================

func (r *Reporter) ReportConfigurationReload(configLevel string) {
	r.configurationReloadsTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("config_level", configLevel),
		),
	)
}

func (r *Reporter) ReportConfigurationError(configLevel string) {
	r.configurationErrorsTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("config_level", configLevel),
		),
	)
}

// ============================================================================
// Resource Age Metrics Methods
// ============================================================================

func (r *Reporter) ReportResourceAgeAtDeletion(namespace, resourceType string, age time.Duration) {
	r.resourceAgeAtDeletion.Record(context.Background(), age.Seconds(),
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

// ============================================================================
// Error Breakdown Metrics Methods
// ============================================================================

func (r *Reporter) ReportResourceDeleteError(namespace, resourceType string) {
	r.resourceDeleteErrorsTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

func (r *Reporter) ReportResourceUpdateError(namespace, resourceType string) {
	r.resourceUpdateErrorsTotal.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("resource_type", resourceType),
		),
	)
}

// ============================================================================
// Additional Operational Metrics Methods
// ============================================================================

func (r *Reporter) ReportGarbageCollectionDuration(duration time.Duration, namespacesCount int) {
	r.garbageCollectionDuration.Record(context.Background(), duration.Seconds())
	r.namespacesProcessedTotal.Add(context.Background(), int64(namespacesCount))
}

func (r *Reporter) ReportActiveWorkers(count int) {
	r.mu.Lock()
	prev := r.lastActiveWorkersCount
	r.lastActiveWorkersCount = int64(count)
	delta := int64(count) - prev
	r.mu.Unlock()
	if delta != 0 {
		r.activeWorkersCount.Add(context.Background(), delta)
	}
}

// ============================================================================
// Backwards compatibility methods (deprecated, use specific methods above)
// ============================================================================

// ReportError - DEPRECATED: Use ReportResourceError instead
func (r *Reporter) ReportError(namespace, resourceType, operation, reason string) {
	r.ReportResourceError(namespace, resourceType, reason)
}

// ReportReconcileLatency - DEPRECATED: Use ReportReconciliationDuration instead
func (r *Reporter) ReportReconcileLatency(resourceType string, duration time.Duration) {
	r.ReportReconciliationDuration("", resourceType, duration)
}

// SetupPrometheusExporter creates and configures the Prometheus exporter
func SetupPrometheusExporter() (*prometheus.Exporter, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	return exporter, nil
}
