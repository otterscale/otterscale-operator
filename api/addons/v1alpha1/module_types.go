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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ModuleSpec defines the desired state of Module
type ModuleSpec struct {
	// Enabled toggles this module on/off.
	//
	// - When enabled is true, the operator will ensure the corresponding Flux resource
	//   (Kustomization or HelmRelease) exists.
	// - When enabled is false, the operator will delete the previously created Flux resource(s).
	//
	// +optional
	Enabled bool `json:"enabled"`

	// TemplateRef selects which template entry to use from the templates ConfigMap.
	// If omitted, the operator will use this Module's name (metadata.name) as the template ID.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?)$`
	TemplateRef *string `json:"templateRef,omitempty"`
}

// ModuleStatus defines the observed state of Module.
type ModuleStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// AppliedResources contains references to Flux resources created/managed for this Module.
	// It is used for cleanup when the module is disabled.
	//
	// +listType=atomic
	// +optional
	AppliedResources []corev1.ObjectReference `json:"appliedResources,omitempty"`

	// TemplateResourceVersion captures the templates ConfigMap resourceVersion that was last applied.
	// This helps with observability/debugging when templates are updated.
	// +optional
	TemplateResourceVersion *string `json:"templateResourceVersion,omitempty"`

	// conditions represent the current state of the Module resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Enabled",type=boolean,JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Module is the Schema for the modules API
type Module struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Module
	// +required
	Spec ModuleSpec `json:"spec"`

	// status defines the observed state of Module
	// +optional
	Status ModuleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ModuleList contains a list of Module
type ModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Module `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Module{}, &ModuleList{})
}
