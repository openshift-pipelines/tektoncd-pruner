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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Trace operation names
	TraceOpReconcileTaskRun     = "reconcile-taskrun"
	TraceOpReconcilePipelineRun = "reconcile-pipelinerun"
	TraceOpGarbageCollection    = "garbage-collection"
	TraceOpNamespaceProcessing  = "namespace-processing"
	TraceOpTTLCleanup           = "ttl-cleanup"
	TraceOpHistoryCleanup       = "history-cleanup"
	TraceOpResourceProcessing   = "resource-processing"
	TraceOpConfigMapReload      = "configmap-reload"

	// Tracer name for OpenTelemetry
	TracerName = "github.com/openshift-pipelines/tektoncd-pruner"
)

// TraceHelper provides tracing utilities for the pruner using OpenTelemetry
type TraceHelper struct {
	tracer  trace.Tracer
	enabled bool
}

// NewTraceHelper creates a new OpenTelemetry trace helper
func NewTraceHelper() *TraceHelper {
	return &TraceHelper{
		tracer:  otel.Tracer(TracerName),
		enabled: true,
	}
}

// StartSpan starts a new trace span with the given operation name
func (t *TraceHelper) StartSpan(ctx context.Context, operationName string) (context.Context, trace.Span) {
	if !t.enabled {
		return ctx, trace.SpanFromContext(ctx)
	}
	return t.tracer.Start(ctx, operationName)
}

// StartSpanWithAttributes starts a new trace span with attributes
func (t *TraceHelper) StartSpanWithAttributes(ctx context.Context, operationName string, attrs map[string]interface{}) (context.Context, trace.Span) {
	if !t.enabled {
		return ctx, trace.SpanFromContext(ctx)
	}

	// Convert attributes to OpenTelemetry format
	otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
	for key, value := range attrs {
		otelAttrs = append(otelAttrs, convertToAttribute(key, value))
	}

	return t.tracer.Start(ctx, operationName, trace.WithAttributes(otelAttrs...))
}

// convertToAttribute converts various types to OpenTelemetry attributes
func convertToAttribute(key string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case int:
		return attribute.Int(key, v)
	case int32:
		return attribute.Int(key, int(v))
	case int64:
		return attribute.Int64(key, v)
	case bool:
		return attribute.Bool(key, v)
	case float64:
		return attribute.Float64(key, v)
	case float32:
		return attribute.Float64(key, float64(v))
	default:
		// For unknown types, convert to string representation
		return attribute.String(key, "unknown")
	}
}

