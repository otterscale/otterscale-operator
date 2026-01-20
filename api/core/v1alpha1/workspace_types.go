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

// WorkspaceUserRole defines the role of a user in the workspace.
// It determines the RBAC permissions granted within the target namespace.
// +kubebuilder:validation:Enum=admin;edit;view
// +enum
type WorkspaceUserRole string

const (
	// WorkspaceUserRoleAdmin has full control over the workspace resources.
	WorkspaceUserRoleAdmin WorkspaceUserRole = "admin"
	// WorkspaceUserRoleEdit can create/update application resources but cannot modify role bindings.
	WorkspaceUserRoleEdit WorkspaceUserRole = "edit"
	// WorkspaceUserRoleView has read-only access to resources.
	WorkspaceUserRoleView WorkspaceUserRole = "view"
)

// WorkspaceUser defines a single user entity associated with a workspace.
type WorkspaceUser struct {
	// Role defines the authorization level (Admin, Edit, View).
	// +required
	Role WorkspaceUserRole `json:"role"`

	// Subject is the unique identifier of the user (e.g., OIDC subject or username).
	// This identifier maps directly to the Kubernetes RBAC Subject.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$`
	// +required
	Subject string `json:"subject"`

	// Name is the human-readable display name of the user.
	// +optional
	Name *string `json:"name,omitempty"`
}

// WorkspaceNetworkIsolation configures network policies for the workspace.
// It supports both standard NetworkPolicy and Istio AuthorizationPolicy.
type WorkspaceNetworkIsolation struct {
	// Enabled toggles the enforcement of network isolation.
	// If true, default deny-all ingress rules are applied except for allowed namespaces.
	// +optional
	Enabled bool `json:"enabled"`

	// AllowedNamespaces specifies a list of external namespaces permitted to access this workspace
	// when isolation is enabled. Essential system namespaces (e.g., 'istio-system', 'monitoring')
	// should be included here if required.
	// +listType=set
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}

// WorkspaceSpec defines the desired state of the Workspace.
// It includes user management, resource constraints, and network security settings.
//
// +kubebuilder:validation:XValidation:rule="self.users.exists(u, u.role == 'admin')",message="At least one user must have the 'admin' role"
type WorkspaceSpec struct {
	// Users is the list of users granted access to this workspace.
	// +listType=map
	// +listMapKey=subject
	// +kubebuilder:validation:MinItems=1
	// +required
	Users []WorkspaceUser `json:"users,omitempty"`

	// ResourceQuota describes the compute resource constraints (CPU, Memory, etc.) applied to the underlying namespace.
	// +optional
	ResourceQuota *corev1.ResourceQuotaSpec `json:"resourceQuota,omitempty"`

	// LimitRange describes the default resource limits and requests for pods in the workspace.
	// +optional
	LimitRange *corev1.LimitRangeSpec `json:"limitRange,omitempty"`

	// NetworkIsolation defines the ingress traffic rules for the workspace.
	// +optional
	NetworkIsolation WorkspaceNetworkIsolation `json:"networkIsolation,omitzero"`
}

// WorkspaceStatus defines the observed state of the Workspace.
// It contains references to the actual Kubernetes resources created by the operator.
type WorkspaceStatus struct {
	// NamespaceRef is a reference to the corev1.Namespace managed by this Workspace.
	// +optional
	NamespaceRef *corev1.ObjectReference `json:"namespaceRef,omitempty"`

	// ResourceQuotaRef is a reference to the corev1.ResourceQuota managed by this Workspace.
	// +optional
	ResourceQuotaRef *corev1.ObjectReference `json:"resourceQuotaRef,omitempty"`

	// LimitRangeRef is a reference to the corev1.LimitRange managed by this Workspace.
	// +optional
	LimitRangeRef *corev1.ObjectReference `json:"limitRangeRef,omitempty"`

	// RoleBindingRefs contains references to all RBAC RoleBindings created for the workspace users.
	// +listType=atomic
	// +optional
	RoleBindingRefs []corev1.ObjectReference `json:"roleBindingRefs,omitempty"`

	// PeerAuthenticationRef is a reference to the Istio PeerAuthentication resource for mTLS settings.
	// +optional
	PeerAuthenticationRef *corev1.ObjectReference `json:"peerAuthenticationRef,omitempty"`

	// AuthorizationPolicyRef is a reference to the Istio AuthorizationPolicy enforcing network isolation.
	// +optional
	AuthorizationPolicyRef *corev1.ObjectReference `json:"authorizationPolicyRef,omitempty"`

	// NetworkPolicyRef is a reference to the corev1.NetworkPolicy enforcing network isolation.
	// +optional
	NetworkPolicyRef *corev1.ObjectReference `json:"networkPolicyRef,omitempty"`

	// Conditions store the status conditions of the Workspace (e.g., Ready, Failed).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace.name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Workspace is the Schema for the workspaces API.
// A Workspace represents a logical isolation unit (Namespace) with associated policies, quotas, and user access.
type Workspace struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// Spec defines the desired behavior of the Workspace.
	// +required
	Spec WorkspaceSpec `json:"spec"`

	// Status represents the current information about the Workspace.
	// +optional
	Status WorkspaceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace resources.
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
