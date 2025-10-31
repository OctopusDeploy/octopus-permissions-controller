/*
Copyright 2025.

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

package v1beta1

import (
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterWorkloadServiceAccountScope is an alias for WorkloadServiceAccountScope, in case we need to extend it in the future
type ClusterWorkloadServiceAccountScope = WorkloadServiceAccountScope

// ClusterWorkloadServiceAccountPermissions defines the permissions for the ClusterWorkloadServiceAccount.
type ClusterWorkloadServiceAccountPermissions struct {
	// +optional
	ClusterRoles []v1.RoleRef `json:"clusterRoles,omitempty"`

	// Allows users to specify permissions, rather than roles
	// A cluster role will be created with the specified permissions
	// +optional
	Permissions []v1.PolicyRule `json:"permissions,omitempty"`
}

// ClusterWorkloadServiceAccountSpec defines the desired state of ClusterWorkloadServiceAccount
type ClusterWorkloadServiceAccountSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// +required
	Scope ClusterWorkloadServiceAccountScope `json:"scope"`

	// +required
	Permissions ClusterWorkloadServiceAccountPermissions `json:"permissions"`
}

// ClusterWorkloadServiceAccountStatus defines the observed state of ClusterWorkloadServiceAccount.
type ClusterWorkloadServiceAccountStatus struct {
	// conditions represent the current state of the ClusterWorkloadServiceAccount resource.
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
// +kubebuilder:resource:scope=Cluster,shortName=cwsa

// ClusterWorkloadServiceAccount is the Schema for the clusterworkloadserviceaccounts API
type ClusterWorkloadServiceAccount struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ClusterWorkloadServiceAccount
	// +required
	Spec ClusterWorkloadServiceAccountSpec `json:"spec"`

	// status defines the observed state of ClusterWorkloadServiceAccount
	// +optional
	Status ClusterWorkloadServiceAccountStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ClusterWorkloadServiceAccountList contains a list of ClusterWorkloadServiceAccount
type ClusterWorkloadServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterWorkloadServiceAccount `json:"items"`
}

func (cwsa *ClusterWorkloadServiceAccount) GetConditions() *[]metav1.Condition {
	return &cwsa.Status.Conditions
}

func init() {
	SchemeBuilder.Register(&ClusterWorkloadServiceAccount{}, &ClusterWorkloadServiceAccountList{})
}
