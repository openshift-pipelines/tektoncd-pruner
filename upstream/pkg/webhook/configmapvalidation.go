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
	"fmt"

	"github.com/tektoncd/pruner/pkg/config"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/system"
	"knative.dev/pkg/webhook"
	certresources "knative.dev/pkg/webhook/certificates/resources"
)

type ValidateConfigMap struct {
	Client      kubernetes.Interface
	SecretName  string
	WebhookName string
}

var _ webhook.AdmissionController = (*ValidateConfigMap)(nil)
var _ webhook.StatelessAdmissionController = (*ValidateConfigMap)(nil)
var _ controller.Reconciler = (*ValidateConfigMap)(nil)

// ThisTypeDoesNotDependOnInformerState implements StatelessAdmissionController
func (v *ValidateConfigMap) ThisTypeDoesNotDependOnInformerState() {}

// validateRequiredLabels checks if the ConfigMap has the required labels for pruner configs
func validateRequiredLabels(cm *corev1.ConfigMap) error {
	if cm.Labels == nil {
		return fmt.Errorf("ConfigMap must have labels")
	}

	// Check for required label: app.kubernetes.io/part-of=tekton-pruner
	if partOf, ok := cm.Labels["app.kubernetes.io/part-of"]; !ok || partOf != "tekton-pruner" {
		return fmt.Errorf("ConfigMap must have label app.kubernetes.io/part-of=tekton-pruner")
	}

	// Check for config-type label
	configType, ok := cm.Labels["pruner.tekton.dev/config-type"]
	if !ok {
		return fmt.Errorf("ConfigMap must have label pruner.tekton.dev/config-type (global or namespace)")
	}

	// Validate config-type value
	if configType != "global" && configType != "namespace" {
		return fmt.Errorf("label pruner.tekton.dev/config-type must be 'global' or 'namespace', got: %s", configType)
	}

	return nil
}

// Reconcile updates the ValidatingWebhookConfiguration with CA bundle
func (v *ValidateConfigMap) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)

	// Get the secret containing the CA certificate
	secret, err := v.Client.CoreV1().Secrets(system.Namespace()).Get(ctx, v.SecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("Secret %s not found yet, skipping webhook configuration update", v.SecretName)
		return nil
	} else if err != nil {
		logger.Errorw("Error fetching secret", "secret", v.SecretName, "error", err)
		return err
	}

	// Get the CA certificate from the secret
	caCert, ok := secret.Data[certresources.CACert]
	if !ok {
		logger.Infof("CA cert not yet present in secret %s", v.SecretName)
		return nil
	}

	// Get the ValidatingWebhookConfiguration
	vwc, err := v.Client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, v.WebhookName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("ValidatingWebhookConfiguration %s not found yet", v.WebhookName)
		return nil
	} else if err != nil {
		logger.Errorw("Error fetching ValidatingWebhookConfiguration", "name", v.WebhookName, "error", err)
		return err
	}

	// Update the CA bundle in all webhooks
	updated := false
	for i := range vwc.Webhooks {
		if string(vwc.Webhooks[i].ClientConfig.CABundle) != string(caCert) {
			vwc.Webhooks[i].ClientConfig.CABundle = caCert
			updated = true
		}
	}

	if !updated {
		logger.Debug("CA bundle already up to date")
		return nil
	}

	// Update the webhook configuration
	_, err = v.Client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(ctx, vwc, metav1.UpdateOptions{})
	if err != nil {
		logger.Errorw("Error updating ValidatingWebhookConfiguration", "name", v.WebhookName, "error", err)
		return err
	}

	logger.Infow("Successfully updated ValidatingWebhookConfiguration with CA bundle", "name", v.WebhookName)
	return nil
}

func (v *ValidateConfigMap) Path() string {
	return "/validate-configmap"
}

