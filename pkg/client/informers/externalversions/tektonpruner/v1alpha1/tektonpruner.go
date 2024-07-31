/*
Copyright 2024 The Tekton Authors

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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	tektonprunerv1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/apis/tektonpruner/v1alpha1"
	versioned "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/clientset/versioned"
	internalinterfaces "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/openshift-pipelines/tektoncd-pruner/pkg/client/listers/tektonpruner/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// TektonPrunerInformer provides access to a shared informer and lister for
// TektonPruners.
type TektonPrunerInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.TektonPrunerLister
}

type tektonPrunerInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewTektonPrunerInformer constructs a new informer for TektonPruner type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewTektonPrunerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredTektonPrunerInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredTektonPrunerInformer constructs a new informer for TektonPruner type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredTektonPrunerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PrunerV1alpha1().TektonPruners(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PrunerV1alpha1().TektonPruners(namespace).Watch(context.TODO(), options)
			},
		},
		&tektonprunerv1alpha1.TektonPruner{},
		resyncPeriod,
		indexers,
	)
}

func (f *tektonPrunerInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredTektonPrunerInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *tektonPrunerInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&tektonprunerv1alpha1.TektonPruner{}, f.defaultInformer)
}

func (f *tektonPrunerInformer) Lister() v1alpha1.TektonPrunerLister {
	return v1alpha1.NewTektonPrunerLister(f.Informer().GetIndexer())
}
