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

package webhook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tektoncd/pruner/pkg/config"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
	"knative.dev/pkg/system"

	// Required for system.Namespace() in tests
	_ "knative.dev/pkg/system/testing"
)

func TestValidateConfigMap_Admit_GlobalConfig(t *testing.T) {
	tests := []struct {
		name        string
		configData  string
		wantAllowed bool
		wantMessage string
	}{
		{
			name: "valid global config",
			configData: `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10
failedHistoryLimit: 10`,
			wantAllowed: true,
		},
		{
			name:        "invalid global config - negative TTL",
			configData:  `ttlSecondsAfterFinished: -1`,
			wantAllowed: false,
			wantMessage: "ttlSecondsAfterFinished cannot be negative",
		},
		{
			name:        "invalid global config - negative history limit",
			configData:  `successfulHistoryLimit: -5`,
			wantAllowed: false,
			wantMessage: "successfulHistoryLimit cannot be negative",
		},
		{
			name:        "invalid global config - bad enum",
			configData:  `enforcedConfigLevel: invalid`,
			wantAllowed: false,
			wantMessage: "invalid enforcedConfigLevel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: system.Namespace(),
					Labels: map[string]string{
						"app.kubernetes.io/part-of":     "tekton-pruner",
						"pruner.tekton.dev/config-type": "global",
					},
				},
				Data: map[string]string{
					config.PrunerGlobalConfigKey: tt.configData,
				},
			}

			req := makeAdmissionRequest(t, cm, admissionv1.Create)
			ctx := logtesting.TestContextWithLogger(t)

			client := fake.NewSimpleClientset()
			validator := &ValidateConfigMap{
				Client:      client,
				SecretName:  "test-secret",
				WebhookName: "test-webhook",
			}

			resp := validator.Admit(ctx, req)

			if resp.Allowed != tt.wantAllowed {
				t.Errorf("Admit() allowed = %v, want %v", resp.Allowed, tt.wantAllowed)
			}

			if !tt.wantAllowed && resp.Result != nil {
				if !contains(resp.Result.Message, tt.wantMessage) {
					t.Errorf("Admit() message = %v, want to contain %v", resp.Result.Message, tt.wantMessage)
				}
			}
		})
	}
}

