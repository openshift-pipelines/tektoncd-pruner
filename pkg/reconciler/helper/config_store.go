package helper

import (
	"sync"

	tektonprunerv1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// for internal use
// to manage different resources and different fields
type PrunerResourceType string
type PrunerFieldType string

const (
	PrunerResourceTypePipeline PrunerResourceType = "pipeline"
	PrunerResourceTypeTask     PrunerResourceType = "task"

	PrunerFieldTypeTTLSecondsAfterFinished PrunerFieldType = "ttlSecondsAfterFinished"
	PrunerFieldTypeSuccessfulHistoryLimit  PrunerFieldType = "successfulHistoryLimit"
	PrunerFieldTypeFailedHistoryLimit      PrunerFieldType = "failedHistoryLimit"
)

// used to hold the config of a specific namespace
type PrunerResourceSpec struct {
	// EnforcedConfigLevel allowed values: global, namespace, resource (default: resource)
	EnforcedConfigLevel     *tektonprunerv1alpha1.EnforcedConfigLevel `yaml:"enforcedConfigLevel"`
	TTLSecondsAfterFinished *int32                                    `yaml:"ttlSecondsAfterFinished"`
	SuccessfulHistoryLimit  *int32                                    `yaml:"successfulHistoryLimit"`
	FailedHistoryLimit      *int32                                    `yaml:"failedHistoryLimit"`
	HistoryLimit            *int32                                    `yaml:"historyLimit"`
	Pipelines               []tektonprunerv1alpha1.ResourceSpec       `yaml:"pipelines"`
	Tasks                   []tektonprunerv1alpha1.ResourceSpec       `yaml:"tasks"`
}

// used to hold the config of namespaces
// and global config
type PrunerConfig struct {
	// EnforcedConfigLevel allowed values: global, namespace, resource (default: resource)
	EnforcedConfigLevel     *tektonprunerv1alpha1.EnforcedConfigLevel `yaml:"enforcedConfigLevel"`
	TTLSecondsAfterFinished *int32                                    `yaml:"ttlSecondsAfterFinished"`
	SuccessfulHistoryLimit  *int32                                    `yaml:"successfulHistoryLimit"`
	FailedHistoryLimit      *int32                                    `yaml:"failedHistoryLimit"`
	HistoryLimit            *int32                                    `yaml:"historyLimit"`
	Namespaces              map[string]PrunerResourceSpec             `yaml:"namespaces"`
}

// defines the store structure
// holds config from ConfigMap (global config) and config from namespaces (namespaced config)
type prunerConfigStore struct {
	mutex            sync.RWMutex
	globalConfig     PrunerConfig
	namespacedConfig map[string]PrunerResourceSpec
}

var (
	// store to manage pruner config
	// singleton instance
	PrunerConfigStore = prunerConfigStore{mutex: sync.RWMutex{}}
)

// loads config from configMap (global-config)
// should be called on startup and if there is a change detected on the ConfigMap
func (ps *prunerConfigStore) LoadGlobalConfig(configMap *corev1.ConfigMap) error {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	globalConfig := &PrunerConfig{}
	if configMap.Data != nil && configMap.Data[PrunerGlobalConfigKey] != "" {
		err := yaml.Unmarshal([]byte(configMap.Data[PrunerGlobalConfigKey]), globalConfig)
		if err != nil {
			return err
		}
	}

	ps.globalConfig = *globalConfig

	if ps.globalConfig.Namespaces == nil {
		ps.globalConfig.Namespaces = map[string]PrunerResourceSpec{}
	}

	if ps.namespacedConfig == nil {
		ps.namespacedConfig = map[string]PrunerResourceSpec{}
	}

	return nil
}

func (ps *prunerConfigStore) UpdateNamespacedSpec(prunerCR *tektonprunerv1alpha1.TektonPruner) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	namespace := prunerCR.Namespace

	// update in the local store
	namespacedSpec := PrunerResourceSpec{
		TTLSecondsAfterFinished: prunerCR.Spec.TTLSecondsAfterFinished,
		Pipelines:               prunerCR.Spec.Pipelines,
		Tasks:                   prunerCR.Spec.Tasks,
	}
	ps.namespacedConfig[namespace] = namespacedSpec
}

