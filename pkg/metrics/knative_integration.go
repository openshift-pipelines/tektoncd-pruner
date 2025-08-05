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
	"os"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	knativemetrics "knative.dev/pkg/metrics"
)

// KnativeIntegration provides integration with Knative's observability configuration
// while maintaining our OpenTelemetry-native implementation
type KnativeIntegration struct {
	domain    string
	component string
}

// NewKnativeIntegration creates a new Knative observability integration
func NewKnativeIntegration(domain, component string) *KnativeIntegration {
	return &KnativeIntegration{
		domain:    domain,
		component: component,
	}
}

// SetupWithKnativeConfig initializes metrics using Knative's configuration approach
// but with our OpenTelemetry implementation underneath
func SetupWithKnativeConfig(ctx context.Context, logger *zap.SugaredLogger, configMap *corev1.ConfigMap) error {
	// Read Knative-style configuration
	config := parseKnativeConfig(configMap)

	// Configure our OpenTelemetry setup based on Knative config
	if config.MetricsBackend == "prometheus" {
		logger.Info("Knative config specifies Prometheus backend - using OpenTelemetry Prometheus exporter")
		return Setup(ctx, logger)
	}

	// For other backends, we still use OpenTelemetry but could add other exporters
	logger.Info("Using OpenTelemetry with default Prometheus exporter")
	return Setup(ctx, logger)
}

// KnativeObservabilityConfig represents Knative's observability configuration
type KnativeObservabilityConfig struct {
	MetricsBackend string
	Domain         string
	Component      string
}

// parseKnativeConfig parses Knative's config-observability ConfigMap format
func parseKnativeConfig(configMap *corev1.ConfigMap) *KnativeObservabilityConfig {
	config := &KnativeObservabilityConfig{
		MetricsBackend: "prometheus", // default
		Domain:         Domain,
		Component:      Component,
	}

	if configMap != nil && configMap.Data != nil {
		if backend, exists := configMap.Data["metrics.backend-destination"]; exists {
			config.MetricsBackend = backend
		}
	}

	// Check environment variables (Knative style)
	if domain := os.Getenv("METRICS_DOMAIN"); domain != "" {
		config.Domain = domain
	}

	return config
}

// InitializeKnativeCompatibleMetrics sets up metrics with Knative-style configuration
// This provides compatibility with Knative's observability system while using OpenTelemetry
func InitializeKnativeCompatibleMetrics(ctx context.Context) error {
	logger := zap.S()

	// Use the same exporter options that Knative would expect
	exporterOpts := knativemetrics.ExporterOptions{
		Domain:    Domain,
		Component: Component,
		ConfigMap: nil, // Will be set by config watcher
	}

	logger.Infow("Initializing Knative-compatible metrics with OpenTelemetry",
		"domain", exporterOpts.Domain,
		"component", exporterOpts.Component)

	// Setup our OpenTelemetry implementation
	return Setup(ctx, logger)
}

// ReportWithKnativeContext reports metrics using Knative-style context but OpenTelemetry underneath
func ReportWithKnativeContext(ctx context.Context, namespace, resourceType, operation string, value int64) {
	reporter := GetReporter()
	if reporter == nil {
		return
	}

	// Map Knative-style operations to our metrics
	switch operation {
	case "reconcile":
		reporter.ReportResourceProcessed(namespace, resourceType, "processing")
	case "reconcile_success":
		reporter.ReportResourceProcessed(namespace, resourceType, "success")
	case "reconcile_error":
		reporter.ReportResourceProcessed(namespace, resourceType, "error")
	case "queue_depth":
		reporter.ReportCurrentResourcesQueued(namespace, resourceType, value)
	}
}