// Admit handles the admission request
func (v *ValidateConfigMap) Admit(ctx context.Context, request *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	logger := logging.FromContext(ctx)

	// Only validate ConfigMaps
	if request.Kind.Kind != "ConfigMap" {
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	// Parse the ConfigMap (use OldObject for DELETE operations)
	var cm corev1.ConfigMap
	if request.Operation == admissionv1.Delete {
		if err := json.Unmarshal(request.OldObject.Raw, &cm); err != nil {
			logger.Warnw("Failed to unmarshal ConfigMap from OldObject", "error", err)
			return &admissionv1.AdmissionResponse{Allowed: true}
		}
	} else {
		if err := json.Unmarshal(request.Object.Raw, &cm); err != nil {
			logger.Warnw("Failed to unmarshal ConfigMap", "error", err)
			return &admissionv1.AdmissionResponse{Allowed: true}
		}
	}

	// Validate that ConfigMap has required labels
	// The webhook objectSelector ensures only ConfigMaps with proper labels reach this point
	// This is a defense-in-depth check
	if err := validateRequiredLabels(&cm); err != nil {
		logger.Warnw("ConfigMap missing required labels", "name", cm.Name, "namespace", cm.Namespace, "error", err)
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Invalid pruner ConfigMap labels: %v", err),
				Reason:  metav1.StatusReasonInvalid,
				Code:    400,
			},
		}
	}

	// Determine config type from labels
	configType := cm.Labels["pruner.tekton.dev/config-type"]
	isGlobalConfig := configType == "global" && cm.Namespace == system.Namespace()
	isNamespaceConfig := configType == "namespace" && cm.Namespace != system.Namespace()

	// Validate ConfigMap names match expected patterns
	if isGlobalConfig && cm.Name != "tekton-pruner-default-spec" {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Global config must be named 'tekton-pruner-default-spec', got: %s", cm.Name),
				Reason:  metav1.StatusReasonInvalid,
				Code:    400,
			},
		}
	}

	if isNamespaceConfig && cm.Name != "tekton-pruner-namespace-spec" {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Namespace config must be named 'tekton-pruner-namespace-spec', got: %s", cm.Name),
				Reason:  metav1.StatusReasonInvalid,
				Code:    400,
			},
		}
	}

	if !isGlobalConfig && !isNamespaceConfig {
		logger.Warnw("Received unexpected ConfigMap", "name", cm.Name, "namespace", cm.Namespace, "configType", configType)
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Invalid pruner ConfigMap configuration: wrong config-type label or namespace combination"),
				Reason:  metav1.StatusReasonInvalid,
				Code:    400,
			},
		}
	}

	logger.Infow("Validating pruner ConfigMap",
		"name", cm.Name,
		"namespace", cm.Namespace,
		"operation", request.Operation,
		"isGlobalConfig", isGlobalConfig,
		"isNamespaceConfig", isNamespaceConfig)

	// Handle DELETE operations
	if request.Operation == admissionv1.Delete {
		// Prevent deletion of global config if namespace configs still exist
		if isGlobalConfig {
			nsList, err := v.Client.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
				FieldSelector: "metadata.name=tekton-pruner-namespace-spec",
			})
			if err != nil {
				logger.Errorw("Failed to check for existing namespace configs", "error", err)
				// Allow deletion if we can't check (fail open for DELETE)
				logger.Infow("Allowing deletion of global config (unable to verify dependents)", "name", cm.Name)
				return &admissionv1.AdmissionResponse{Allowed: true}
			}
			if len(nsList.Items) > 0 {
				namespaces := make([]string, len(nsList.Items))
				for i, item := range nsList.Items {
					namespaces[i] = item.Namespace
				}
				logger.Errorw("Attempted to delete global config while namespace configs exist",
					"namespaceConfigCount", len(nsList.Items),
					"affectedNamespaces", namespaces)
				return &admissionv1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Status: metav1.StatusFailure,
						Message: fmt.Sprintf("Cannot delete global config: %d namespace config(s) still exist that depend on it in namespaces: %v. Delete namespace configs first.",
							len(nsList.Items), namespaces),
						Reason: metav1.StatusReasonInvalid,
						Code:   422,
					},
				}
			}
			logger.Infow("Allowing deletion of global config (no dependents)", "name", cm.Name)
		} else if isNamespaceConfig {
			logger.Infow("Allowing deletion of namespace config", "name", cm.Name, "namespace", cm.Namespace)
		}
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	// For CREATE/UPDATE operations, perform validation
	// For namespace-level configs, fetch global config to enforce limits
	var globalConfig *corev1.ConfigMap
	if isNamespaceConfig {
		var err error
		globalConfig, err = v.Client.CoreV1().ConfigMaps(system.Namespace()).Get(ctx, "tekton-pruner-default-spec", metav1.GetOptions{})
		if err != nil {
			logger.Warnw("Failed to fetch global config for namespace validation", "error", err)
			// Allow if global config is not available (e.g., during initial setup)
			// Basic validation will still be performed
		}
	}

	// Validate using the centralized validation function
	if err := config.ValidateConfigMapWithGlobal(&cm, globalConfig); err != nil {
		logger.Errorw("ConfigMap validation failed", "name", cm.Name, "namespace", cm.Namespace, "error", err)
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Invalid pruner configuration: %v", err),
				Reason:  metav1.StatusReasonInvalid,
				Code:    422,
			},
		}
	}

	logger.Infow("ConfigMap validation successful", "name", cm.Name, "namespace", cm.Namespace)
	return &admissionv1.AdmissionResponse{Allowed: true}
}
