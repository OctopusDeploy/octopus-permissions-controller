package rules

import (
	"crypto/sha256"
	"fmt"
)

// Constants for metadata keys (used in both labels and annotations)
const (
	MetadataNamespace     = "agent.octopus.com"
	PermissionsKey        = MetadataNamespace + "/permissions"
	ProjectKey            = MetadataNamespace + "/project"
	EnvironmentKey        = MetadataNamespace + "/environment"
	TenantKey             = MetadataNamespace + "/tenant"
	StepKey               = MetadataNamespace + "/step"
	SpaceKey              = MetadataNamespace + "/space"
	PermissionsContextKey = MetadataNamespace + "/permissions-context"
)

// generateServiceAccountName generates a ServiceAccountName based on the given scope
func generateServiceAccountName(scope Scope) ServiceAccountName {
	hash := shortHash(scope.String())
	return ServiceAccountName(fmt.Sprintf("octopus-sa-%s", hash))
}

// generateServiceAccountLabels generates the expected labels for a ServiceAccount based on scope
func generateServiceAccountLabels(scope Scope) map[string]string {
	labels := map[string]string{
		PermissionsKey: "enabled",
	}

	// Hash values for labels to keep them under 63 characters
	if scope.Project != "" {
		labels[ProjectKey] = shortHash(scope.Project)
	}
	if scope.Environment != "" {
		labels[EnvironmentKey] = shortHash(scope.Environment)
	}
	if scope.Tenant != "" {
		labels[TenantKey] = shortHash(scope.Tenant)
	}
	if scope.Step != "" {
		labels[StepKey] = shortHash(scope.Step)
	}
	if scope.Space != "" {
		labels[SpaceKey] = shortHash(scope.Space)
	}

	return labels
}

// generateExpectedAnnotations generates the expected annotations for a ServiceAccount
func generateExpectedAnnotations(scope Scope) map[string]string {
	annotations := make(map[string]string)

	// Store human-readable scope values in annotations
	if scope.Project != "" {
		annotations[ProjectKey] = scope.Project
	}
	if scope.Environment != "" {
		annotations[EnvironmentKey] = scope.Environment
	}
	if scope.Tenant != "" {
		annotations[TenantKey] = scope.Tenant
	}
	if scope.Step != "" {
		annotations[StepKey] = scope.Step
	}
	if scope.Space != "" {
		annotations[SpaceKey] = scope.Space
	}

	return annotations
}

// shortHash generates a hash of a string for use in labels and names
func shortHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", hash)[:32] // Use first 32 characters (128 bits)
}
