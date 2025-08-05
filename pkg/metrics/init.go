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

	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Global instances
	globalReporter *Reporter
	globalTracer   *TraceHelper
	initOnce       sync.Once
	exporter       *prometheus.Exporter
)

// Setup initializes the OpenTelemetry observability system for the pruner
func Setup(ctx context.Context, logger *zap.SugaredLogger) error {
	var setupErr error
	initOnce.Do(func() {
		// Set up Prometheus exporter
		exp, err := SetupPrometheusExporter()
		if err != nil {
			setupErr = err
			return
		}
		exporter = exp

		// Initialize the metrics reporter
		reporter, err := NewReporter(ctx)
		if err != nil {
			setupErr = err
			return
		}
		globalReporter = reporter

		// Initialize the trace helper
		globalTracer = NewTraceHelper()

		logger.Info("OpenTelemetry observability system initialized successfully")
	})

	return setupErr
}

// InitializeMetrics initializes metrics with the given configuration
func InitializeMetrics(ctx context.Context, configMap *corev1.ConfigMap, logger *zap.SugaredLogger) error {
	if configMap == nil {
		logger.Warn("No observability config map provided, using defaults")
		return Setup(ctx, logger)
	}

	// For OpenTelemetry, we can read custom configuration from the ConfigMap
	// For now, use the default setup but this can be extended
	logger.Info("Using OpenTelemetry configuration")

	// Initialize our internal components
	return Setup(ctx, logger)
}

// GetReporter returns the global metrics reporter instance
func GetReporter() *Reporter {
	return globalReporter
}

// GetTracer returns the global trace helper instance
func GetTracer() *TraceHelper {
	return globalTracer
}

// SetupWithConfigMapWatcher sets up observability with a ConfigMap watcher
func SetupWithConfigMapWatcher(ctx context.Context, logger *zap.SugaredLogger) func(*corev1.ConfigMap) {
	// Initialize with default configuration first
	if err := Setup(ctx, logger); err != nil {
		logger.Fatalw("Failed to setup observability", "error", err)
	}

	// Return a function that can be used as a ConfigMap watcher
	return func(configMap *corev1.ConfigMap) {
		if configMap == nil {
			logger.Warn("Received nil config map in watcher")
			return
		}

		logger.Infow("Updating observability configuration", "configMap", configMap.Name)

		if err := InitializeMetrics(ctx, configMap, logger); err != nil {
			logger.Errorw("Failed to update metrics configuration", "error", err)
			// Report this as a metric
			if globalReporter != nil {
				globalReporter.ReportConfigurationError("configmap")
				globalReporter.ReportError("", "", "config_reload", err.Error())
			}
			return
		}

		// Report successful reload
		if globalReporter != nil {
			globalReporter.ReportConfigurationReload("configmap")
		}

		logger.Info("Observability configuration updated successfully")
	}
}

// FlushMetrics flushes any pending metrics (no-op for OpenTelemetry/Prometheus)
func FlushMetrics() {
	// OpenTelemetry with Prometheus exporter doesn't require explicit flushing
	// Metrics are pulled by Prometheus scraper
}

// MustSetup sets up observability and panics on error
func MustSetup(ctx context.Context, logger *zap.SugaredLogger) {
	if err := Setup(ctx, logger); err != nil {
		logger.Fatalw("Failed to setup observability", "error", err)
	}
}

// GetPrometheusExporter returns the prometheus exporter for HTTP handler
func GetPrometheusExporter() *prometheus.Exporter {
	return exporter
}
