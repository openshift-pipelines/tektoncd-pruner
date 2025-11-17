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

// TestValidateConfigMapWithGlobal_HierarchicalFallback tests the hierarchical fallback logic:
// 1. If global granular limit (successfulHistoryLimit/failedHistoryLimit) is set, use it
// 2. If only global historyLimit is set, use it for all history-based limits
// 3. If neither is set, use system maximum
func TestValidateConfigMapWithGlobal_HierarchicalFallback(t *testing.T) {
	tests := []struct {
		name            string
		globalConfig    string
		namespaceConfig string
		wantErr         bool
		wantErrMsg      string
	}{
		{
			name:            "namespace successfulHistoryLimit uses global historyLimit when no granular limit",
			globalConfig:    `historyLimit: 50`,
			namespaceConfig: `successfulHistoryLimit: 50`,
			wantErr:         false,
		},
		{
			name:            "namespace successfulHistoryLimit exceeds global historyLimit when no granular limit",
			globalConfig:    `historyLimit: 50`,
			namespaceConfig: `successfulHistoryLimit: 51`,
			wantErr:         true,
			wantErrMsg:      "cannot exceed global historyLimit (50)",
		},
		{
			name:            "namespace failedHistoryLimit uses global historyLimit when no granular limit",
			globalConfig:    `historyLimit: 50`,
			namespaceConfig: `failedHistoryLimit: 50`,
			wantErr:         false,
		},
		{
			name:            "namespace failedHistoryLimit exceeds global historyLimit when no granular limit",
			globalConfig:    `historyLimit: 50`,
			namespaceConfig: `failedHistoryLimit: 51`,
			wantErr:         true,
			wantErrMsg:      "cannot exceed global historyLimit (50)",
		},
		{
			name: "granular limit takes precedence over historyLimit",
			globalConfig: `historyLimit: 50
successfulHistoryLimit: 30`,
			namespaceConfig: `successfulHistoryLimit: 30`,
			wantErr:         false,
		},
		{
			name: "namespace exceeds granular limit even though within historyLimit",
			globalConfig: `historyLimit: 50
successfulHistoryLimit: 30`,
			namespaceConfig: `successfulHistoryLimit: 40`,
			wantErr:         true,
			wantErrMsg:      "cannot exceed global limit (30)",
		},
		{
			name: "both granular limits set, namespace within both",
			globalConfig: `successfulHistoryLimit: 40
failedHistoryLimit: 20`,
			namespaceConfig: `successfulHistoryLimit: 40
failedHistoryLimit: 20`,
			wantErr: false,
		},
		{
			name:         "only one granular limit set, other uses system maximum",
			globalConfig: `successfulHistoryLimit: 40`,
			namespaceConfig: `successfulHistoryLimit: 40
failedHistoryLimit: 100`,
			wantErr: false,
		},
		{
			name:         "only one granular limit set, other exceeds system maximum",
			globalConfig: `successfulHistoryLimit: 40`,
			namespaceConfig: `successfulHistoryLimit: 40
failedHistoryLimit: 101`,
			wantErr:    true,
			wantErrMsg: "cannot exceed system maximum (100)",
		},
		{
			name:            "no global limits at all, namespace uses system maximum",
			globalConfig:    `ttlSecondsAfterFinished: 3600`,
			namespaceConfig: `successfulHistoryLimit: 100`,
			wantErr:         false,
		},
		{
			name:            "no global limits, namespace exceeds system maximum",
			globalConfig:    `ttlSecondsAfterFinished: 3600`,
			namespaceConfig: `successfulHistoryLimit: 101`,
			wantErr:         true,
			wantErrMsg:      "cannot exceed system maximum (100)",
		},
		{
			name:         "global historyLimit 200, namespace successful 200 allowed",
			globalConfig: `historyLimit: 200`,
			namespaceConfig: `successfulHistoryLimit: 200
failedHistoryLimit: 200`,
			wantErr: false,
		},
		{
			name:         "global historyLimit 200, namespace exceeds",
			globalConfig: `historyLimit: 200`,
			namespaceConfig: `successfulHistoryLimit: 201
failedHistoryLimit: 150`,
			wantErr:    true,
			wantErrMsg: "cannot exceed global historyLimit (200)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PrunerConfigMapName,
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					PrunerGlobalConfigKey: tt.globalConfig,
				},
			}

			namespaceCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PrunerNamespaceConfigMapName,
					Namespace: "test-namespace",
				},
				Data: map[string]string{
					PrunerNamespaceConfigKey: tt.namespaceConfig,
				},
			}

			err := ValidateConfigMapWithGlobal(namespaceCM, globalCM)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.wantErrMsg)
				} else if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.wantErrMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