func TestValidateConfigMap_Admit_NamespaceConfig(t *testing.T) {
	tests := []struct {
		name         string
		namespace    string
		configData   string
		globalConfig *corev1.ConfigMap
		wantAllowed  bool
		wantMessage  string
	}{
		{
			name:      "valid namespace config without global",
			namespace: "my-app",
			configData: `ttlSecondsAfterFinished: 1800
successfulHistoryLimit: 5`,
			wantAllowed: true,
		},
		{
			name:        "invalid namespace config - negative value",
			namespace:   "my-app",
			configData:  `ttlSecondsAfterFinished: -100`,
			wantAllowed: false,
			wantMessage: "ttlSecondsAfterFinished cannot be negative",
		},
		{
			name:      "namespace config within global limits",
			namespace: "my-app",
			configData: `ttlSecondsAfterFinished: 1800
successfulHistoryLimit: 5`,
			globalConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: system.Namespace(),
					Labels: map[string]string{
						"app.kubernetes.io/part-of":     "tekton-pruner",
						"pruner.tekton.dev/config-type": "global",
					},
				},
				Data: map[string]string{
					config.PrunerGlobalConfigKey: `ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10`,
				},
			},
			wantAllowed: true,
		},
		{
			name:       "namespace config exceeds global TTL",
			namespace:  "my-app",
			configData: `ttlSecondsAfterFinished: 7200`,
			globalConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: system.Namespace(),
					Labels: map[string]string{
						"app.kubernetes.io/part-of":     "tekton-pruner",
						"pruner.tekton.dev/config-type": "global",
					},
				},
				Data: map[string]string{
					config.PrunerGlobalConfigKey: `ttlSecondsAfterFinished: 3600`,
				},
			},
			wantAllowed: false,
			wantMessage: "cannot exceed global limit",
		},
		{
			name:       "namespace config exceeds global history limit",
			namespace:  "my-app",
			configData: `successfulHistoryLimit: 20`,
			globalConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-default-spec",
					Namespace: system.Namespace(),
					Labels: map[string]string{
						"app.kubernetes.io/part-of":     "tekton-pruner",
						"pruner.tekton.dev/config-type": "global",
					},
				},
				Data: map[string]string{
					config.PrunerGlobalConfigKey: `successfulHistoryLimit: 10`,
				},
			},
			wantAllowed: false,
			wantMessage: "cannot exceed global limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: tt.namespace,
					Labels: map[string]string{
						"app.kubernetes.io/part-of":     "tekton-pruner",
						"pruner.tekton.dev/config-type": "namespace",
					},
				},
				Data: map[string]string{
					config.PrunerNamespaceConfigKey: tt.configData,
				},
			}

			req := makeAdmissionRequest(t, cm, admissionv1.Create)
			ctx := logtesting.TestContextWithLogger(t)

			// Create fake client with or without global config
			// Note: system.Namespace() in tests returns "knative-testing" by default from testing package
			// So we need to create the global config in that namespace
			var client *fake.Clientset
			if tt.globalConfig != nil {
				// Update the global config to be in the system namespace for tests
				testGlobalConfig := tt.globalConfig.DeepCopy()
				testGlobalConfig.Namespace = "knative-testing"
				client = fake.NewSimpleClientset(testGlobalConfig)
			} else {
				client = fake.NewSimpleClientset()
			}

			validator := &ValidateConfigMap{
				Client:      client,
				SecretName:  "test-secret",
				WebhookName: "test-webhook",
			}

			resp := validator.Admit(ctx, req)

			if resp.Allowed != tt.wantAllowed {
				t.Errorf("Admit() allowed = %v, want %v", resp.Allowed, tt.wantAllowed)
			}

			if !tt.wantAllowed && resp.Result != nil {
				if !contains(resp.Result.Message, tt.wantMessage) {
					t.Errorf("Admit() message = %v, want to contain %v", resp.Result.Message, tt.wantMessage)
				}
			}
		})
	}
}

func TestValidateConfigMap_Admit_NonPrunerConfigMap(t *testing.T) {
	tests := []struct {
		name      string
		cmName    string
		namespace string
	}{
		{
			name:      "different configmap name in system namespace",
			cmName:    "other-config",
			namespace: system.Namespace(),
		},
		{
			name:      "different configmap name in user namespace",
			cmName:    "my-config",
			namespace: "my-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.cmName,
					Namespace: tt.namespace,
				},
				Data: map[string]string{
					"some-key": "some-value",
				},
			}

			req := makeAdmissionRequest(t, cm, admissionv1.Create)
			ctx := logtesting.TestContextWithLogger(t)

			client := fake.NewSimpleClientset()
			validator := &ValidateConfigMap{
				Client:      client,
				SecretName:  "test-secret",
				WebhookName: "test-webhook",
			}

			resp := validator.Admit(ctx, req)

			// With label-based filtering via objectSelector, ConfigMaps without proper labels
			// should be rejected as they shouldn't reach the webhook in the first place
			if resp.Allowed {
				t.Errorf("Admit() should reject ConfigMaps without required labels, got allowed = %v", resp.Allowed)
			}
			if resp.Result != nil && resp.Result.Message != "" {
				t.Logf("Rejection message: %s", resp.Result.Message)
			}
		})
	}
}

func TestValidateConfigMap_Admit_NonConfigMapResource(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	secretBytes, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("Failed to marshal secret: %v", err)
	}

	req := &admissionv1.AdmissionRequest{
		UID: "test-uid",
		Kind: metav1.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
		Operation: admissionv1.Create,
		Object: runtime.RawExtension{
			Raw: secretBytes,
		},
	}

	ctx := logtesting.TestContextWithLogger(t)
	client := fake.NewSimpleClientset()
	validator := &ValidateConfigMap{
		Client:      client,
		SecretName:  "test-secret",
		WebhookName: "test-webhook",
	}

	resp := validator.Admit(ctx, req)

	if !resp.Allowed {
		t.Errorf("Admit() should allow non-ConfigMap resources, got allowed = %v", resp.Allowed)
	}
}

