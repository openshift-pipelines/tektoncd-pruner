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

package config

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateConfigMap_ValidGlobalConfig(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name: "valid global config with all fields",
			config: `enforcedConfigLevel: namespace
ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10
failedHistoryLimit: 10
historyLimit: 100`,
		},
		{
			name:   "valid global config with minimal fields",
			config: `ttlSecondsAfterFinished: 3600`,
		},
		{
			name: "valid global config with zero values",
			config: `ttlSecondsAfterFinished: 0
successfulHistoryLimit: 0
failedHistoryLimit: 0
historyLimit: 0`,
		},
		{
			name: "valid global config with namespace overrides",
			config: `ttlSecondsAfterFinished: 7200
successfulHistoryLimit: 20
namespaces:
  dev:
    ttlSecondsAfterFinished: 3600
    successfulHistoryLimit: 10`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: tt.config,
				},
			}

			err := ValidateConfigMap(cm)
			if err != nil {
				t.Errorf("ValidateConfigMap() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateConfigMap_InvalidGlobalConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantErrMsg string
	}{
		{
			name:       "negative TTL",
			config:     `ttlSecondsAfterFinished: -1`,
			wantErrMsg: "ttlSecondsAfterFinished cannot be negative",
		},
		{
			name:       "negative successfulHistoryLimit",
			config:     `successfulHistoryLimit: -5`,
			wantErrMsg: "successfulHistoryLimit cannot be negative",
		},
		{
			name:       "negative failedHistoryLimit",
			config:     `failedHistoryLimit: -10`,
			wantErrMsg: "failedHistoryLimit cannot be negative",
		},
		{
			name:       "negative historyLimit",
			config:     `historyLimit: -100`,
			wantErrMsg: "historyLimit cannot be negative",
		},
		{
			name:       "invalid enforcedConfigLevel",
			config:     `enforcedConfigLevel: invalid`,
			wantErrMsg: "invalid enforcedConfigLevel 'invalid'",
		},
		{
			name:       "invalid YAML",
			config:     `this is not: valid: yaml:`,
			wantErrMsg: "failed to parse global-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: tt.config,
				},
			}

			err := ValidateConfigMap(cm)
			if err == nil {
				t.Errorf("ValidateConfigMap() expected error containing '%s', got nil", tt.wantErrMsg)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("ValidateConfigMap() error = %v, want error containing %v", err, tt.wantErrMsg)
			}
		})
	}
}

func TestValidateConfigMap_ValidNamespaceConfig(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name: "valid namespace config with all fields",
			config: `ttlSecondsAfterFinished: 1800
successfulHistoryLimit: 5
failedHistoryLimit: 5
historyLimit: 50`,
		},
		{
			name:   "valid namespace config with minimal fields",
			config: `ttlSecondsAfterFinished: 600`,
		},
		{
			name: "valid namespace config with zero values",
			config: `ttlSecondsAfterFinished: 0
historyLimit: 0`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: "my-namespace",
				},
				Data: map[string]string{
					PrunerNamespaceConfigKey: tt.config,
				},
			}

			err := ValidateConfigMap(cm)
			if err != nil {
				t.Errorf("ValidateConfigMap() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateConfigMap_InvalidNamespaceConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantErrMsg string
	}{
		{
			name:       "negative TTL",
			config:     `ttlSecondsAfterFinished: -100`,
			wantErrMsg: "ttlSecondsAfterFinished cannot be negative",
		},
		{
			name:       "negative successfulHistoryLimit",
			config:     `successfulHistoryLimit: -1`,
			wantErrMsg: "successfulHistoryLimit cannot be negative",
		},
		{
			name:       "invalid YAML",
			config:     `bad yaml: [[[`,
			wantErrMsg: "failed to parse ns-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: "my-namespace",
				},
				Data: map[string]string{
					PrunerNamespaceConfigKey: tt.config,
				},
			}

			err := ValidateConfigMap(cm)
			if err == nil {
				t.Errorf("ValidateConfigMap() expected error containing '%s', got nil", tt.wantErrMsg)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("ValidateConfigMap() error = %v, want error containing %v", err, tt.wantErrMsg)
			}
		})
	}
}

