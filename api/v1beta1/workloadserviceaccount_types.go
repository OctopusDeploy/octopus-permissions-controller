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

// WorkloadServiceAccountSpec defines the desired state of WorkloadServiceAccount
type WorkloadServiceAccountSpec struct {
	// Scope defines the scope of the WorkloadServiceAccount.
	// +required
	Scope WorkloadServiceAccountScope `json:"scope"`
	// Permissions defines the permissions for the WorkloadServiceAccount.
	// +required
	Permissions WorkloadServiceAccountPermissions `json:"permissions"`
}

// WorkloadServiceAccountStatus defines the observed state of WorkloadServiceAccount.
type WorkloadServiceAccountStatus struct {
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

// WorkloadServiceAccountScope defines the octopus scope for the WorkloadServiceAccount
type WorkloadServiceAccountScope struct {
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	Projects []string `json:"projects,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	NewProperty []string `json:"newProperty,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	ProjectGroups []string `json:"projectGroups,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	Environments []string `json:"environments,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	Tenants []string `json:"tenants,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	Steps []string `json:"steps,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	Spaces []string `json:"spaces,omitempty"`
}

// WorkloadServiceAccountPermissions defines the permissions for the WorkloadServiceAccount.
type WorkloadServiceAccountPermissions struct {
	// +optional
	ClusterRoles []v1.RoleRef `json:"clusterRoles,omitempty"`
	// +optional
	Roles []v1.RoleRef `json:"roles,omitempty"`

	// Allows users to specify permissions, rather than roles
	// A role will be created with the specified permissions
	// +optional
	Permissions []v1.PolicyRule `json:"permissions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wsa

// WorkloadServiceAccount is the Schema for the workloadserviceaccounts API
type WorkloadServiceAccount struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of WorkloadServiceAccount
	// +required
	Spec WorkloadServiceAccountSpec `json:"spec"`

	// status defines the observed state of WorkloadServiceAccount
	// +optional
	Status WorkloadServiceAccountStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// WorkloadServiceAccountList contains a list of WorkloadServiceAccount
type WorkloadServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadServiceAccount `json:"items"`
}

func (wsa *WorkloadServiceAccount) GetConditions() *[]metav1.Condition {
	return &wsa.Status.Conditions
}

func init() {
	SchemeBuilder.Register(&WorkloadServiceAccount{}, &WorkloadServiceAccountList{})
}
