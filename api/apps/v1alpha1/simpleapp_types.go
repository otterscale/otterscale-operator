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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// SimpleAppSpec defines the desired state of SimpleApp
type SimpleAppSpec struct {
	// deploymentSpec defines the Deployment configuration
	// +kubebuilder:validation:Required
	DeploymentSpec appsv1.DeploymentSpec `json:"deploymentSpec"`

	// serviceSpec defines the Service configuration
	// If specified, a Service will be created
	// +optional
	ServiceSpec *corev1.ServiceSpec `json:"serviceSpec,omitempty"`

	// pvcSpec defines the PersistentVolumeClaim configuration
	// If specified, a PVC will be created
	// +optional
	PVCSpec *corev1.PersistentVolumeClaimSpec `json:"pvcSpec,omitempty"`
}

// SimpleAppStatus defines the observed state of SimpleApp.
type SimpleAppStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// deploymentRef references the managed Deployment resource
	// +required
	DeploymentRef *corev1.ObjectReference `json:"deploymentRef,omitempty"`

	// serviceRef references the managed Service resource
	// +optional
	ServiceRef *corev1.ObjectReference `json:"serviceRef,omitempty"`

	// pvcRef references the managed PersistentVolumeClaim resource
	// +optional
	PVCRef *corev1.ObjectReference `json:"pvcRef,omitempty"`

	// conditions represent the current state of the SimpleApp resource.
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
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.deploymentSpec.replicas`,priority=1
// +kubebuilder:printcolumn:name="Service Type",type=string,JSONPath=`.spec.serviceSpec.type`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SimpleApp is the Schema for the simpleapps API
type SimpleApp struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of SimpleApp
	// +required
	Spec SimpleAppSpec `json:"spec"`

	// status defines the observed state of SimpleApp
	// +optional
	Status SimpleAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SimpleAppList contains a list of SimpleApp
type SimpleAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SimpleApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SimpleApp{}, &SimpleAppList{})
}
