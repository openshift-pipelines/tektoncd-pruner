/*
Copyright 2026 The Tekton Authors

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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestIsPipelineRunOwned verifies both ownership signals and ensures unrelated
// metadata does not prevent standalone resources from being pruned.
func TestIsPipelineRunOwned(t *testing.T) {
	// tests covers ownership metadata shared by history and TaskRun processing.
	tests := []struct {
		name     string            // name describes the ownership scenario.
		metadata metav1.ObjectMeta // metadata contains the labels and owners to check.
		want     bool              // want indicates whether the resource has a PipelineRun parent.
	}{
		{
			name: "no ownership metadata",
		},
		{
			name: "empty labels",
			metadata: metav1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
		{
			name: "empty PipelineRun label",
			metadata: metav1.ObjectMeta{
				Labels: map[string]string{LabelPipelineRunName: ""},
			},
		},
		{
			name: "PipelineRun label",
			metadata: metav1.ObjectMeta{
				Labels: map[string]string{LabelPipelineRunName: "pipeline-run"},
			},
			want: true,
		},
		{
			name: "unrelated label",
			metadata: metav1.ObjectMeta{
				Labels: map[string]string{LabelTaskRunName: "task-run"},
			},
		},
		{
			name: "PipelineRun owner without label",
			metadata: metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{{Kind: KindPipelineRun}},
			},
			want: true,
		},
		{
			name: "unrelated owner",
			metadata: metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{{Kind: "Pod"}},
			},
		},
		{
			name: "PipelineRun among multiple owners",
			metadata: metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Pod"},
					{Kind: KindPipelineRun},
				},
			},
			want: true,
		},
		{
			name: "PipelineRun owner with empty label",
			metadata: metav1.ObjectMeta{
				Labels:          map[string]string{LabelPipelineRunName: ""},
				OwnerReferences: []metav1.OwnerReference{{Kind: KindPipelineRun}},
			},
			want: true,
		},
	}

	// testCase contains one ownership scenario and its expected result.
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, IsPipelineRunOwned(&testCase.metadata))
		})
	}
}
