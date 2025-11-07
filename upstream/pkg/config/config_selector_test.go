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

// TestValidateConfigMap_GlobalWithSelectorsRejected verifies that global ConfigMaps
// with selectors in namespace sections are REJECTED by validation
func TestValidateConfigMap_GlobalWithSelectorsRejected(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantErrMsg string
	}{
		{
			name: "global config with pipelineRuns selector should fail",
			config: `ttlSecondsAfterFinished: 3600
namespaces:
  dev:
    pipelineRuns:
      - selector:
          - matchLabels:
              app: myapp
        ttlSecondsAfterFinished: 1800`,
			wantErrMsg: "global-config.namespaces.dev.pipelineRuns[0]: selectors are NOT supported in global ConfigMap",
		},
		{
			name: "global config with taskRuns selector should fail",
			config: `ttlSecondsAfterFinished: 3600
namespaces:
  prod:
    taskRuns:
      - selector:
          - matchAnnotations:
              team: platform
        successfulHistoryLimit: 5`,
			wantErrMsg: "global-config.namespaces.prod.taskRuns[0]: selectors are NOT supported in global ConfigMap",
		},
		{
			name: "global config with multiple selectors should fail on first",
			config: `namespaces:
  dev:
    pipelineRuns:
      - selector:
          - matchLabels:
              app: myapp
      - selector:
          - matchLabels:
              app: anotherapp`,
			wantErrMsg: "global-config.namespaces.dev.pipelineRuns[0]: selectors are NOT supported in global ConfigMap",
		},
		{
			name: "global config with mixed label and annotation selectors should fail",
			config: `namespaces:
  staging:
    pipelineRuns:
      - selector:
          - matchLabels:
              app: myapp
            matchAnnotations:
              version: v1`,
			wantErrMsg: "global-config.namespaces.staging.pipelineRuns[0]: selectors are NOT supported in global ConfigMap",
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

// TestValidateConfigMap_NamespaceWithSelectorsAccepted verifies that namespace ConfigMaps
// with selectors ARE accepted by validation
func TestValidateConfigMap_NamespaceWithSelectorsAccepted(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name: "namespace config with pipelineRuns selector should pass",
			config: `pipelineRuns:
  - selector:
      - matchLabels:
          app: myapp
    ttlSecondsAfterFinished: 1800`,
		},
		{
			name: "namespace config with taskRuns selector should pass",
			config: `taskRuns:
  - selector:
      - matchAnnotations:
          team: platform
    successfulHistoryLimit: 5`,
		},
		{
			name: "namespace config with mixed selectors should pass",
			config: `pipelineRuns:
  - selector:
      - matchLabels:
          app: myapp
        matchAnnotations:
          version: v1
    ttlSecondsAfterFinished: 600`,
		},
		{
			name: "namespace config with multiple resource specs should pass",
			config: `pipelineRuns:
  - selector:
      - matchLabels:
          app: app1
    ttlSecondsAfterFinished: 1800
  - selector:
      - matchLabels:
          app: app2
    ttlSecondsAfterFinished: 3600`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: "dev",
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

// TestGetFromPrunerConfigResourceLevelwithSelector_NamePrecedence verifies that
// name-based matching has absolute precedence over selector-based matching
func TestGetFromPrunerConfigResourceLevelwithSelector_NamePrecedence(t *testing.T) {
	ttl1800 := int32(1800)
	ttl3600 := int32(3600)

	namespaceSpec := map[string]NamespaceSpec{
		"dev": {
			PipelineRuns: []ResourceSpec{
				{
					Name: "build-pipeline",
					PrunerConfig: PrunerConfig{
						TTLSecondsAfterFinished: &ttl1800, // Should match by name
					},
				},
				{
					Selector: []SelectorSpec{
						{
							MatchLabels: map[string]string{"app": "myapp"},
						},
					},
					PrunerConfig: PrunerConfig{
						TTLSecondsAfterFinished: &ttl3600, // Should NOT match even if labels match
					},
				},
			},
		},
	}

	selector := SelectorSpec{
		MatchLabels: map[string]string{"app": "myapp"}, // This matches second spec
	}

	// Name should take precedence
	result, identifiedBy := getFromPrunerConfigResourceLevelwithSelector(
		namespaceSpec,
		"dev",
		"build-pipeline", // Name provided
		selector,         // Selector also matches, but name should win
		PrunerResourceTypePipelineRun,
		PrunerFieldTypeTTLSecondsAfterFinished,
	)

	if result == nil {
		t.Fatal("Expected non-nil result for name match")
	}
	if *result != 1800 {
		t.Errorf("Expected TTL=1800 from name match, got %d", *result)
	}
	if identifiedBy != "identifiedBy_resource_name" {
		t.Errorf("Expected identifiedBy='identifiedBy_resource_name', got '%s'", identifiedBy)
	}
}

// TestGetFromPrunerConfigResourceLevelwithSelector_ANDLogic verifies that
// both labels AND annotations must match when both are specified
func TestGetFromPrunerConfigResourceLevelwithSelector_ANDLogic(t *testing.T) {
	ttl1800 := int32(1800)

	namespaceSpec := map[string]NamespaceSpec{
		"dev": {
			PipelineRuns: []ResourceSpec{
				{
					Selector: []SelectorSpec{
						{
							MatchLabels:      map[string]string{"app": "myapp"},
							MatchAnnotations: map[string]string{"version": "v1"},
						},
					},
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
			name: "both labels and annotations match - should find",
			selector: SelectorSpec{
				MatchLabels:      map[string]string{"app": "myapp"},
				MatchAnnotations: map[string]string{"version": "v1"},
			},
			shouldFind: true,
		},
		{
			name: "only labels match - should NOT find",
			selector: SelectorSpec{
				MatchLabels: map[string]string{"app": "myapp"},
			},
			shouldFind: false,
		},
		{
			name: "only annotations match - should NOT find",
			selector: SelectorSpec{
				MatchAnnotations: map[string]string{"version": "v1"},
			},
			shouldFind: false,
		},
		{
			name: "labels match but annotations don't - should NOT find",
			selector: SelectorSpec{
				MatchLabels:      map[string]string{"app": "myapp"},
				MatchAnnotations: map[string]string{"version": "v2"},
			},
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, identifiedBy := getFromPrunerConfigResourceLevelwithSelector(
				namespaceSpec,
				"dev",
				"", // No name, use selector
				tt.selector,
				PrunerResourceTypePipelineRun,
				PrunerFieldTypeTTLSecondsAfterFinished,
			)

			if tt.shouldFind {
				if result == nil {
					t.Error("Expected to find result, but got nil")
				} else if *result != 1800 {
					t.Errorf("Expected TTL=1800, got %d", *result)
				}
				if identifiedBy != "identifiedBy_resource_selector" {
					t.Errorf("Expected identifiedBy='identifiedBy_resource_selector', got '%s'", identifiedBy)
				}
			} else {
				if result != nil {
					t.Errorf("Expected nil result, but got %v", *result)
				}
				if identifiedBy != "" {
					t.Errorf("Expected empty identifiedBy, got '%s'", identifiedBy)
				}
			}
		})
	}
}

// TestGetFromPrunerConfigResourceLevelwithSelector_NoMatchReturnsNil verifies that
// when name is provided but doesn't match, nil is returned (no fallback to selectors)
func TestGetFromPrunerConfigResourceLevelwithSelector_NoMatchReturnsNil(t *testing.T) {
	ttl1800 := int32(1800)

	namespaceSpec := map[string]NamespaceSpec{
		"dev": {
			PipelineRuns: []ResourceSpec{
				{
					Selector: []SelectorSpec{
						{
							MatchLabels: map[string]string{"app": "myapp"},
						},
					},
					PrunerConfig: PrunerConfig{
						TTLSecondsAfterFinished: &ttl1800,
					},
				},
			},
		},
	}

	selector := SelectorSpec{
		MatchLabels: map[string]string{"app": "myapp"},
	}

	// Name provided but doesn't match - should return nil (no fallback to selector)
	result, identifiedBy := getFromPrunerConfigResourceLevelwithSelector(
		namespaceSpec,
		"dev",
		"non-existent-pipeline", // Name doesn't match any resource
		selector,                // Selector would match, but should be ignored
		PrunerResourceTypePipelineRun,
		PrunerFieldTypeTTLSecondsAfterFinished,
	)

	if result != nil {
		t.Errorf("Expected nil result for non-matching name, got %v", *result)
	}
	if identifiedBy != "" {
		t.Errorf("Expected empty identifiedBy, got '%s'", identifiedBy)
	}
}

// TestGetResourceFieldData_NamespaceConfigMapSelectors verifies that
// EnforcedConfigLevelNamespace checks namespace ConfigMap selectors first
func TestGetResourceFieldData_NamespaceConfigMapSelectors(t *testing.T) {
	ttl1800 := int32(1800)
	ttl3600 := int32(3600)
	ttl7200 := int32(7200)

	globalSpec := GlobalConfig{
		PrunerConfig: PrunerConfig{
			TTLSecondsAfterFinished: &ttl7200, // Global default
		},
	}

	namespaceConfigMap := map[string]NamespaceSpec{
		"dev": {
			PipelineRuns: []ResourceSpec{
				{
					Selector: []SelectorSpec{
						{
							MatchLabels: map[string]string{"app": "myapp"},
						},
					},
					PrunerConfig: PrunerConfig{
						TTLSecondsAfterFinished: &ttl1800, // Namespace ConfigMap selector
					},
				},
			},
			PrunerConfig: PrunerConfig{
				TTLSecondsAfterFinished: &ttl3600, // Namespace ConfigMap root
			},
		},
	}

	selector := SelectorSpec{
		MatchLabels: map[string]string{"app": "myapp"},
	}

	// Should find selector match from namespace ConfigMap (ttl1800)
	result, identifiedBy := getResourceFieldData(
		globalSpec,
		namespaceConfigMap,
		"dev",
		"", // No name
		selector,
		PrunerResourceTypePipelineRun,
		PrunerFieldTypeTTLSecondsAfterFinished,
		EnforcedConfigLevelNamespace,
	)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if *result != 1800 {
		t.Errorf("Expected TTL=1800 from namespace ConfigMap selector, got %d", *result)
	}
	if identifiedBy != "identifiedBy_resource_selector" {
		t.Errorf("Expected identifiedBy='identifiedBy_resource_selector', got '%s'", identifiedBy)
	}
}

// TestGetResourceFieldData_GlobalConfigMapNoSelectors verifies that
// EnforcedConfigLevelGlobal never uses selectors, only root-level fields
func TestGetResourceFieldData_GlobalConfigMapNoSelectors(t *testing.T) {
	ttl7200 := int32(7200)

	globalSpec := GlobalConfig{
		PrunerConfig: PrunerConfig{
			TTLSecondsAfterFinished: &ttl7200, // Global default - should be used
		},
		Namespaces: map[string]NamespaceSpec{
			"dev": {
				// Even if there are selectors here, they should be ignored for EnforcedConfigLevelGlobal
				PipelineRuns: []ResourceSpec{
					{
						Selector: []SelectorSpec{
							{
								MatchLabels: map[string]string{"app": "myapp"},
							},
						},
						PrunerConfig: PrunerConfig{
							TTLSecondsAfterFinished: int32Ptr(1800), // Should be ignored
						},
					},
				},
			},
		},
	}

	selector := SelectorSpec{
		MatchLabels: map[string]string{"app": "myapp"},
	}

	// EnforcedConfigLevelGlobal should use only global root, ignore all selectors
	result, identifiedBy := getResourceFieldData(
		globalSpec,
		map[string]NamespaceSpec{},
		"dev",
		"",
		selector,
		PrunerResourceTypePipelineRun,
		PrunerFieldTypeTTLSecondsAfterFinished,
		EnforcedConfigLevelGlobal,
	)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if *result != 7200 {
		t.Errorf("Expected TTL=7200 from global root (ignoring selectors), got %d", *result)
	}
	if identifiedBy != "identified_by_global" {
		t.Errorf("Expected identifiedBy='identified_by_global', got '%s'", identifiedBy)
	}
}