func (ps *prunerConfigStore) DeleteNamespacedSpec(namespace string) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	delete(ps.namespacedConfig, namespace)
}

func getFromPrunerConfigResourceLevel(namespacesSpec map[string]PrunerResourceSpec, namespace, name string, resourceType PrunerResourceType, fieldType PrunerFieldType) *int32 {
	prunerResourceSpec, found := namespacesSpec[namespace]
	if !found {
		return nil
	}

	var resourceSpecs []tektonprunerv1alpha1.ResourceSpec

	switch resourceType {
	case PrunerResourceTypePipeline:
		resourceSpecs = prunerResourceSpec.Pipelines

	case PrunerResourceTypeTask:
		resourceSpecs = prunerResourceSpec.Tasks
	}

	for _, resourceSpec := range resourceSpecs {
		if resourceSpec.Name == name {
			switch fieldType {
			case PrunerFieldTypeTTLSecondsAfterFinished:
				return resourceSpec.TTLSecondsAfterFinished

			case PrunerFieldTypeSuccessfulHistoryLimit:
				return resourceSpec.SuccessfulHistoryLimit

			case PrunerFieldTypeFailedHistoryLimit:
				return resourceSpec.FailedHistoryLimit
			}
		}
	}
	return nil
}

// TODO: Add unit test for this
func getResourceFieldData(namespacedSpec map[string]PrunerResourceSpec, globalSpec PrunerConfig, namespace, name string, resourceType PrunerResourceType, fieldType PrunerFieldType, enforcedConfigLevel tektonprunerv1alpha1.EnforcedConfigLevel) *int32 {
	var ttl *int32

	switch enforcedConfigLevel {
	case tektonprunerv1alpha1.EnforcedConfigLevelResource:
		// get from namespaced spec, resource level
		ttl = getFromPrunerConfigResourceLevel(namespacedSpec, namespace, name, resourceType, fieldType)

		fallthrough

	case tektonprunerv1alpha1.EnforcedConfigLevelNamespace:
		if ttl == nil {
			// get it from namespace spec, root level
			spec, found := namespacedSpec[namespace]
			if found {
				switch fieldType {
				case PrunerFieldTypeTTLSecondsAfterFinished:
					ttl = spec.TTLSecondsAfterFinished

				case PrunerFieldTypeSuccessfulHistoryLimit:
					ttl = spec.SuccessfulHistoryLimit

				case PrunerFieldTypeFailedHistoryLimit:
					ttl = spec.FailedHistoryLimit
				}
			}
		}
		fallthrough

	case tektonprunerv1alpha1.EnforcedConfigLevelGlobal:
		if ttl == nil {
			// get from global spec, resource level
			ttl = getFromPrunerConfigResourceLevel(globalSpec.Namespaces, namespace, name, resourceType, fieldType)
		}

		if ttl == nil {
			// get it from global spec, namespace root level
			spec, found := globalSpec.Namespaces[namespace]
			if found {
				switch fieldType {
				case PrunerFieldTypeTTLSecondsAfterFinished:
					ttl = spec.TTLSecondsAfterFinished

				case PrunerFieldTypeSuccessfulHistoryLimit:
					ttl = spec.SuccessfulHistoryLimit

				case PrunerFieldTypeFailedHistoryLimit:
					ttl = spec.FailedHistoryLimit
				}
			}
		}

		if ttl == nil {
			// get it from global spec, root level
			switch fieldType {
			case PrunerFieldTypeTTLSecondsAfterFinished:
				ttl = globalSpec.TTLSecondsAfterFinished

			case PrunerFieldTypeSuccessfulHistoryLimit:
				ttl = globalSpec.SuccessfulHistoryLimit

			case PrunerFieldTypeFailedHistoryLimit:
				ttl = globalSpec.FailedHistoryLimit
			}
		}

	}

	return ttl
}

