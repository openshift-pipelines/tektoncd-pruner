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
)

// ErrorReporter handles error reporting and tracking for observability
type ErrorReporter struct {
	reporter MetricsReporter
	tracer   TraceReporter
	logger   *zap.SugaredLogger

	// Error rate limiting
	errorCounts map[string]int
	lastReset   time.Time
	mu          sync.RWMutex
}

// NewErrorReporter creates a new error reporter
func NewErrorReporter(reporter MetricsReporter, tracer TraceReporter, logger *zap.SugaredLogger) *ErrorReporter {
	return &ErrorReporter{
		reporter:    reporter,
		tracer:      tracer,
		logger:      logger,
		errorCounts: make(map[string]int),
		lastReset:   time.Now(),
	}
}

// ReportError reports an error with consistent patterns
func (er *ErrorReporter) ReportError(ctx context.Context, err error, operation, namespace, resourceType string) {
	if err == nil {
		return
	}

	// Categorize the error
	category := er.categorizeError(err, operation)

	// Report to metrics
	if er.reporter != nil {
		er.reporter.ReportResourceError(namespace, resourceType, category)
	}

	// Report to tracing
	if er.tracer != nil && er.tracer.IsEnabled() {
		er.tracer.TraceError(ctx, err, fmt.Sprintf("%s failed", operation))
	}

	// Log the error
	er.logger.Errorw("Operation failed",
		"error", err,
		"operation", operation,
		"namespace", namespace,
		"resource_type", resourceType,
		"category", category)

	// Track error frequency
	er.trackErrorFrequency(category)
}

// ReportReconcileError reports a reconciliation error
func (er *ErrorReporter) ReportReconcileError(ctx context.Context, err error, namespace, resourceType, phase string) {
	if err == nil {
		return
	}

	operation := fmt.Sprintf("reconcile_%s", phase)
	er.ReportError(ctx, err, operation, namespace, resourceType)
}

// ReportConfigError reports a configuration error
func (er *ErrorReporter) ReportConfigError(ctx context.Context, err error, configType string) {
	if err == nil {
		return
	}

	if er.reporter != nil {
		er.reporter.ReportConfigurationError(configType)
	}

	if er.tracer != nil && er.tracer.IsEnabled() {
		er.tracer.TraceError(ctx, err, "Configuration error")
	}

	er.logger.Errorw("Configuration error",
		"error", err,
		"config_type", configType)
}

// ReportInitializationError reports an initialization error
func (er *ErrorReporter) ReportInitializationError(ctx context.Context, err error, component string) {
	if err == nil {
		return
	}

	if er.tracer != nil && er.tracer.IsEnabled() {
		er.tracer.TraceError(ctx, err, fmt.Sprintf("%s initialization failed", component))
	}

	er.logger.Fatalw("Component initialization failed",
		"error", err,
		"component", component)
}

// categorizeError categorizes errors for better reporting
func (er *ErrorReporter) categorizeError(err error, operation string) string {
	errStr := err.Error()

	// Common error patterns
	switch {
	case containsAny(errStr, "not found", "404"):
		return "not_found"
	case containsAny(errStr, "forbidden", "403", "unauthorized", "401"):
		return "permission_denied"
	case containsAny(errStr, "timeout", "deadline exceeded", "context canceled"):
		return "timeout"
	case containsAny(errStr, "connection refused", "network"):
		return "network_error"
	case containsAny(errStr, "conflict", "409"):
		return "conflict"
	case containsAny(errStr, "rate limit", "too many requests", "429"):
		return "rate_limited"
	case containsAny(errStr, "internal server error", "500"):
		return "server_error"
	case containsAny(errStr, "bad request", "400", "invalid"):
		return "validation_error"
	case containsAny(errStr, "quota", "limit exceeded"):
		return "quota_exceeded"
	default:
		return "unknown"
	}
}

// trackErrorFrequency tracks error frequency for alerting
func (er *ErrorReporter) trackErrorFrequency(category string) {
	er.mu.Lock()
	defer er.mu.Unlock()

	// Reset counters every hour
	if time.Since(er.lastReset) > time.Hour {
		er.errorCounts = make(map[string]int)
		er.lastReset = time.Now()
	}

	er.errorCounts[category]++

	// Alert on high error rates
	if er.errorCounts[category] > 50 { // More than 50 errors of same type per hour
		er.logger.Warnw("High error rate detected",
			"category", category,
			"count", er.errorCounts[category],
			"window", "1 hour")
	}
}

// GetErrorStats returns current error statistics
func (er *ErrorReporter) GetErrorStats() map[string]int {
	er.mu.RLock()
	defer er.mu.RUnlock()

	stats := make(map[string]int)
	for k, v := range er.errorCounts {
		stats[k] = v
	}
	return stats
}

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// RecoverPanic recovers from panics and reports them as errors
func (er *ErrorReporter) RecoverPanic(ctx context.Context, operation, namespace, resourceType string) {
	if r := recover(); r != nil {
		err := fmt.Errorf("panic in %s: %v", operation, r)

		// Report as critical error
		if er.reporter != nil {
			er.reporter.ReportResourceError(namespace, resourceType, "panic")
		}

		if er.tracer != nil && er.tracer.IsEnabled() {
			er.tracer.TraceError(ctx, err, "Panic recovered")
		}

		er.logger.Errorw("Panic recovered",
			"error", err,
			"operation", operation,
			"namespace", namespace,
			"resource_type", resourceType)
	}
}

// WithErrorReporting wraps a function with error reporting
func (er *ErrorReporter) WithErrorReporting(ctx context.Context, operation, namespace, resourceType string, fn func() error) error {
	defer er.RecoverPanic(ctx, operation, namespace, resourceType)

	err := fn()
	if err != nil {
		er.ReportError(ctx, err, operation, namespace, resourceType)
	}

	return err
}
