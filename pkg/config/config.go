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
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"knative.dev/pkg/logging"
)

// following types are for internal use

// PrunerResourceType is a string type used to represent different types of resources that the pruner manages
type PrunerResourceType string

// PrunerFieldType is a string type used to represent different configuration types for pruner
type PrunerFieldType string

// EnforcedConfigLevel is a string type to manage the different override levels allowed for Pruner config
type EnforcedConfigLevel string

const (
	// PrunerResourceTypePipelineRun represents the resource type for a PipelineRun in the pruner.
	PrunerResourceTypePipelineRun PrunerResourceType = "pipelineRun"

	// PrunerResourceTypeTaskRun represents the resource type for a TaskRun in the pruner.
	PrunerResourceTypeTaskRun PrunerResourceType = "taskRun"

	// PrunerFieldTypeTTLSecondsAfterFinished represents the field type for the TTL (Time-to-Live) in seconds after the resource is finished.
	PrunerFieldTypeTTLSecondsAfterFinished PrunerFieldType = "ttlSecondsAfterFinished"

	// PrunerFieldTypeSuccessfulHistoryLimit represents the field type for the successful history limit of a resource.
	PrunerFieldTypeSuccessfulHistoryLimit PrunerFieldType = "successfulHistoryLimit"

	// PrunerFieldTypeFailedHistoryLimit represents the field type for the failed history limit of a resource.
	PrunerFieldTypeFailedHistoryLimit PrunerFieldType = "failedHistoryLimit"

	// EnforcedConfigLevelGlobal represents the global config level for the pruner.
	EnforcedConfigLevelGlobal EnforcedConfigLevel = "global"

	// EnforcedConfigLevelNamespace represents the namespace config level for the pruner.
	EnforcedConfigLevelNamespace EnforcedConfigLevel = "namespace"

	// EnforcedConfigLevelResource represents the resource-level config for the pruner.
	EnforcedConfigLevelResource EnforcedConfigLevel = "resource"
)

// ResourceSpec is used to hold the config of a specific resource
type ResourceSpec struct {
	// EnforcedConfigLevel allowed values: global, namespace, resource (default: resource)
	Name                    string               `yaml:"name"`
	EnforcedConfigLevel     *EnforcedConfigLevel `yaml:"enforcedConfigLevel"`
	TTLSecondsAfterFinished *int32               `yaml:"ttlSecondsAfterFinished"`
	SuccessfulHistoryLimit  *int32               `yaml:"successfulHistoryLimit"`
	FailedHistoryLimit      *int32               `yaml:"failedHistoryLimit"`
	HistoryLimit            *int32               `yaml:"historyLimit"`
}

// PrunerResourceSpec is used to hold the config of a specific namespace
type PrunerResourceSpec struct {
	// EnforcedConfigLevel allowed values: global, namespace, resource (default: resource)
	EnforcedConfigLevel     *EnforcedConfigLevel `yaml:"enforcedConfigLevel"`
	TTLSecondsAfterFinished *int32               `yaml:"ttlSecondsAfterFinished"`
	SuccessfulHistoryLimit  *int32               `yaml:"successfulHistoryLimit"`
	FailedHistoryLimit      *int32               `yaml:"failedHistoryLimit"`
	HistoryLimit            *int32               `yaml:"historyLimit"`
	PipelineRuns            []ResourceSpec       `yaml:"pipelineRuns"`
	TaskRuns                []ResourceSpec       `yaml:"taskRuns"`
}

// PrunerConfig used to hold the config of namespaces
// and global config
type PrunerConfig struct {
	// EnforcedConfigLevel allowed values: global, namespace, resource (default: resource)
	EnforcedConfigLevel     *EnforcedConfigLevel          `yaml:"enforcedConfigLevel"`
	TTLSecondsAfterFinished *int32                        `yaml:"ttlSecondsAfterFinished"`
	SuccessfulHistoryLimit  *int32                        `yaml:"successfulHistoryLimit"`
	FailedHistoryLimit      *int32                        `yaml:"failedHistoryLimit"`
	HistoryLimit            *int32                        `yaml:"historyLimit"`
	Namespaces              map[string]PrunerResourceSpec `yaml:"namespaces"`
}

// prunerConfigStore defines the store structure
// holds config from ConfigMap (global config) and config from namespaces (namespaced config)
type prunerConfigStore struct {
	mutex            sync.RWMutex
	globalConfig     PrunerConfig
	namespacedConfig map[string]PrunerResourceSpec
}

var (
	// PrunerConfigStore is the singleton instance to store pruner config
	PrunerConfigStore = prunerConfigStore{mutex: sync.RWMutex{}}
)