func TestValidateConfigMap_Admit_ForbiddenNamespaces(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		wantAllowed bool
		wantMessage string
	}{
		{
			name:        "kube-system namespace - forbidden",
			namespace:   "kube-system",
			wantAllowed: false,
			wantMessage: "kube-* namespaces",
		},
		{
			name:        "kube-public namespace - forbidden",
			namespace:   "kube-public",
			wantAllowed: false,
			wantMessage: "kube-* namespaces",
		},
		{
			name:        "kube-custom namespace - forbidden",
			namespace:   "kube-custom",
			wantAllowed: false,
			wantMessage: "kube-* namespaces",
		},
		{
			name:        "openshift-pipelines namespace - forbidden",
			namespace:   "openshift-pipelines",
			wantAllowed: false,
			wantMessage: "openshift-* namespaces",
		},
		{
			name:        "openshift-operators namespace - forbidden",
			namespace:   "openshift-operators",
			wantAllowed: false,
			wantMessage: "openshift-* namespaces",
		},
		{
			name:        "openshift-custom namespace - forbidden",
			namespace:   "openshift-custom",
			wantAllowed: false,
			wantMessage: "openshift-* namespaces",
		},
		{
			name:        "tekton-pipelines namespace - forbidden",
			namespace:   "tekton-pipelines",
			wantAllowed: false,
			wantMessage: "tekton-pipelines namespace",
		},
		{
			name:        "tekton-operator namespace - forbidden",
			namespace:   "tekton-operator",
			wantAllowed: false,
			wantMessage: "tekton-operator namespace",
		},
		{
			name:        "tekton-custom namespace - allowed",
			namespace:   "tekton-custom",
			wantAllowed: true,
		},
		{
			name:        "user namespace - allowed",
			namespace:   "my-app",
			wantAllowed: true,
		},
		{
			name:        "user namespace with similar name - allowed",
			namespace:   "my-kube-app",
			wantAllowed: true,
		},
		{
			name:        "user namespace with tekton suffix - allowed",
			namespace:   "my-tekton-app",
			wantAllowed: true,
		},
		{
			name:        "default namespace - allowed (not in forbidden patterns)",
			namespace:   "default",
			wantAllowed: true,
		},
		{
			name:        "dev namespace - allowed",
			namespace:   "dev",
			wantAllowed: true,
		},
		{
			name:        "production namespace - allowed",
			namespace:   "production",
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tekton-pruner-namespace-spec",
					Namespace: tt.namespace,
					Labels: map[string]string{
						"app.kubernetes.io/part-of":     "tekton-pruner",
						"pruner.tekton.dev/config-type": "namespace",
					},
				},
				Data: map[string]string{
					config.PrunerNamespaceConfigKey: `ttlSecondsAfterFinished: 3600`,
				},
			}

			req := makeAdmissionRequest(t, cm, admissionv1.Create)
			ctx := logtesting.TestContextWithLogger(t)

			client := fake.NewSimpleClientset()
			validator := &ValidateConfigMap{
				Client:      client,
				SecretName:  "test-secret",
				WebhookName: "test-webhook",
			}

			resp := validator.Admit(ctx, req)

			if resp.Allowed != tt.wantAllowed {
				t.Errorf("Admit() allowed = %v, want %v", resp.Allowed, tt.wantAllowed)
			}

			if !tt.wantAllowed && resp.Result != nil {
				if !contains(resp.Result.Message, tt.wantMessage) {
					t.Errorf("Admit() message = %v, want to contain %v", resp.Result.Message, tt.wantMessage)
				}
			}
		})
	}
}

func TestValidateConfigMap_Admit_UpdateOperation(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tekton-pruner-default-spec",
			Namespace: system.Namespace(),
			Labels: map[string]string{
				"app.kubernetes.io/part-of":     "tekton-pruner",
				"pruner.tekton.dev/config-type": "global",
			},
		},
		Data: map[string]string{
			config.PrunerGlobalConfigKey: `ttlSecondsAfterFinished: 3600`,
		},
	}

	req := makeAdmissionRequest(t, cm, admissionv1.Update)
	ctx := logtesting.TestContextWithLogger(t)

	client := fake.NewSimpleClientset()
	validator := &ValidateConfigMap{
		Client:      client,
		SecretName:  "test-secret",
		WebhookName: "test-webhook",
	}

	resp := validator.Admit(ctx, req)

	if !resp.Allowed {
		t.Errorf("Admit() should allow valid UPDATE operations, got allowed = %v", resp.Allowed)
	}
}

