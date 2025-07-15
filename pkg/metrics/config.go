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
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

// ObservabilityConfig holds configuration for the observability system
type ObservabilityConfig struct {
	// Metrics configuration
	MetricsEnabled  bool          `json:"metrics_enabled"`
	MetricsBackend  string        `json:"metrics_backend"`
	MetricsPort     int           `json:"metrics_port"`
	MetricsDomain   string        `json:"metrics_domain"`
	MetricsPrefix   string        `json:"metrics_prefix"`
	MetricsInterval time.Duration `json:"metrics_interval"`

	// Tracing configuration
	TracingEnabled    bool    `json:"tracing_enabled"`
	TracingBackend    string  `json:"tracing_backend"`
	TracingEndpoint   string  `json:"tracing_endpoint"`
	TracingSampleRate float64 `json:"tracing_sample_rate"`

	// Performance configuration
	MaxMetricCardinality int  `json:"max_metric_cardinality"`
	EnableDebugMetrics   bool `json:"enable_debug_metrics"`

	mu sync.RWMutex
}

// NewDefaultConfig returns a configuration with sensible defaults
func NewDefaultConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		// Metrics defaults
		MetricsEnabled:  true,
		MetricsBackend:  "prometheus",
		MetricsPort:     9090,
		MetricsDomain:   Domain,
		MetricsPrefix:   "tektoncd_pruner_",
		MetricsInterval: 15 * time.Second,

		// Tracing defaults
		TracingEnabled:    true,
		TracingBackend:    "jaeger",
		TracingEndpoint:   "",
		TracingSampleRate: 0.1, // 10% sampling

		// Performance defaults
		MaxMetricCardinality: 10000,
		EnableDebugMetrics:   false,
	}
}

// LoadFromConfigMap loads configuration from a Kubernetes ConfigMap
func (c *ObservabilityConfig) LoadFromConfigMap(configMap *corev1.ConfigMap) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if configMap == nil || configMap.Data == nil {
		return nil // Use defaults
	}

	data := configMap.Data

	// Metrics configuration
	if backend, exists := data["metrics.backend-destination"]; exists {
		c.MetricsBackend = backend
	}

	if enabled, exists := data["metrics.enabled"]; exists {
		if val, err := strconv.ParseBool(enabled); err == nil {
			c.MetricsEnabled = val
		}
	}

	if port, exists := data["metrics.port"]; exists {
		if val, err := strconv.Atoi(port); err == nil {
			c.MetricsPort = val
		}
	}

	if domain, exists := data["metrics.domain"]; exists {
		c.MetricsDomain = domain
	}

	if prefix, exists := data["metrics.prefix"]; exists {
		c.MetricsPrefix = prefix
	}

	// Tracing configuration
	if enabled, exists := data["tracing.enabled"]; exists {
		if val, err := strconv.ParseBool(enabled); err == nil {
			c.TracingEnabled = val
		}
	}

	if backend, exists := data["tracing.backend"]; exists {
		c.TracingBackend = backend
	}

	if endpoint, exists := data["tracing.endpoint"]; exists {
		c.TracingEndpoint = endpoint
	}

	if rate, exists := data["tracing.sample-rate"]; exists {
		if val, err := strconv.ParseFloat(rate, 64); err == nil {
			c.TracingSampleRate = val
		}
	}

	// Performance configuration
	if cardinality, exists := data["metrics.max-cardinality"]; exists {
		if val, err := strconv.Atoi(cardinality); err == nil {
			c.MaxMetricCardinality = val
		}
	}

	if debug, exists := data["metrics.debug"]; exists {
		if val, err := strconv.ParseBool(debug); err == nil {
			c.EnableDebugMetrics = val
		}
	}

	return nil
}

// LoadFromEnvironment loads configuration from environment variables
func (c *ObservabilityConfig) LoadFromEnvironment() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Metrics environment variables
	if val := os.Getenv("METRICS_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			c.MetricsEnabled = enabled
		}
	}

	if val := os.Getenv("METRICS_BACKEND"); val != "" {
		c.MetricsBackend = val
	}

	if val := os.Getenv("METRICS_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.MetricsPort = port
		}
	}

	if val := os.Getenv("METRICS_DOMAIN"); val != "" {
		c.MetricsDomain = val
	}

	// Tracing environment variables
	if val := os.Getenv("TRACING_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			c.TracingEnabled = enabled
		}
	}

	if val := os.Getenv("TRACING_BACKEND"); val != "" {
		c.TracingBackend = val
	}

	if val := os.Getenv("TRACING_ENDPOINT"); val != "" {
		c.TracingEndpoint = val
	}

	if val := os.Getenv("TRACING_SAMPLE_RATE"); val != "" {
		if rate, err := strconv.ParseFloat(val, 64); err == nil {
			c.TracingSampleRate = rate
		}
	}
}

