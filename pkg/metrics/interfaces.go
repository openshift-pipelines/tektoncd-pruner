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
	"time"

	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/types"
)

// MetricsReporter defines the interface for reporting metrics
type MetricsReporter interface {
	// Core resource operations
	ReportResourceProcessed(namespace, resourceType, status string)
	ReportResourceDeleted(namespace, resourceType, reason string)
	ReportResourceError(namespace, resourceType, reason string)
	ReportResourceSkipped(namespace, resourceType, reason string)

	// Performance metrics
	ReportReconciliationDuration(namespace, resourceType string, duration time.Duration)
	ReportTTLProcessingDuration(namespace, resourceType string, duration time.Duration)
	ReportHistoryProcessingDuration(namespace, resourceType string, duration time.Duration)
	ReportResourceDeletionDuration(namespace, resourceType string, duration time.Duration)

	// State metrics
	ReportResourceQueued(namespace, resourceType string)
	ReportActiveResourcesCount(namespace, resourceType string, count int64)
	ReportCurrentResourcesQueued(namespace, resourceType string, count int64)

	// TTL-specific metrics
	ReportTTLAnnotationUpdate(namespace, resourceType string)
	ReportTTLExpirationEvent(namespace, resourceType string)

	// History limit metrics
	ReportHistoryLimitEvent(namespace, resourceType string)
	ReportResourceCleanedByHistory(namespace, resourceType string)

	// Configuration metrics
	ReportConfigurationReload(configLevel string)
	ReportConfigurationError(configLevel string)

	// Operational metrics
	ReportGarbageCollectionDuration(duration time.Duration, namespacesCount int)
	ReportResourceAgeAtDeletion(namespace, resourceType string, age time.Duration)
}

// ControllerReporter defines the interface for controller-level metrics
type ControllerReporter interface {
	ReportReconcile(duration time.Duration, success bool, key types.NamespacedName, resourceType string)
	ReportQueueDepth(depth int64)
}

// TraceReporter defines the interface for distributed tracing
type TraceReporter interface {
	StartSpan(ctx context.Context, operationName string) (context.Context, trace.Span)
	StartSpanWithAttributes(ctx context.Context, operationName string, attrs map[string]interface{}) (context.Context, trace.Span)
	EndSpan(span trace.Span)

	// High-level tracing operations
	TraceReconcile(ctx context.Context, resourceType, namespace, name string) (context.Context, trace.Span)
	TraceResourceProcessing(ctx context.Context, operation, resourceType, namespace, name string) (context.Context, trace.Span)
	TraceError(ctx context.Context, err error, message string)

	// Control tracing
	Enable()
	Disable()
	IsEnabled() bool
}

// ObservabilityReporter combines all observability capabilities
type ObservabilityReporter interface {
	MetricsReporter
	ControllerReporter
	TraceReporter

	// Health and summary
	GetMetricsSummary() map[string]interface{}
}

// Ensure our implementations satisfy these interfaces
var (
	_ MetricsReporter    = (*Reporter)(nil)
	_ ControllerReporter = (*HybridReporter)(nil)
	_ TraceReporter      = (*TraceHelper)(nil)
)