// loads config from configMap (global-config)
// should be called on startup and if there is a change detected on the ConfigMap
func (ps *prunerConfigStore) LoadGlobalConfig(ctx context.Context, configMap *corev1.ConfigMap) error {
	logger := logging.FromContext(ctx)
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	// Log the current state of globalConfig and namespacedConfig before updating
	logger.Debugw("Loading global config",
		"oldGlobalConfig", ps.globalConfig,
		"oldNamespacedConfig", ps.namespacedConfig,
	)

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

	// Log the updated state of globalConfig and namespacedConfig after the update
	logger.Debugw("Updated global config",
		"newGlobalConfig", ps.globalConfig,
		"newNamespacedConfig", ps.namespacedConfig,
	)

	return nil
}

/*
func (ps *prunerConfigStore) UpdateNamespacedSpec(prunerCR *tektonprunerv1alpha1.TektonPruner) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	namespace := prunerCR.Namespace

	// update in the local store
	namespacedSpec := PrunerResourceSpec{
		TTLSecondsAfterFinished: prunerCR.Spec.TTLSecondsAfterFinished,
		PipelineRuns:               prunerCR.Spec.Pipelines,
		TaskRuns:                   prunerCR.Spec.Tasks,
	}
	ps.namespacedConfig[namespace] = namespacedSpec
}
*/

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

	var resourceSpecs []ResourceSpec

	switch resourceType {
	case PrunerResourceTypePipelineRun:
		resourceSpecs = prunerResourceSpec.PipelineRuns

	case PrunerResourceTypeTaskRun:
		resourceSpecs = prunerResourceSpec.TaskRuns
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

func getResourceFieldData(namespacedSpec map[string]PrunerResourceSpec, globalSpec PrunerConfig, namespace, name string, resourceType PrunerResourceType, fieldType PrunerFieldType, enforcedConfigLevel EnforcedConfigLevel) *int32 {
	var ttl *int32

	switch enforcedConfigLevel {
	case EnforcedConfigLevelResource:
		// get from namespaced spec, resource level
		ttl = getFromPrunerConfigResourceLevel(namespacedSpec, namespace, name, resourceType, fieldType)

		fallthrough

	case EnforcedConfigLevelNamespace:
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

	case EnforcedConfigLevelGlobal:
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

func (ps *prunerConfigStore) GetEnforcedConfigLevelFromNamespaceSpec(namespacesSpec map[string]PrunerResourceSpec, namespace, name string, resourceType PrunerResourceType) *EnforcedConfigLevel {
	var enforcedConfigLevel *EnforcedConfigLevel
	var resourceSpecs []ResourceSpec
	var namespaceSpec PrunerResourceSpec
	var found bool

	namespaceSpec, found = ps.globalConfig.Namespaces[namespace]
	if found {
		switch resourceType {
		case PrunerResourceTypePipelineRun:
			resourceSpecs = namespaceSpec.PipelineRuns

		case PrunerResourceTypeTaskRun:
			resourceSpecs = namespaceSpec.TaskRuns
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

func (ps *prunerConfigStore) getEnforcedConfigLevel(namespace, name string, resourceType PrunerResourceType) EnforcedConfigLevel {
	var enforcedConfigLevel *EnforcedConfigLevel

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
	return EnforcedConfigLevelResource
}

func (ps *prunerConfigStore) GetPipelineEnforcedConfigLevel(namespace, name string) EnforcedConfigLevel {
	return ps.getEnforcedConfigLevel(namespace, name, PrunerResourceTypePipelineRun)
}

func (ps *prunerConfigStore) GetTaskEnforcedConfigLevel(namespace, name string) EnforcedConfigLevel {
	return ps.getEnforcedConfigLevel(namespace, name, PrunerResourceTypeTaskRun)
}

func (ps *prunerConfigStore) GetPipelineTTLSecondsAfterFinished(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetPipelineEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipelineRun, PrunerFieldTypeTTLSecondsAfterFinished, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetPipelineSuccessHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetPipelineEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipelineRun, PrunerFieldTypeSuccessfulHistoryLimit, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetPipelineFailedHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetPipelineEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypePipelineRun, PrunerFieldTypeFailedHistoryLimit, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetTaskTTLSecondsAfterFinished(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetTaskEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTaskRun, PrunerFieldTypeTTLSecondsAfterFinished, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetTaskSuccessHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetTaskEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTaskRun, PrunerFieldTypeSuccessfulHistoryLimit, enforcedConfigLevel)
}

func (ps *prunerConfigStore) GetTaskFailedHistoryLimitCount(namespace, name string) *int32 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	enforcedConfigLevel := ps.GetTaskEnforcedConfigLevel(namespace, name)
	return getResourceFieldData(ps.namespacedConfig, ps.globalConfig, namespace, name, PrunerResourceTypeTaskRun, PrunerFieldTypeFailedHistoryLimit, enforcedConfigLevel)
}