func TestValidateConfigMapWithGlobal_NamespaceExceedsLimits(t *testing.T) {
	tests := []struct {
		name            string
		globalConfig    string
		namespaceConfig string
		wantErrMsg      string
	}{
		{
			name:            "namespace TTL exceeds global",
			globalConfig:    `ttlSecondsAfterFinished: 3600`,
			namespaceConfig: `ttlSecondsAfterFinished: 7200`,
			wantErrMsg:      "ttlSecondsAfterFinished (7200) cannot exceed global limit (3600)",
		},
		{
			name:            "namespace successfulHistoryLimit exceeds global",
			globalConfig:    `successfulHistoryLimit: 10`,
			namespaceConfig: `successfulHistoryLimit: 20`,
			wantErrMsg:      "successfulHistoryLimit (20) cannot exceed global limit (10)",
		},
		{
			name:            "namespace failedHistoryLimit exceeds global",
			globalConfig:    `failedHistoryLimit: 5`,
			namespaceConfig: `failedHistoryLimit: 15`,
			wantErrMsg:      "failedHistoryLimit (15) cannot exceed global limit (5)",
		},
		{
			name:            "namespace historyLimit exceeds global",
			globalConfig:    `historyLimit: 50`,
			namespaceConfig: `historyLimit: 100`,
			wantErrMsg:      "historyLimit (100) cannot exceed global limit (50)",
		},
		{
			name: "multiple fields exceed global",
			globalConfig: `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10
failedHistoryLimit: 10`,
			namespaceConfig: `ttlSecondsAfterFinished: 7200
successfulHistoryLimit: 20
failedHistoryLimit: 20`,
			wantErrMsg: "cannot exceed global limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: tt.globalConfig,
				},
			}

			namespaceCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: "my-namespace",
				},
				Data: map[string]string{
					PrunerNamespaceConfigKey: tt.namespaceConfig,
				},
			}

			err := ValidateConfigMapWithGlobal(namespaceCM, globalCM)
			if err == nil {
				t.Errorf("ValidateConfigMapWithGlobal() expected error containing '%s', got nil", tt.wantErrMsg)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("ValidateConfigMapWithGlobal() error = %v, want error containing %v", err, tt.wantErrMsg)
			}
		})
	}
}

func TestValidateConfigMapWithGlobal_NamespaceWithinLimits(t *testing.T) {
	tests := []struct {
		name            string
		globalConfig    string
		namespaceConfig string
	}{
		{
			name:            "namespace TTL within global",
			globalConfig:    `ttlSecondsAfterFinished: 3600`,
			namespaceConfig: `ttlSecondsAfterFinished: 1800`,
		},
		{
			name: "namespace history limits within global",
			globalConfig: `successfulHistoryLimit: 10
failedHistoryLimit: 10
historyLimit: 100`,
			namespaceConfig: `successfulHistoryLimit: 5
failedHistoryLimit: 5
historyLimit: 50`,
		},
		{
			name: "namespace equal to global limits",
			globalConfig: `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10`,
			namespaceConfig: `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10`,
		},
		{
			name: "namespace has subset of fields",
			globalConfig: `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10
failedHistoryLimit: 10`,
			namespaceConfig: `ttlSecondsAfterFinished: 1800`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: tt.globalConfig,
				},
			}

			namespaceCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: "my-namespace",
				},
				Data: map[string]string{
					PrunerNamespaceConfigKey: tt.namespaceConfig,
				},
			}

			err := ValidateConfigMapWithGlobal(namespaceCM, globalCM)
			if err != nil {
				t.Errorf("ValidateConfigMapWithGlobal() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateConfigMap_NestedNamespaceConfigExceedsGlobal(t *testing.T) {
	globalConfig := `ttlSecondsAfterFinished: 7200
successfulHistoryLimit: 20
namespaces:
  dev:
    ttlSecondsAfterFinished: 10800
    successfulHistoryLimit: 10`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tekton-pruner-default-spec",
			Namespace: "tekton-pipelines",
		},
		Data: map[string]string{
			PrunerGlobalConfigKey: globalConfig,
		},
	}

	err := ValidateConfigMap(cm)
	if err == nil {
		t.Error("ValidateConfigMap() expected error for nested namespace config exceeding global, got nil")
		return
	}
	if !strings.Contains(err.Error(), "global-config.namespaces.dev") {
		t.Errorf("ValidateConfigMap() error should mention nested namespace path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "cannot exceed global limit") {
		t.Errorf("ValidateConfigMap() error should mention limit exceeded, got: %v", err)
	}
}