func TestValidateConfigMap_Admit_InvalidJSONParsing(t *testing.T) {
	req := &admissionv1.AdmissionRequest{
		UID: "test-uid",
		Kind: metav1.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "ConfigMap",
		},
		Operation: admissionv1.Create,
		Object: runtime.RawExtension{
			Raw: []byte("invalid json"),
		},
	}

	ctx := logtesting.TestContextWithLogger(t)
	client := fake.NewSimpleClientset()
	validator := &ValidateConfigMap{
		Client:      client,
		SecretName:  "test-secret",
		WebhookName: "test-webhook",
	}

	resp := validator.Admit(ctx, req)

	// Should allow on parse errors (defensive)
	if !resp.Allowed {
		t.Errorf("Admit() should allow on parse errors, got allowed = %v", resp.Allowed)
	}
}

func TestValidateConfigMap_Admit_SystemMaximumEnforcement(t *testing.T) {
	tests := []struct {
		name        string
		configData  string
		configType  string
		wantAllowed bool
		wantMessage string
	}{
		{
			name:        "global config can exceed system maximum TTL",
			configType:  "global",
			configData:  `ttlSecondsAfterFinished: 2592001`,
			wantAllowed: true,
		},
		{
			name:        "global config at system maximum TTL",
			configType:  "global",
			configData:  `ttlSecondsAfterFinished: 2592000`,
			wantAllowed: true,
		},
		{
			name:        "global config can exceed system maximum successfulHistoryLimit",
			configType:  "global",
			configData:  `successfulHistoryLimit: 101`,
			wantAllowed: true,
		},
		{
			name:        "global config at system maximum successfulHistoryLimit",
			configType:  "global",
			configData:  `successfulHistoryLimit: 100`,
			wantAllowed: true,
		},
		{
			name:        "namespace config exceeds system maximum without global",
			configType:  "namespace",
			configData:  `ttlSecondsAfterFinished: 3000000`,
			wantAllowed: false,
			wantMessage: "cannot exceed system maximum",
		},
		{
			name:        "namespace config within system maximum",
			configType:  "namespace",
			configData:  `ttlSecondsAfterFinished: 2592000`,
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cm *corev1.ConfigMap
			if tt.configType == "global" {
				cm = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.PrunerConfigMapName,
						Namespace: system.Namespace(),
						Labels: map[string]string{
							"app.kubernetes.io/part-of":     "tekton-pruner",
							"pruner.tekton.dev/config-type": "global",
						},
					},
					Data: map[string]string{
						config.PrunerGlobalConfigKey: tt.configData,
					},
				}
			} else {
				cm = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.PrunerNamespaceConfigMapName,
						Namespace: "test-namespace",
						Labels: map[string]string{
							"app.kubernetes.io/part-of":     "tekton-pruner",
							"pruner.tekton.dev/config-type": "namespace",
						},
					},
					Data: map[string]string{
						config.PrunerNamespaceConfigKey: tt.configData,
					},
				}
			}

			req := makeAdmissionRequest(t, cm, admissionv1.Create)
			ctx := logtesting.TestContextWithLogger(t)

			client := fake.NewSimpleClientset()
			validator := &ValidateConfigMap{
				Client:      client,
				SecretName:  "test-secret",
				WebhookName: "test-webhook",
			}

			resp := validator.Admit(ctx, req)

			if resp.Allowed != tt.wantAllowed {
				t.Errorf("Admit() allowed = %v, want %v", resp.Allowed, tt.wantAllowed)
			}

			if !tt.wantAllowed && resp.Result != nil && tt.wantMessage != "" {
				if !contains(resp.Result.Message, tt.wantMessage) {
					t.Errorf("Admit() error message = %v, want to contain %v", resp.Result.Message, tt.wantMessage)
				}
			}
		})
	}
}

