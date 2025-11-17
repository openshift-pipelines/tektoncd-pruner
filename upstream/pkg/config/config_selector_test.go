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

// Helper function to create int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}

// TestValidateConfigMap_SelectorsInGlobal verifies selector support rules
func TestValidateConfigMap_SelectorsInGlobal(t *testing.T) {
	tests := []struct {
		name       string
		configKey  string
		config     string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:      "global config with pipelineRuns selector rejected",
			configKey: PrunerGlobalConfigKey,
			config: `ttlSecondsAfterFinished: 3600
namespaces:
  dev:
    pipelineRuns:
      - selector:
          - matchLabels:
              app: myapp
        ttlSecondsAfterFinished: 1800`,
			wantErr:    true,
			wantErrMsg: "selectors are NOT supported in global ConfigMap",
		},
		{
			name:      "global config with taskRuns selector rejected",
			configKey: PrunerGlobalConfigKey,
			config: `ttlSecondsAfterFinished: 3600
namespaces:
  prod:
    taskRuns:
      - selector:
          - matchAnnotations:
              team: platform
        successfulHistoryLimit: 5`,
			wantErr:    true,
			wantErrMsg: "selectors are NOT supported in global ConfigMap",
		},
		{
			name:      "namespace config with pipelineRuns selector accepted",
			configKey: PrunerNamespaceConfigKey,
			config: `pipelineRuns:
  - selector:
      - matchLabels:
          app: myapp
    ttlSecondsAfterFinished: 1800`,
			wantErr: false,
		},
		{
			name:      "namespace config with taskRuns selector accepted",
			configKey: PrunerNamespaceConfigKey,
			config: `taskRuns:
  - selector:
      - matchAnnotations:
          team: platform
    successfulHistoryLimit: 5`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "tekton-pipelines",
				},
				Data: map[string]string{
					tt.configKey: tt.config,
				},
			}

			err := ValidateConfigMap(cm)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.wantErrMsg)
				} else if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error containing '%s', got: %s", tt.wantErrMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %s", err.Error())
				}
			}
		})
	}
}

// TestSelectorMatching_Precedence verifies selector matching precedence rules
func TestSelectorMatching_Precedence(t *testing.T) {
	ttl1800 := int32(1800)
	ttl3600 := int32(3600)

	namespaceSpec := map[string]NamespaceSpec{
		"dev": {
			PipelineRuns: []ResourceSpec{
				{
					Name: "build-pipeline",
					PrunerConfig: PrunerConfig{
						TTLSecondsAfterFinished: &ttl1800,
					},
				},
				{
					Selector: []SelectorSpec{{MatchLabels: map[string]string{"app": "myapp"}}},
					PrunerConfig: PrunerConfig{
						TTLSecondsAfterFinished: &ttl3600,
					},
				},
			},
		},
	}

	tests := []struct {
		name         string
		resourceName string
		selector     SelectorSpec
		wantTTL      int32
		wantID       string
	}{
		{
			name:         "name match takes precedence over selector",
			resourceName: "build-pipeline",
			selector:     SelectorSpec{MatchLabels: map[string]string{"app": "myapp"}},
			wantTTL:      1800,
			wantID:       "identifiedBy_resource_name",
		},
		{
			name:         "selector match when no name provided",
			resourceName: "",
			selector:     SelectorSpec{MatchLabels: map[string]string{"app": "myapp"}},
			wantTTL:      3600,
			wantID:       "identifiedBy_resource_selector",
		},
		{
			name:         "selector match when name doesn't exist",
			resourceName: "non-existent",
			selector:     SelectorSpec{MatchLabels: map[string]string{"app": "myapp"}},
			wantTTL:      3600, // Falls through to selector matching after name mismatch
			wantID:       "identifiedBy_resource_selector",
		},
		{
			name:         "no match when both name and selector don't match",
			resourceName: "non-existent",
			selector:     SelectorSpec{MatchLabels: map[string]string{"app": "different"}},
			wantTTL:      0, // Neither name nor selector match
			wantID:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, identifiedBy := getFromPrunerConfigResourceLevelwithSelector(
				namespaceSpec,
				"dev",
				tt.resourceName,
				tt.selector,
				PrunerResourceTypePipelineRun,
				PrunerFieldTypeTTLSecondsAfterFinished,
			)

			if tt.wantTTL == 0 {
				if result != nil {
					t.Errorf("expected nil result, got %v", *result)
				}
			} else {
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if *result != tt.wantTTL {
					t.Errorf("expected TTL=%d, got %d", tt.wantTTL, *result)
				}
			}

			if identifiedBy != tt.wantID {
				t.Errorf("expected identifiedBy='%s', got '%s'", tt.wantID, identifiedBy)
			}
		})
	}
}