func (ps *prunerConfigStore) GetEnforcedConfigLevelFromNamespaceSpec(namespacesSpec map[string]PrunerResourceSpec, namespace, name string, resourceType PrunerResourceType) *tektonprunerv1alpha1.EnforcedConfigLevel {
	var enforcedConfigLevel *tektonprunerv1alpha1.EnforcedConfigLevel
	var resourceSpecs []tektonprunerv1alpha1.ResourceSpec
	var namespaceSpec PrunerResourceSpec
	var found bool

	namespaceSpec, found = ps.globalConfig.Namespaces[namespace]
	if found {
		switch resourceType {
		case PrunerResourceTypePipeline:
			resourceSpecs = namespaceSpec.Pipelines

		case PrunerResourceTypeTask:
			resourceSpecs = namespaceSpec.Tasks
		}
		for _, resourceSpec := range resourceSpecs {
			if resourceSpec.Name == name {
				// if found on resource level
				enforcedConfigLevel = resourceSpec.EnforcedConfigLevel
				if enforcedConfigLevel != nil {
					return enforcedConfigLevel
				}
				break
			}
		}

		// get it from namespace root level
		enforcedConfigLevel = namespaceSpec.EnforcedConfigLevel
		if enforcedConfigLevel != nil {
			return enforcedConfigLevel
		}
	}
	return nil
}

func (ps *prunerConfigStore) getEnforcedConfigLevel(namespace, name string, resourceType PrunerResourceType) tektonprunerv1alpha1.EnforcedConfigLevel {
	var enforcedConfigLevel *tektonprunerv1alpha1.EnforcedConfigLevel

	// get it from global spec (order: resource level, namespace root level)
	enforcedConfigLevel = ps.GetEnforcedConfigLevelFromNamespaceSpec(ps.globalConfig.Namespaces, namespace, name, resourceType)
	if enforcedConfigLevel != nil {
		return *enforcedConfigLevel
	}

	// get it from global spec, root level
	enforcedConfigLevel = ps.globalConfig.EnforcedConfigLevel
	if enforcedConfigLevel != nil {
		return *enforcedConfigLevel
	}

	// get it from namespace spec (order: resource level, root level)
	enforcedConfigLevel = ps.GetEnforcedConfigLevelFromNamespaceSpec(ps.namespacedConfig, namespace, name, resourceType)
	if enforcedConfigLevel != nil {
		return *enforcedConfigLevel
	}

	// default level, if no where specified
	return tektonprunerv1alpha1.EnforcedConfigLevelResource
}

func (ps *prunerConfigStore) GetPipelineEnforcedConfigLevel(namespace, name string) tektonprunerv1alpha1.EnforcedConfigLevel {
	return ps.getEnforcedConfigLevel(namespace, name, PrunerResourceTypePipeline)
}

func (ps *prunerConfigStore) GetTaskEnforcedConfigLevel(namespace, name string) tektonprunerv1alpha1.EnforcedConfigLevel {
	return ps.getEnforcedConfigLevel(namespace, name, PrunerResourceTypeTask)
}

func (ps *prunerConfigStore) GetPipelineTTLSecondsAfterFinished(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetPipelineEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipeline, PrunerFieldTypeTTLSecondsAfterFinished, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetPipelineSuccessHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetPipelineEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipeline, PrunerFieldTypeSuccessfulHistoryLimit, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetPipelineFailedHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetPipelineEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipeline, PrunerFieldTypeFailedHistoryLimit, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetTaskTTLSecondsAfterFinished(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetTaskEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTask, PrunerFieldTypeTTLSecondsAfterFinished, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetTaskSuccessHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetTaskEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTask, PrunerFieldTypeSuccessfulHistoryLimit, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetTaskFailedHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetTaskEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTask, PrunerFieldTypeFailedHistoryLimit, enforcedConfigLevel)
}