func TestValidateConfigMap_Path_WithClient(t *testing.T) {
	validator := &ValidateConfigMap{
		Client:      fake.NewSimpleClientset(),
		SecretName:  "test-secret",
		WebhookName: "test-webhook",
	}

	path := validator.Path()
	expectedPath := "/validate-configmap"

	if path != expectedPath {
		t.Errorf("Path() = %v, want %v", path, expectedPath)
	}
}

func TestValidateConfigMap_Reconcile(t *testing.T) {
	// Test that reconcile updates the webhook configuration with CA bundle
	// This is a basic test - full reconcile testing would require more setup
	ctx := context.Background()
	ctx = logging.WithLogger(ctx, logtesting.TestLogger(t))

	client := fake.NewSimpleClientset()
	validator := &ValidateConfigMap{
		Client:      client,
		SecretName:  "test-secret",
		WebhookName: "test-webhook",
	}

	// Should not error when secret doesn't exist yet
	err := validator.Reconcile(ctx, "test-key")
	if err != nil {
		t.Errorf("Reconcile() unexpected error = %v", err)
	}
}

// Helper functions

func makeAdmissionRequest(t *testing.T, obj runtime.Object, operation admissionv1.Operation) *admissionv1.AdmissionRequest {
	objBytes, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("Failed to marshal object: %v", err)
	}

	return &admissionv1.AdmissionRequest{
		UID: "test-uid",
		Kind: metav1.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "ConfigMap",
		},
		Operation: operation,
		Object: runtime.RawExtension{
			Raw: objBytes,
		},
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestValidateNamespaceForConfig_AdditionalCases(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "allowed user namespace",
			namespace: "my-app",
			wantErr:   false,
		},
		{
			name:      "allowed dev namespace",
			namespace: "dev",
			wantErr:   false,
		},
		{
			name:      "allowed production namespace",
			namespace: "production",
			wantErr:   false,
		},
		{
			name:      "allowed namespace with kube in middle",
			namespace: "my-kube-app",
			wantErr:   false,
		},
		{
			name:      "allowed namespace with tekton in middle",
			namespace: "my-tekton-app",
			wantErr:   false,
		},
		{
			name:      "allowed default namespace",
			namespace: "default",
			wantErr:   false,
		},
		{
			name:      "forbidden kube-system",
			namespace: "kube-system",
			wantErr:   true,
			errMsg:    "kube-* namespaces",
		},
		{
			name:      "forbidden kube-public",
			namespace: "kube-public",
			wantErr:   true,
			errMsg:    "kube-* namespaces",
		},
		{
			name:      "forbidden kube-node-lease",
			namespace: "kube-node-lease",
			wantErr:   true,
			errMsg:    "kube-* namespaces",
		},
		{
			name:      "forbidden kube-custom",
			namespace: "kube-custom",
			wantErr:   true,
			errMsg:    "kube-* namespaces",
		},
		{
			name:      "forbidden openshift-pipelines",
			namespace: "openshift-pipelines",
			wantErr:   true,
			errMsg:    "openshift-* namespaces",
		},
		{
			name:      "forbidden openshift-operators",
			namespace: "openshift-operators",
			wantErr:   true,
			errMsg:    "openshift-* namespaces",
		},
		{
			name:      "forbidden openshift-custom",
			namespace: "openshift-custom",
			wantErr:   true,
			errMsg:    "openshift-* namespaces",
		},
		{
			name:      "forbidden tekton-pipelines",
			namespace: "tekton-pipelines",
			wantErr:   true,
			errMsg:    "tekton-pipelines namespace",
		},
		{
			name:      "forbidden tekton-operator",
			namespace: "tekton-operator",
			wantErr:   true,
			errMsg:    "tekton-operator namespace",
		},
		{
			name:      "allowed tekton-custom",
			namespace: "tekton-custom",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNamespaceForConfig(tt.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNamespaceForConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && !contains(err.Error(), tt.errMsg) {
				t.Errorf("validateNamespaceForConfig() error = %v, want to contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}
