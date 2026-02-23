/*
Copyright 2026 The OtterScale Authors.

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

package v1alpha1

import (
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmReleaseTemplateSpec wraps the FluxCD HelmReleaseSpec for use as a module template.
type HelmReleaseTemplateSpec struct {
	// Spec is the FluxCD HelmRelease specification to use as a template.
	// +required
	Spec helmv2.HelmReleaseSpec `json:"spec"`
}

// KustomizationTemplateSpec wraps the FluxCD KustomizationSpec for use as a module template.
type KustomizationTemplateSpec struct {
	// Spec is the FluxCD Kustomization specification to use as a template.
	// +required
	Spec kustomizev1.KustomizationSpec `json:"spec"`
}

// ModuleTemplateSpec defines the desired state of a ModuleTemplate.
// It serves as a reusable catalog entry for platform modules, containing either
// a FluxCD HelmRelease or Kustomization specification.
// +kubebuilder:validation:XValidation:rule="(has(self.helmRelease) && !has(self.kustomization)) || (!has(self.helmRelease) && has(self.kustomization))",message="exactly one of helmRelease or kustomization must be set"
type ModuleTemplateSpec struct {
	// Description is a human-readable description of the module template.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Description string `json:"description,omitempty"`

	// Namespace is the default target namespace where FluxCD resources will be created
	// when a Module is instantiated from this template.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?)$`
	// +required
	Namespace string `json:"namespace"`

	// HelmRelease defines the FluxCD HelmRelease spec template.
	// Mutually exclusive with Kustomization (enforced via CEL).
	// +optional
	HelmRelease *HelmReleaseTemplateSpec `json:"helmRelease,omitempty"`

	// Kustomization defines the FluxCD Kustomization spec template.
	// Mutually exclusive with HelmRelease (enforced via CEL).
	// +optional
	Kustomization *KustomizationTemplateSpec `json:"kustomization,omitempty"`
}

// ModuleTemplateStatus defines the observed state of the ModuleTemplate.
type ModuleTemplateStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions store the status conditions of the ModuleTemplate.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.namespace`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ModuleTemplate is the Schema for the moduletemplates API.
// A ModuleTemplate defines a reusable platform module blueprint containing either
// a FluxCD HelmRelease or Kustomization specification. Users create Module CRs
// to instantiate and deploy modules from these templates.
type ModuleTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// Spec defines the module template blueprint.
	// +required
	Spec ModuleTemplateSpec `json:"spec"`

	// Status represents the current information about the ModuleTemplate.
	// +optional
	Status ModuleTemplateStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ModuleTemplateList contains a list of ModuleTemplate resources.
type ModuleTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ModuleTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModuleTemplate{}, &ModuleTemplateList{})
}