// AddAttributes adds multiple attributes to the current span
func (t *TraceHelper) AddAttributes(ctx context.Context, attrs map[string]interface{}) {
	if !t.enabled {
		return
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	// Convert attributes to OpenTelemetry format
	otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
	for key, value := range attrs {
		otelAttrs = append(otelAttrs, convertToAttribute(key, value))
	}

	span.SetAttributes(otelAttrs...)
}

// AddAttribute adds a single attribute to the current span
func (t *TraceHelper) AddAttribute(ctx context.Context, key string, value interface{}) {
	if !t.enabled {
		return
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(convertToAttribute(key, value))
}

// AddAnnotation adds an event to the current span (equivalent to OpenCensus annotation)
func (t *TraceHelper) AddAnnotation(ctx context.Context, message string, attrs map[string]interface{}) {
	if !t.enabled {
		return
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	// Convert attributes to OpenTelemetry format
	otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
	for key, value := range attrs {
		otelAttrs = append(otelAttrs, convertToAttribute(key, value))
	}

	span.AddEvent(message, trace.WithAttributes(otelAttrs...))
}

// SetStatus sets the status of the current span
func (t *TraceHelper) SetStatus(ctx context.Context, code codes.Code, message string) {
	if !t.enabled {
		return
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetStatus(code, message)
}

// EndSpan safely ends a span
func (t *TraceHelper) EndSpan(span trace.Span) {
	if t.enabled && span != nil {
		span.End()
	}
}

// TraceReconcile traces a reconcile operation
func (t *TraceHelper) TraceReconcile(ctx context.Context, resourceType, namespace, name string) (context.Context, trace.Span) {
	var operationName string
	switch resourceType {
	case "taskrun":
		operationName = TraceOpReconcileTaskRun
	case "pipelinerun":
		operationName = TraceOpReconcilePipelineRun
	default:
		operationName = "reconcile-" + resourceType
	}

	attributes := map[string]interface{}{
		"resource.type":      resourceType,
		"resource.namespace": namespace,
		"resource.name":      name,
	}

	return t.StartSpanWithAttributes(ctx, operationName, attributes)
}

// TraceResourceProcessing traces resource processing operations
func (t *TraceHelper) TraceResourceProcessing(ctx context.Context, operation, resourceType, namespace, name string) (context.Context, trace.Span) {
	attributes := map[string]interface{}{
		"operation":          operation,
		"resource.type":      resourceType,
		"resource.namespace": namespace,
		"resource.name":      name,
	}

	return t.StartSpanWithAttributes(ctx, TraceOpResourceProcessing, attributes)
}

// TraceGarbageCollection traces garbage collection operations
func (t *TraceHelper) TraceGarbageCollection(ctx context.Context, namespacesCount int) (context.Context, trace.Span) {
	attributes := map[string]interface{}{
		"namespaces.count": namespacesCount,
	}

	return t.StartSpanWithAttributes(ctx, TraceOpGarbageCollection, attributes)
}

// TraceNamespaceProcessing traces namespace processing
func (t *TraceHelper) TraceNamespaceProcessing(ctx context.Context, namespace string, workerID int) (context.Context, trace.Span) {
	attributes := map[string]interface{}{
		"namespace": namespace,
		"worker.id": workerID,
	}

	return t.StartSpanWithAttributes(ctx, TraceOpNamespaceProcessing, attributes)
}

// TraceTTLCleanup traces TTL cleanup operations
func (t *TraceHelper) TraceTTLCleanup(ctx context.Context, resourceType, namespace, name string, ttlSeconds int32) (context.Context, trace.Span) {
	attributes := map[string]interface{}{
		"resource.type":      resourceType,
		"resource.namespace": namespace,
		"resource.name":      name,
		"ttl.seconds":        ttlSeconds,
	}

	return t.StartSpanWithAttributes(ctx, TraceOpTTLCleanup, attributes)
}

// TraceHistoryCleanup traces history cleanup operations
func (t *TraceHelper) TraceHistoryCleanup(ctx context.Context, resourceType, namespace, name string, limit int32, limitType string) (context.Context, trace.Span) {
	attributes := map[string]interface{}{
		"resource.type":      resourceType,
		"resource.namespace": namespace,
		"resource.name":      name,
		"history.limit":      limit,
		"history.type":       limitType,
	}

	return t.StartSpanWithAttributes(ctx, TraceOpHistoryCleanup, attributes)
}

// TraceConfigMapReload traces configuration reload operations
func (t *TraceHelper) TraceConfigMapReload(ctx context.Context, configMapName string) (context.Context, trace.Span) {
	attributes := map[string]interface{}{
		"configmap.name": configMapName,
	}

	return t.StartSpanWithAttributes(ctx, TraceOpConfigMapReload, attributes)
}

// TraceError records an error in the current span
func (t *TraceHelper) TraceError(ctx context.Context, err error, message string) {
	if !t.enabled || err == nil {
		return
	}

	t.AddAnnotation(ctx, "error: "+message, map[string]interface{}{
		"error.message": err.Error(),
	})

	t.SetStatus(ctx, codes.Error, err.Error())
}

// TraceDeletion records a successful deletion
func (t *TraceHelper) TraceDeletion(ctx context.Context, resourceType, namespace, name, reason string) {
	if !t.enabled {
		return
	}

	t.AddAnnotation(ctx, "resource deleted", map[string]interface{}{
		"resource.type":      resourceType,
		"resource.namespace": namespace,
		"resource.name":      name,
		"deletion.reason":    reason,
	})
}

// TraceConfigurationChange records configuration changes
func (t *TraceHelper) TraceConfigurationChange(ctx context.Context, configType, namespace, resourceType string, oldValue, newValue interface{}) {
	if !t.enabled {
		return
	}

	t.AddAnnotation(ctx, "configuration changed", map[string]interface{}{
		"config.type":      configType,
		"config.namespace": namespace,
		"config.resource":  resourceType,
		"config.old_value": oldValue,
		"config.new_value": newValue,
	})
}

// Disable disables tracing
func (t *TraceHelper) Disable() {
	t.enabled = false
}

// Enable enables tracing
func (t *TraceHelper) Enable() {
	t.enabled = true
}

// IsEnabled returns whether tracing is enabled
func (t *TraceHelper) IsEnabled() bool {
	return t.enabled
}