func TestValidateConfigMap_EmptyConfigMap(t *testing.T) {
	tests := []struct {
		name string
		cm   *corev1.ConfigMap
	}{
		{
			name: "nil data",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: nil,
			},
		},
		{
			name: "empty data",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string]string{},
			},
		},
		{
			name: "empty config string",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigMap(tt.cm)
			if err != nil {
				t.Errorf("ValidateConfigMap() should accept empty config, got error = %v", err)
			}
		})
	}
}

func TestValidateConfigMap_ValidEnforcedConfigLevels(t *testing.T) {
	validLevels := []string{"global", "namespace", "resource"}

	for _, level := range validLevels {
		t.Run("enforcedConfigLevel="+level, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: "enforcedConfigLevel: " + level,
				},
			}

			err := ValidateConfigMap(cm)
			if err != nil {
				t.Errorf("ValidateConfigMap() unexpected error for valid level '%s': %v", level, err)
			}
		})
	}
}

func TestValidateConfigMapWithGlobal_NoGlobalConfigProvided(t *testing.T) {
	// Namespace config should still validate basic rules even without global config
	namespaceCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tekton-pruner-namespace-spec",
			Namespace: "my-namespace",
		},
		Data: map[string]string{
			PrunerNamespaceConfigKey: `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 100`,
		},
	}

	err := ValidateConfigMapWithGlobal(namespaceCM, nil)
	if err != nil {
		t.Errorf("ValidateConfigMapWithGlobal() should succeed without global config, got error = %v", err)
	}
}

func TestValidateConfigMapWithGlobal_InvalidGlobalConfig(t *testing.T) {
	// If global config is invalid, namespace validation should still work with basic validation
	globalCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tekton-pruner-default-spec",
			Namespace: "tekton-pipelines",
		},
		Data: map[string]string{
			PrunerGlobalConfigKey: `invalid yaml [[[`,
		},
	}

	namespaceCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tekton-pruner-namespace-spec",
			Namespace: "my-namespace",
		},
		Data: map[string]string{
			PrunerNamespaceConfigKey: `ttlSecondsAfterFinished: 1800`,
		},
	}

	// Should still validate the namespace config (without global limit enforcement)
	err := ValidateConfigMapWithGlobal(namespaceCM, globalCM)
	if err != nil {
		t.Errorf("ValidateConfigMapWithGlobal() should gracefully handle invalid global config, got error = %v", err)
	}
}

