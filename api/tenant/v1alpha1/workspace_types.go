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

// MemberRole defines the role of a member in the workspace.
// It determines the RBAC permissions granted within the target namespace.
// +kubebuilder:validation:Enum=admin;edit;view
// +enum
type MemberRole string

const (
	// MemberRoleAdmin has full control over the workspace resources.
	MemberRoleAdmin MemberRole = "admin"
	// MemberRoleEdit can create/update application resources but cannot modify role bindings.
	MemberRoleEdit MemberRole = "edit"
	// MemberRoleView has read-only access to resources.
	MemberRoleView MemberRole = "view"
)

// WorkspaceMember defines a single member entity associated with a workspace.
type WorkspaceMember struct {
	// Role defines the authorization level (Admin, Edit, View).
	// +required
	Role MemberRole `json:"role"`

	// Subject is the unique identifier of the member (e.g., OIDC subject or username).
	// This identifier maps directly to the Kubernetes RBAC Subject.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$`
	// +required
	Subject string `json:"subject"`

	// Name is the human-readable display name of the member.
	// +optional
	Name *string `json:"name,omitempty"`
}

// NetworkIsolationSpec configures network policies for the workspace.
// It supports both standard NetworkPolicy and Istio AuthorizationPolicy.
// +kubebuilder:validation:XValidation:rule="!has(self.allowedNamespaces) || size(self.allowedNamespaces) == 0 || self.enabled",message="allowedNamespaces can only be set when network isolation is enabled"
type NetworkIsolationSpec struct {
	// Enabled toggles the enforcement of network isolation.
	// If true, default deny-all ingress rules are applied except for allowed namespaces.
	// +optional
	Enabled bool `json:"enabled"`

	// AllowedNamespaces specifies a list of external namespaces permitted to access this workspace
	// when isolation is enabled. Essential system namespaces (e.g., 'istio-system', 'monitoring')
	// should be included here if required.
	// +listType=set
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=63
	// +kubebuilder:validation:items:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?)$`
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}

// WorkspaceSpec defines the desired state of the Workspace.
// It includes member management, resource constraints, and network security settings.
type WorkspaceSpec struct {
	// Namespace is the name of the Kubernetes Namespace to be created for this workspace.
	// It must be unique across all Workspaces.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?)$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="namespace is immutable"
	// +kubebuilder:validation:XValidation:rule="!(self in ['default','kube-system','kube-public','kube-node-lease','otterscale-system'])",message="namespace is reserved and cannot be used for a workspace"
	// +required
	Namespace string `json:"namespace"`

	// Members is the list of members granted access to this workspace.
	// +listType=map
	// +listMapKey=subject
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.exists(u, u.role == 'admin')",message="at least one workspace member must have role 'admin'"
	// +required
	Members []WorkspaceMember `json:"members"`

	// ResourceQuota describes the compute resource constraints (CPU, Memory, etc.) applied to the underlying namespace.
	// +optional
	ResourceQuota *corev1.ResourceQuotaSpec `json:"resourceQuota,omitempty"`

	// LimitRange describes the default resource limits and requests for pods in the workspace.
	// +optional
	LimitRange *corev1.LimitRangeSpec `json:"limitRange,omitempty"`

	// NetworkIsolation defines the ingress traffic rules for the workspace.
	// +optional
	NetworkIsolation NetworkIsolationSpec `json:"networkIsolation,omitzero"`
}

// ResourceReference is a lightweight reference to a Kubernetes resource managed by the operator.
// Unlike corev1.ObjectReference, it only contains the essential fields needed
// to identify the resource, avoiding stale UID/ResourceVersion issues.
type ResourceReference struct {
	// Name is the name of the referenced resource.
	// +required
	Name string `json:"name"`

	// Namespace is the namespace of the referenced resource.
	// Empty for cluster-scoped resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// WorkspaceStatus defines the observed state of the Workspace.
// It contains references to the actual Kubernetes resources created by the operator.
type WorkspaceStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// It corresponds to the Workspace's generation, which is updated on mutation by the API Server.
	// This allows clients to determine whether the controller has processed the latest spec changes.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// NamespaceRef is a reference to the Namespace managed by this Workspace.
	// +optional
	NamespaceRef *ResourceReference `json:"namespaceRef,omitempty"`

	// ResourceQuotaRef is a reference to the ResourceQuota managed by this Workspace.
	// +optional
	ResourceQuotaRef *ResourceReference `json:"resourceQuotaRef,omitempty"`

	// LimitRangeRef is a reference to the LimitRange managed by this Workspace.
	// +optional
	LimitRangeRef *ResourceReference `json:"limitRangeRef,omitempty"`

	// RoleBindingRefs contains references to all RBAC RoleBindings created for the workspace members.
	// +listType=map
	// +listMapKey=name
	// +optional
	RoleBindingRefs []ResourceReference `json:"roleBindingRefs,omitempty"`

	// PeerAuthenticationRef is a reference to the Istio PeerAuthentication resource for mTLS settings.
	// +optional
	PeerAuthenticationRef *ResourceReference `json:"peerAuthenticationRef,omitempty"`

	// AuthorizationPolicyRef is a reference to the Istio AuthorizationPolicy enforcing network isolation.
	// +optional
	AuthorizationPolicyRef *ResourceReference `json:"authorizationPolicyRef,omitempty"`

	// NetworkPolicyRef is a reference to the NetworkPolicy enforcing network isolation.
	// +optional
	NetworkPolicyRef *ResourceReference `json:"networkPolicyRef,omitempty"`

	// Conditions store the status conditions of the Workspace (e.g., Ready, Failed).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespaceRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Workspace is the Schema for the workspaces API.
// A Workspace represents a logical isolation unit (Namespace) with associated policies, quotas, and member access.
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
