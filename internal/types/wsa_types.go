package types

type WorkloadServiceAccountScope struct {
	// +optional
	// +kubebuilder:validation:items:Pattern="^[\\p{Ll}\\p{N}]+(?:-[\\p{L}\\p{N}]+)*$"
	Projects []string `json:"projects,omitempty"`
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