func TestValidateConfigMap_SystemMaximumEnforcement(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		configType string // "global" or "namespace"
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "global config can exceed system maximum TTL",
			configType: "global",
			config:     `ttlSecondsAfterFinished: 2592001`,
			wantErr:    false,
		},
		{
			name:       "global config at system maximum TTL",
			configType: "global",
			config:     `ttlSecondsAfterFinished: 2592000`,
			wantErr:    false,
		},
		{
			name:       "global config can exceed system maximum successfulHistoryLimit",
			configType: "global",
			config:     `successfulHistoryLimit: 101`,
			wantErr:    false,
		},
		{
			name:       "global config at system maximum successfulHistoryLimit",
			configType: "global",
			config:     `successfulHistoryLimit: 100`,
			wantErr:    false,
		},
		{
			name:       "global config can exceed system maximum failedHistoryLimit",
			configType: "global",
			config:     `failedHistoryLimit: 150`,
			wantErr:    false,
		},
		{
			name:       "global config at system maximum failedHistoryLimit",
			configType: "global",
			config:     `failedHistoryLimit: 100`,
			wantErr:    false,
		},
		{
			name:       "global config can exceed system maximum historyLimit",
			configType: "global",
			config:     `historyLimit: 200`,
			wantErr:    false,
		},
		{
			name:       "global config at system maximum historyLimit",
			configType: "global",
			config:     `historyLimit: 100`,
			wantErr:    false,
		},
		{
			name:       "namespace config exceeds system maximum TTL without global",
			configType: "namespace",
			config:     `ttlSecondsAfterFinished: 3000000`,
			wantErr:    true,
			wantErrMsg: "cannot exceed system maximum (2592000 seconds / 30 days)",
		},
		{
			name:       "namespace config within system maximum TTL",
			configType: "namespace",
			config:     `ttlSecondsAfterFinished: 2592000`,
			wantErr:    false,
		},
		{
			name:       "namespace config exceeds system maximum successfulHistoryLimit without global",
			configType: "namespace",
			config:     `successfulHistoryLimit: 150`,
			wantErr:    true,
			wantErrMsg: "cannot exceed system maximum (100)",
		},
		{
			name:       "namespace config within system maximum successfulHistoryLimit",
			configType: "namespace",
			config:     `successfulHistoryLimit: 50`,
			wantErr:    false,
		},
		{
			name:       "global config can have multiple fields exceeding system maximum",
			configType: "global",
			config: `ttlSecondsAfterFinished: 3000000
successfulHistoryLimit: 150
failedHistoryLimit: 200`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cm *corev1.ConfigMap
			if tt.configType == "global" {
				cm = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-pruner-default-spec",
						Namespace: "tekton-pipelines",
					},
					Data: map[string]string{
						PrunerGlobalConfigKey: tt.config,
					},
				}
			} else {
				cm = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tekton-pruner-namespace-spec",
						Namespace: "my-namespace",
					},
					Data: map[string]string{
						PrunerNamespaceConfigKey: tt.config,
					},
				}
			}

			err := ValidateConfigMap(cm)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateConfigMap() expected error containing '%s', got nil", tt.wantErrMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ValidateConfigMap() error = %v, want error containing %v", err, tt.wantErrMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateConfigMap() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateConfigMapWithGlobal_GlobalOverridesSystemMaximum(t *testing.T) {
	// When global config sets a stricter limit, it should override system maximum
	tests := []struct {
		name            string
		globalConfig    string
		namespaceConfig string
		wantErr         bool
		wantErrMsg      string
	}{
		{
			name:            "namespace within global limit (stricter than system max)",
			globalConfig:    `ttlSecondsAfterFinished: 1800`,
			namespaceConfig: `ttlSecondsAfterFinished: 1800`,
			wantErr:         false,
		},
		{
			name:            "namespace exceeds global limit but within system max",
			globalConfig:    `ttlSecondsAfterFinished: 1800`,
			namespaceConfig: `ttlSecondsAfterFinished: 3600`,
			wantErr:         true,
			wantErrMsg:      "cannot exceed global limit (1800)",
		},
		{
			name:            "namespace within system max when no global limit set",
			globalConfig:    `enforcedConfigLevel: namespace`,
			namespaceConfig: `ttlSecondsAfterFinished: 2592000`,
			wantErr:         false,
		},
		{
			name:            "namespace exceeds system max when no global limit set",
			globalConfig:    `enforcedConfigLevel: namespace`,
			namespaceConfig: `ttlSecondsAfterFinished: 2592001`,
			wantErr:         true,
			wantErrMsg:      "cannot exceed system maximum (2592000 seconds / 30 days)",
		},
		{
			name:            "global limit can be stricter than system default",
			globalConfig:    `successfulHistoryLimit: 10`,
			namespaceConfig: `successfulHistoryLimit: 50`,
			wantErr:         true,
			wantErrMsg:      "cannot exceed global limit (10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: tt.globalConfig,
				},
			}

			namespaceCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: "my-namespace",
				},
				Data: map[string]string{
					PrunerNamespaceConfigKey: tt.namespaceConfig,
				},
			}

			err := ValidateConfigMapWithGlobal(namespaceCM, globalCM)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateConfigMapWithGlobal() expected error containing '%s', got nil", tt.wantErrMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ValidateConfigMapWithGlobal() error = %v, want error containing %v", err, tt.wantErrMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateConfigMapWithGlobal() unexpected error = %v", err)
				}
			}
		})
	}
}
