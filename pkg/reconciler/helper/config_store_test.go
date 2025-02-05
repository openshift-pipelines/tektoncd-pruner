package helper

import (
	"sync"
	"testing"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	tektonprunerv1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLoadGlobalConfig(t *testing.T) {
	type testCase struct {
		name           string
		configMap      *corev1.ConfigMap
		expectedConfig PrunerConfig
		expectError    bool
	}

	ttlSeconds := int32(3600)
	successfulHistory := int32(5)
	failedHistory := int32(3)
	historyLimit := int32(10)

	tests := []testCase{
		{
			name:      "Empty ConfigMap",
			configMap: &corev1.ConfigMap{},
			expectedConfig: PrunerConfig{
				Namespaces: map[string]PrunerResourceSpec{},
			},
			expectError: false,
		},
		{
			name: "Valid ConfigMap Data",
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					PrunerGlobalConfigKey: `
namespaces:
  default:
    ttlSecondsAfterFinished: 3600
    successfulHistoryLimit: 5
    failedHistoryLimit: 3
    historyLimit: 10
    pipelines:
      - name: "pipeline-1"
      - name: "pipeline-2"
    tasks:
      - name: "task-1"
      - name: "task-2"
`,
				},
			},
			expectedConfig: PrunerConfig{
				Namespaces: map[string]PrunerResourceSpec{
					"default": {
						TTLSecondsAfterFinished: &ttlSeconds,
						SuccessfulHistoryLimit:  &successfulHistory,
						FailedHistoryLimit:      &failedHistory,
						HistoryLimit:            &historyLimit,
						Pipelines: []tektonprunerv1alpha1.ResourceSpec{
							{Name: "pipeline-1"},
							{Name: "pipeline-2"},
						},
						Tasks: []tektonprunerv1alpha1.ResourceSpec{
							{Name: "task-1"},
							{Name: "task-2"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid YAML in ConfigMap Data",
			configMap: &corev1.ConfigMap{
				Data: map[string]string{
					PrunerGlobalConfigKey: "invalid-yaml-data",
				},
			},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ps := &prunerConfigStore{
				mutex:            sync.RWMutex{},
				namespacedConfig: map[string]PrunerResourceSpec{},
			}

			err := ps.LoadGlobalConfig(test.configMap)

			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedConfig.Namespaces, ps.globalConfig.Namespaces)
			}
		})
	}
}

func TestUpdateNamespacedSpec(t *testing.T) {
	ttlSeconds := int32(3600)

	tp := &v1alpha1.TektonPruner{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: v1alpha1.TektonPrunerSpec{
			TTLSecondsAfterFinished: &ttlSeconds,
			Pipelines: []tektonprunerv1alpha1.ResourceSpec{
				{Name: "pipeline-1"},
				{Name: "pipeline-2"},
			},
			Tasks: []tektonprunerv1alpha1.ResourceSpec{
				{Name: "task-1"},
				{Name: "task-2"},
			},
		},
	}

	ps := &prunerConfigStore{
		mutex:            sync.RWMutex{},
		namespacedConfig: map[string]PrunerResourceSpec{},
	}

	ps.UpdateNamespacedSpec(tp)

	ns := PrunerResourceSpec{
		TTLSecondsAfterFinished: tp.Spec.TTLSecondsAfterFinished,
		Pipelines:               tp.Spec.Pipelines,
		Tasks:                   tp.Spec.Tasks,
	}

	assert.Equal(t, ps.namespacedConfig[tp.Namespace], ns)
}

func TestGetFromPrunerConfigResourceLevel(t *testing.T) {
	namespaceSpec := map[string]PrunerResourceSpec{
		"namespace1": {
			Pipelines: []tektonprunerv1alpha1.ResourceSpec{
				{
					Name:                    "pipeline1",
					TTLSecondsAfterFinished: int32Ptr(300),
					SuccessfulHistoryLimit:  int32Ptr(5),
					FailedHistoryLimit:      int32Ptr(2),
				},
			},
			Tasks: []tektonprunerv1alpha1.ResourceSpec{
				{
					Name:                    "task1",
					TTLSecondsAfterFinished: int32Ptr(600),
					SuccessfulHistoryLimit:  int32Ptr(10),
					FailedHistoryLimit:      int32Ptr(3),
				},
			},
		},
	}

	tests := []struct {
		name         string
		namespace    string
		nameArg      string
		resourceType PrunerResourceType
		fieldType    PrunerFieldType
		expected     *int32
	}{
		{
			name:         "Pipeline - TTLSecondsAfterFinished",
			namespace:    "namespace1",
			nameArg:      "pipeline1",
			resourceType: PrunerResourceTypePipeline,
			fieldType:    PrunerFieldTypeTTLSecondsAfterFinished,
			expected:     int32Ptr(300),
		},
		{
			name:         "Task - TTLSecondsAfterFinished",
			namespace:    "namespace1",
			nameArg:      "task1",
			resourceType: PrunerResourceTypeTask,
			fieldType:    PrunerFieldTypeTTLSecondsAfterFinished,
			expected:     int32Ptr(600),
		},
		{
			name:         "Non existing namespace",
			namespace:    "foo",
			nameArg:      "task1",
			resourceType: PrunerResourceTypeTask,
			fieldType:    PrunerFieldTypeTTLSecondsAfterFinished,
			expected:     nil,
		},
		{
			name:         "Non existing resource",
			namespace:    "foo",
			nameArg:      "foo-resource",
			resourceType: PrunerResourceTypeTask,
			fieldType:    PrunerFieldTypeTTLSecondsAfterFinished,
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := getFromPrunerConfigResourceLevel(namespaceSpec, tt.namespace, tt.nameArg, tt.resourceType, tt.fieldType)
			// fmt.Println("=========================", *res)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestGetEnforcedConfigLevelFromNamespaceSpec(t *testing.T) {
	enforcedConfigLevelNamespace := tektonprunerv1alpha1.EnforcedConfigLevelNamespace
	enforcedConfigLevelResource := tektonprunerv1alpha1.EnforcedConfigLevelResource

	ps := &prunerConfigStore{
		globalConfig: PrunerConfig{
			Namespaces: map[string]PrunerResourceSpec{
				"test-namespace": {
					EnforcedConfigLevel: &enforcedConfigLevelNamespace,
					Pipelines: []tektonprunerv1alpha1.ResourceSpec{
						{Name: "test-pipeline", EnforcedConfigLevel: &enforcedConfigLevelResource},
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		namespace     string
		resourceName  string
		resourceType  PrunerResourceType
		expectedLevel *tektonprunerv1alpha1.EnforcedConfigLevel
	}{
		{
			name:          "Resource level enforcement",
			namespace:     "test-namespace",
			resourceName:  "test-pipeline",
			resourceType:  PrunerResourceTypePipeline,
			expectedLevel: &enforcedConfigLevelResource,
		},
		{
			name:          "Namespace level enforcement",
			namespace:     "test-namespace",
			resourceName:  "non-existent-pipeline",
			resourceType:  PrunerResourceTypePipeline,
			expectedLevel: &enforcedConfigLevelNamespace,
		},
		{
			name:          "Non-existent namespace",
			namespace:     "unknown-namespace",
			resourceName:  "test-pipeline",
			resourceType:  PrunerResourceTypePipeline,
			expectedLevel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ps.GetEnforcedConfigLevelFromNamespaceSpec(ps.globalConfig.Namespaces, tt.namespace, tt.resourceName, tt.resourceType)
			assert.Equal(t, tt.expectedLevel, result)
		})
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}
