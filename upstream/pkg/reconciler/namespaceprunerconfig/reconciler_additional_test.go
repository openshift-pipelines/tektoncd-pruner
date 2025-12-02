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

package namespaceprunerconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tektoncd/pruner/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
)

// TestReconcileValidConfigs verifies reconciliation with various valid configurations.
func TestReconcileValidConfigs(t *testing.T) {
	tests := []struct {
		name       string
		configData string
	}{
		{
			name: "TTL config",
			configData: `
ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 10`,
		},
		{
			name: "PipelineRun selectors",
			configData: `
pipelineRuns:
  - name: my-pipeline
    ttlSecondsAfterFinished: 7200`,
		},
		{
			name: "TaskRun selectors",
			configData: `
taskRuns:
  - selector:
      - matchLabels:
          app: myapp
    successfulHistoryLimit: 5`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := logging.WithLogger(context.Background(), logtesting.TestLogger(t))
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.PrunerNamespaceConfigMapName,
					Namespace: "test-ns",
				},
				Data: map[string]string{config.PrunerNamespaceConfigKey: tt.configData},
			}

			reconciler := &Reconciler{kubeclient: fake.NewSimpleClientset(cm)}
			err := reconciler.Reconcile(ctx, "test-ns/"+config.PrunerNamespaceConfigMapName)

			assert.NoError(t, err)
		})
	}
}

// TestReconcileInvalidConfig verifies error handling for invalid YAML.
func TestReconcileInvalidConfig(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logtesting.TestLogger(t))
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PrunerNamespaceConfigMapName,
			Namespace: "test-ns",
		},
		Data: map[string]string{config.PrunerNamespaceConfigKey: "invalid: yaml: ::::"},
	}

	reconciler := &Reconciler{kubeclient: fake.NewSimpleClientset(cm)}
	err := reconciler.Reconcile(ctx, "test-ns/"+config.PrunerNamespaceConfigMapName)

	assert.Error(t, err)
}

// TestReconcileConfigMapNotFound verifies handling of deleted ConfigMaps.
func TestReconcileConfigMapNotFound(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logtesting.TestLogger(t))
	reconciler := &Reconciler{kubeclient: fake.NewSimpleClientset()}

	err := reconciler.Reconcile(ctx, "test-ns/"+config.PrunerNamespaceConfigMapName)

	assert.NoError(t, err)
}

// TestReconcileConfigMapUpdate verifies config updates are processed.
func TestReconcileConfigMapUpdate(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logtesting.TestLogger(t))
	initialCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PrunerNamespaceConfigMapName,
			Namespace: "test-ns",
		},
		Data: map[string]string{config.PrunerNamespaceConfigKey: "ttlSecondsAfterFinished: 3600"},
	}

	kubeClient := fake.NewSimpleClientset(initialCM)
	reconciler := &Reconciler{kubeclient: kubeClient}

	err := reconciler.Reconcile(ctx, "test-ns/"+config.PrunerNamespaceConfigMapName)
	assert.NoError(t, err)

	updatedCM := initialCM.DeepCopy()
	updatedCM.Data[config.PrunerNamespaceConfigKey] = "ttlSecondsAfterFinished: 7200"
	_, err = kubeClient.CoreV1().ConfigMaps("test-ns").Update(ctx, updatedCM, metav1.UpdateOptions{})
	assert.NoError(t, err)

	err = reconciler.Reconcile(ctx, "test-ns/"+config.PrunerNamespaceConfigMapName)
	assert.NoError(t, err)
}

// TestReconcileMultipleNamespaces verifies independent namespace config handling.
func TestReconcileMultipleNamespaces(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logtesting.TestLogger(t))
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: config.PrunerNamespaceConfigMapName, Namespace: "ns1"},
		Data:       map[string]string{config.PrunerNamespaceConfigKey: "ttlSecondsAfterFinished: 1800"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: config.PrunerNamespaceConfigMapName, Namespace: "ns2"},
		Data:       map[string]string{config.PrunerNamespaceConfigKey: "ttlSecondsAfterFinished: 3600"},
	}

	reconciler := &Reconciler{kubeclient: fake.NewSimpleClientset(cm1, cm2)}

	assert.NoError(t, reconciler.Reconcile(ctx, "ns1/"+config.PrunerNamespaceConfigMapName))
	assert.NoError(t, reconciler.Reconcile(ctx, "ns2/"+config.PrunerNamespaceConfigMapName))
}

// TestReconcileEmptyData verifies graceful handling of empty ConfigMap data.
func TestReconcileEmptyData(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logtesting.TestLogger(t))
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PrunerNamespaceConfigMapName,
			Namespace: "test-ns",
		},
		Data: map[string]string{},
	}

	reconciler := &Reconciler{kubeclient: fake.NewSimpleClientset(cm)}
	err := reconciler.Reconcile(ctx, "test-ns/"+config.PrunerNamespaceConfigMapName)

	assert.NoError(t, err)
}

// TestParseKeyVariations verifies key parsing edge cases.
func TestParseKeyVariations(t *testing.T) {
	tests := []struct {
		key         string
		wantNS      string
		wantName    string
		expectError bool
	}{
		{key: "test-ns/config-map", wantNS: "test-ns", wantName: "config-map", expectError: false},
		{key: "invalid-key", expectError: true},
		{key: "ns/name/extra", wantNS: "ns", wantName: "name/extra", expectError: false}, // SplitN allows this
		{key: "", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			ns, name, err := parseKey(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantNS, ns)
				assert.Equal(t, tt.wantName, name)
			}
		})
	}
}

// TestConcurrentReconciliation verifies thread-safe concurrent reconciliation.
func TestConcurrentReconciliation(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logtesting.TestLogger(t))

	var runtimeObjs []runtime.Object
	for i := 1; i <= 5; i++ {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.PrunerNamespaceConfigMapName,
				Namespace: fmt.Sprintf("concurrent-ns-%d", i),
			},
			Data: map[string]string{config.PrunerNamespaceConfigKey: "ttlSecondsAfterFinished: 3600"},
		}
		runtimeObjs = append(runtimeObjs, cm)
	}

	reconciler := &Reconciler{kubeclient: fake.NewSimpleClientset(runtimeObjs...)}
	done := make(chan bool, 5)

	for i := 1; i <= 5; i++ {
		go func(idx int) {
			key := fmt.Sprintf("concurrent-ns-%d/%s", idx, config.PrunerNamespaceConfigMapName)
			err := reconciler.Reconcile(ctx, key)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}
