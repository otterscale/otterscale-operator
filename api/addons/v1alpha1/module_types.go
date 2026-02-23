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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModuleSpec defines the desired state of an installed Module.
// A Module instantiates a ModuleTemplate by referencing it and optionally
// overriding the target namespace or Helm values.
type ModuleSpec struct {
	// TemplateRef is the name of the ModuleTemplate to instantiate.
	// This field is immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="templateRef is immutable"
	// +required
	TemplateRef string `json:"templateRef"`

	// Namespace overrides the default target namespace defined in the ModuleTemplate.
	// If not specified, the namespace from the ModuleTemplate is used.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?)$`
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// Values overrides the default values for HelmRelease-based modules.
	// Only applicable when the referenced ModuleTemplate uses a HelmRelease template.
	// Ignored for Kustomization-based modules.
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// ResourceReference is a lightweight reference to a Kubernetes resource managed by the operator.
type ResourceReference struct {
	// Name is the name of the referenced resource.
	// +required
	Name string `json:"name"`

	// Namespace is the namespace of the referenced resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ModuleStatus defines the observed state of a Module.
// It contains references to the actual FluxCD resources created by the controller
// and reflects their health status.
type ModuleStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// It corresponds to the Module's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// TemplateGeneration tracks the observed generation of the referenced ModuleTemplate.
	// This allows the controller to detect and reconcile template changes.
	// +optional
	TemplateGeneration int64 `json:"templateGeneration,omitempty"`

	// HelmReleaseRef is a reference to the FluxCD HelmRelease managed by this Module.
	// +optional
	HelmReleaseRef *ResourceReference `json:"helmReleaseRef,omitempty"`

	// KustomizationRef is a reference to the FluxCD Kustomization managed by this Module.
	// +optional
	KustomizationRef *ResourceReference `json:"kustomizationRef,omitempty"`

	// Conditions store the status conditions of the Module (e.g., Ready, TemplateNotFound).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.namespace`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Module is the Schema for the modules API.
// A Module represents an installed platform addon instantiated from a ModuleTemplate.
// The controller creates the corresponding FluxCD HelmRelease or Kustomization
// and reflects its status back to the Module.
type Module struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// Spec defines the desired behavior of the Module.
	// +required
	Spec ModuleSpec `json:"spec"`

	// Status represents the current information about the Module.
	// +optional
	Status ModuleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ModuleList contains a list of Module resources.
type ModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Module `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Module{}, &ModuleList{})
}