// TestSelectorMatching_ANDLogic verifies AND logic for labels and annotations
func TestSelectorMatching_ANDLogic(t *testing.T) {
	ttl1800 := int32(1800)
	namespaceSpec := map[string]NamespaceSpec{
		"dev": {
			PipelineRuns: []ResourceSpec{
				{
					Selector: []SelectorSpec{{
						MatchLabels:      map[string]string{"app": "myapp"},
						MatchAnnotations: map[string]string{"version": "v1"},
					}},
					PrunerConfig: PrunerConfig{
						TTLSecondsAfterFinished: &ttl1800,
					},
				},
			},
		},
	}

	tests := []struct {
		name       string
		selector   SelectorSpec
		shouldFind bool
	}{
		{
			name: "both labels and annotations match",
			selector: SelectorSpec{
				MatchLabels:      map[string]string{"app": "myapp"},
				MatchAnnotations: map[string]string{"version": "v1"},
			},
			shouldFind: true,
		},
		{
			name: "only labels match",
			selector: SelectorSpec{
				MatchLabels: map[string]string{"app": "myapp"},
			},
			shouldFind: false,
		},
		{
			name: "only annotations match",
			selector: SelectorSpec{
				MatchAnnotations: map[string]string{"version": "v1"},
			},
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, identifiedBy := getFromPrunerConfigResourceLevelwithSelector(
				namespaceSpec,
				"dev",
				"",
				tt.selector,
				PrunerResourceTypePipelineRun,
				PrunerFieldTypeTTLSecondsAfterFinished,
			)

			if tt.shouldFind {
				if result == nil || *result != 1800 {
					t.Errorf("expected TTL=1800, got %v", result)
				}
				if identifiedBy != "identifiedBy_resource_selector" {
					t.Errorf("expected identifiedBy='identifiedBy_resource_selector', got '%s'", identifiedBy)
				}
			} else {
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
				if identifiedBy != "" {
					t.Errorf("expected empty identifiedBy, got '%s'", identifiedBy)
				}
			}
		})
	}
}

// TestGetResourceFieldData_ConfigLevels verifies config level precedence
func TestGetResourceFieldData_ConfigLevels(t *testing.T) {
	ttl1800 := int32(1800)
	ttl3600 := int32(3600)
	ttl7200 := int32(7200)

	globalSpec := GlobalConfig{
		PrunerConfig: PrunerConfig{TTLSecondsAfterFinished: &ttl7200},
	}

	namespaceConfigMap := map[string]NamespaceSpec{
		"dev": {
			PipelineRuns: []ResourceSpec{
				{
					Selector:     []SelectorSpec{{MatchLabels: map[string]string{"app": "myapp"}}},
					PrunerConfig: PrunerConfig{TTLSecondsAfterFinished: &ttl1800},
				},
			},
			PrunerConfig: PrunerConfig{TTLSecondsAfterFinished: &ttl3600},
		},
	}

	selector := SelectorSpec{MatchLabels: map[string]string{"app": "myapp"}}

	tests := []struct {
		name    string
		level   EnforcedConfigLevel
		wantTTL int32
		wantID  string
	}{
		{
			name:    "namespace level uses selector",
			level:   EnforcedConfigLevelNamespace,
			wantTTL: 1800,
			wantID:  "identifiedBy_resource_selector",
		},
		{
			name:    "global level ignores selector",
			level:   EnforcedConfigLevelGlobal,
			wantTTL: 7200,
			wantID:  "identified_by_global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, identifiedBy := getResourceFieldData(
				globalSpec,
				namespaceConfigMap,
				"dev",
				"",
				selector,
				PrunerResourceTypePipelineRun,
				PrunerFieldTypeTTLSecondsAfterFinished,
				tt.level,
			)

			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if *result != tt.wantTTL {
				t.Errorf("expected TTL=%d, got %d", tt.wantTTL, *result)
			}
			if identifiedBy != tt.wantID {
				t.Errorf("expected identifiedBy='%s', got '%s'", tt.wantID, identifiedBy)
			}
		})
	}
}
