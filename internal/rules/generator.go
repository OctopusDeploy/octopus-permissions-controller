package rules

import (
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

// GenerateAllScopesWithPermissions creates ServiceAccounts for all unique scope combinations
func GenerateAllScopesWithPermissions(wsas []v1beta1.WorkloadServiceAccount) map[Scope]v1beta1.WorkloadServiceAccountPermissions {
	if len(wsas) == 0 {
		return nil
	}

	result := make(map[Scope]v1beta1.WorkloadServiceAccountPermissions)

	// Get all unique scope values (including wildcards)
	projectSet, environmentSet, tenantSet, stepSet := getAllScopeValues(wsas)

	// Generate all possible scope combinations
	allScopeCombinations := generateAllScopeCombinations(projectSet, environmentSet, tenantSet, stepSet)

	// For each scope combination find WSAs that match the scope and merge their permissions
	// If no WSAs match the scope, no ServiceAccount is created for that scope
	for _, scope := range allScopeCombinations {
		matchingWSAs := getMatchingWSAsForScope(scope, wsas)
		if len(matchingWSAs) > 0 {
			var mergedPermissions v1beta1.WorkloadServiceAccountPermissions
			for _, wsa := range matchingWSAs {
				mergedPermissions = mergePermissions(mergedPermissions, wsa.Spec.Permissions)
			}
			result[scope] = mergedPermissions
		}
	}

	return result
}

// getAllScopeValues extracts all unique scope values from all WSAs
func getAllScopeValues(wsas []v1beta1.WorkloadServiceAccount) (projects, environments, tenants, steps map[string]struct{}) {
	projects = make(map[string]struct{})
	environments = make(map[string]struct{})
	tenants = make(map[string]struct{})
	steps = make(map[string]struct{})

	hasWildcardProjects := false
	hasWildcardEnvironments := false
	hasWildcardTenants := false
	hasWildcardSteps := false

	for _, wsa := range wsas {
		processScopeValues(wsa.Spec.Scope.Projects, projects, &hasWildcardProjects)
		processScopeValues(wsa.Spec.Scope.Environments, environments, &hasWildcardEnvironments)
		processScopeValues(wsa.Spec.Scope.Tenants, tenants, &hasWildcardTenants)
		processScopeValues(wsa.Spec.Scope.Steps, steps, &hasWildcardSteps)
	}

	if hasWildcardProjects {
		projects["*"] = struct{}{}
	}
	if hasWildcardEnvironments {
		environments["*"] = struct{}{}
	}
	if hasWildcardTenants {
		tenants["*"] = struct{}{}
	}
	if hasWildcardSteps {
		steps["*"] = struct{}{}
	}

	return projects, environments, tenants, steps
}

func processScopeValues(slice []string, valueSet map[string]struct{}, hasWildcard *bool) {
	if !*hasWildcard && len(slice) == 0 {
		*hasWildcard = true
	} else {
		for _, value := range slice {
			valueSet[value] = struct{}{}
		}
	}
}

// generateAllScopeCombinations generates all possible scope combinations
func generateAllScopeCombinations(projects, environments, tenants, steps map[string]struct{}) []Scope {
	capacity := len(projects) * len(environments) * len(tenants) * len(steps)
	scopes := make([]Scope, 0, capacity)

	for project := range projects {
		for environment := range environments {
			for tenant := range tenants {
				for step := range steps {
					scope := Scope{
						Project:     project,
						Environment: environment,
						Tenant:      tenant,
						Step:        step,
					}
					scopes = append(scopes, scope)
				}
			}
		}
	}

	return scopes
}

// getMatchingWSAsForScope returns all WSAs that apply to the given concrete scope
func getMatchingWSAsForScope(scope Scope, wsas []v1beta1.WorkloadServiceAccount) []v1beta1.WorkloadServiceAccount {
	var matchingWSAs []v1beta1.WorkloadServiceAccount

	for _, wsa := range wsas {
		if scopeMatchesWSA(scope, wsa) {
			matchingWSAs = append(matchingWSAs, wsa)
		}
	}

	return matchingWSAs
}

// scopeMatchesWSA checks if a WSA applies to a concrete scope
func scopeMatchesWSA(scope Scope, wsa v1beta1.WorkloadServiceAccount) bool {
	return hasMatchingScopeValue(wsa.Spec.Scope.Projects, scope.Project) &&
		hasMatchingScopeValue(wsa.Spec.Scope.Environments, scope.Environment) &&
		hasMatchingScopeValue(wsa.Spec.Scope.Tenants, scope.Tenant) &&
		hasMatchingScopeValue(wsa.Spec.Scope.Steps, scope.Step)
}

// hasMatchingScopeValue checks if a WSA dimension matches a concrete scope value
func hasMatchingScopeValue(wsaScopes []string, scopeValue string) bool {
	// Empty WSA scope list means wildcard (matches any value)
	if len(wsaScopes) == 0 {
		return true
	}

	for _, value := range wsaScopes {
		if value == scopeValue {
			return true
		}
	}

	return false
}

// mergePermissions combines permissions from multiple WSAs for the same scope
func mergePermissions(existing, new v1beta1.WorkloadServiceAccountPermissions) v1beta1.WorkloadServiceAccountPermissions {
	merged := v1beta1.WorkloadServiceAccountPermissions{
		ClusterRoles: append(existing.ClusterRoles, new.ClusterRoles...),
		Roles:        append(existing.Roles, new.Roles...),
		Permissions:  append(existing.Permissions, new.Permissions...),
	}

	// TODO: Deduplicate identical roles and permissions

	return merged
}