// Validate validates the configuration
func (c *ObservabilityConfig) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var errs []error

	// Validate metrics configuration
	if c.MetricsEnabled {
		if c.MetricsBackend == "" {
			errs = append(errs, errors.New("metrics backend cannot be empty when metrics are enabled"))
		}

		if c.MetricsPort <= 0 || c.MetricsPort > 65535 {
			errs = append(errs, fmt.Errorf("invalid metrics port: %d", c.MetricsPort))
		}

		if c.MaxMetricCardinality <= 0 {
			errs = append(errs, fmt.Errorf("max metric cardinality must be positive: %d", c.MaxMetricCardinality))
		}
	}

	// Validate tracing configuration
	if c.TracingEnabled {
		if c.TracingSampleRate < 0 || c.TracingSampleRate > 1 {
			errs = append(errs, fmt.Errorf("tracing sample rate must be between 0 and 1: %f", c.TracingSampleRate))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed: %v", errs)
	}

	return nil
}

// GetMetricsConfig returns metrics-specific configuration safely
func (c *ObservabilityConfig) GetMetricsConfig() (enabled bool, backend string, port int, domain string, prefix string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MetricsEnabled, c.MetricsBackend, c.MetricsPort, c.MetricsDomain, c.MetricsPrefix
}

// GetTracingConfig returns tracing-specific configuration safely
func (c *ObservabilityConfig) GetTracingConfig() (enabled bool, backend string, endpoint string, sampleRate float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.TracingEnabled, c.TracingBackend, c.TracingEndpoint, c.TracingSampleRate
}

// IsMetricsEnabled returns whether metrics are enabled
func (c *ObservabilityConfig) IsMetricsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MetricsEnabled
}

// IsTracingEnabled returns whether tracing is enabled
func (c *ObservabilityConfig) IsTracingEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.TracingEnabled
}

// Clone creates a deep copy of the configuration
func (c *ObservabilityConfig) Clone() *ObservabilityConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &ObservabilityConfig{
		MetricsEnabled:       c.MetricsEnabled,
		MetricsBackend:       c.MetricsBackend,
		MetricsPort:          c.MetricsPort,
		MetricsDomain:        c.MetricsDomain,
		MetricsPrefix:        c.MetricsPrefix,
		MetricsInterval:      c.MetricsInterval,
		TracingEnabled:       c.TracingEnabled,
		TracingBackend:       c.TracingBackend,
		TracingEndpoint:      c.TracingEndpoint,
		TracingSampleRate:    c.TracingSampleRate,
		MaxMetricCardinality: c.MaxMetricCardinality,
		EnableDebugMetrics:   c.EnableDebugMetrics,
	}
}

// ConfigManager manages observability configuration lifecycle
type ConfigManager struct {
	config *ObservabilityConfig
	logger *zap.SugaredLogger
	mu     sync.RWMutex
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(logger *zap.SugaredLogger) *ConfigManager {
	return &ConfigManager{
		config: NewDefaultConfig(),
		logger: logger,
	}
}

// LoadConfig loads configuration from multiple sources in priority order:
// 1. Environment variables (highest priority)
// 2. ConfigMap
// 3. Defaults (lowest priority)
func (cm *ConfigManager) LoadConfig(ctx context.Context, configMap *corev1.ConfigMap) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Start with defaults
	newConfig := NewDefaultConfig()

	// Load from ConfigMap if provided
	if err := newConfig.LoadFromConfigMap(configMap); err != nil {
		return fmt.Errorf("failed to load from ConfigMap: %w", err)
	}

	// Load from environment (highest priority)
	newConfig.LoadFromEnvironment()

	// Validate the configuration
	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	cm.config = newConfig
	cm.logger.Infow("Configuration loaded successfully",
		"metrics_enabled", newConfig.MetricsEnabled,
		"metrics_backend", newConfig.MetricsBackend,
		"tracing_enabled", newConfig.TracingEnabled,
		"tracing_backend", newConfig.TracingBackend)

	return nil
}

// GetConfig returns the current configuration
func (cm *ConfigManager) GetConfig() *ObservabilityConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.Clone()
}
