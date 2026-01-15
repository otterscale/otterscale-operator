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

// WorkspaceOwner defines the owner of a workspace.
type WorkspaceOwner struct {
	// Subject is the unique identifier of the owner.
	// +kubebuilder:validation:MinLength=1
	// +required
	Subject string `json:"subject,omitempty"`

	// Name is the display name of the owner.
	// +optional
	Name *string `json:"name,omitempty"`
}

// WorkspaceSpec defines the desired state of Workspace
type WorkspaceSpec struct {
	// Owner is the owner of the workspace.
	// +required
	Owner WorkspaceOwner `json:"owner,omitzero"`

	// ResourceQuota defines the resource quota for the workspace.
	// +optional
	ResourceQuota *corev1.ResourceQuotaSpec `json:"resourceQuota,omitempty"`

	// LimitRange defines the limit range for the workspace.
	// +optional
	LimitRange *corev1.LimitRangeSpec `json:"limitRange,omitempty"`

	// NetworkIsolationEnabled indicates whether network isolation is enabled for the workspace.
	// +optional
	NetworkIsolationEnabled *bool `json:"networkIsolationEnabled,omitempty"`
}

// WorkspaceStatus defines the observed state of Workspace.
type WorkspaceStatus struct {
	// Namespace is the namespace associated with the workspace.
	// +optional
	Namespace *corev1.ObjectReference `json:"namespace,omitempty"`

	// ResourceQuota is the resource quota object associated with the workspace.
	// +optional
	ResourceQuota *corev1.ObjectReference `json:"resourceQuota,omitempty"`

	// LimitRange is the limit range object associated with the workspace.
	// +optional
	LimitRange *corev1.ObjectReference `json:"limitRange,omitempty"`

	// conditions represent the current state of the Workspace resource.
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

// Workspace is the Schema for the workspaces API
type Workspace struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Workspace
	// +required
	Spec WorkspaceSpec `json:"spec"`

	// status defines the observed state of Workspace
	// +optional
	Status WorkspaceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
