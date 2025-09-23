package rules

import (
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

//func getServiceAccountsForWsas(wsas []*v1beta1.WorkloadServiceAccount) map[Scope][]*v1beta1.WorkloadServiceAccount {
//	result := make(map[string][]*v1beta1.WorkloadServiceAccount)
//
//	// Get all unique scope values (including wildcards)
//	projectSet, environmentSet, tenantSet, stepSet, spaceSet := getAllScopeValues(wsas)
//
//	// Generate all possible scope combinations
//	allScopeCombinations := generateAllScopeCombinations(projectSet, environmentSet, tenantSet, stepSet, spaceSet)
//
//}

// generateAllScopesWithPermissions creates ServiceAccounts for all unique scope combinations
func generateAllScopesWithPermissions(wsas []*v1beta1.WorkloadServiceAccount) map[Scope]v1beta1.WorkloadServiceAccountPermissions {
	if len(wsas) == 0 {
		return make(map[Scope]v1beta1.WorkloadServiceAccountPermissions)
	}

	result := make(map[Scope]v1beta1.WorkloadServiceAccountPermissions)

	// Get all unique scope values (including wildcards)
	//projectSet, environmentSet, tenantSet, stepSet, spaceSet := getAllScopeValues(wsas)

	// Generate all possible scope combinations
	//allScopeCombinations := generateAllScopeCombinations(projectSet, environmentSet, tenantSet, stepSet, spaceSet)
	//
	//// For each scope combination find WSAs that match the scope and merge their permissions
	//// If no WSAs match the scope, no ServiceAccount is created for that scope
	//for _, scope := range allScopeCombinations {
	//	matchingWSAs := getMatchingWSAsForScope(scope, wsas)
	//	if len(matchingWSAs) > 0 {
	//		var mergedPermissions v1beta1.WorkloadServiceAccountPermissions
	//		for _, wsa := range matchingWSAs {
	//			mergedPermissions = mergePermissions(mergedPermissions, wsa.Spec.Permissions)
	//		}
	//		result[scope] = mergedPermissions
	//	}
	//}

	return result
}

type ScopeType int

const (
	ScopeProjects ScopeType = iota
	ScopeEnvironments
	ScopeTenants
	ScopeSteps
	ScopeSpaces
)

// getAllScopeValues extracts all unique scope values from all WSAs
func getAllScopeValues(wsas []*v1beta1.WorkloadServiceAccount) (projects, environments, tenants, steps, spaces map[string][]*v1beta1.WorkloadServiceAccount) {
	projects = make(map[string][]*v1beta1.WorkloadServiceAccount)
	environments = make(map[string][]*v1beta1.WorkloadServiceAccount)
	tenants = make(map[string][]*v1beta1.WorkloadServiceAccount)
	steps = make(map[string][]*v1beta1.WorkloadServiceAccount)
	spaces = make(map[string][]*v1beta1.WorkloadServiceAccount)

	for _, wsa := range wsas {
		processScopeValues(wsa, &projects, ScopeProjects)
		processScopeValues(wsa, &environments, ScopeEnvironments)
		processScopeValues(wsa, &tenants, ScopeTenants)
		processScopeValues(wsa, &steps, ScopeSteps)
		processScopeValues(wsa, &spaces, ScopeSpaces)
	}

	return projects, environments, tenants, steps, spaces
}

func processScopeValues(
	wsa *v1beta1.WorkloadServiceAccount, valueSet *map[string][]*v1beta1.WorkloadServiceAccount, scopeType ScopeType,
) {
	var slice []string
	switch scopeType {
	case ScopeProjects:
		slice = wsa.Spec.Scope.Projects
	case ScopeEnvironments:
		slice = wsa.Spec.Scope.Environments
	case ScopeTenants:
		slice = wsa.Spec.Scope.Tenants
	case ScopeSteps:
		slice = wsa.Spec.Scope.Steps
	case ScopeSpaces:
		slice = wsa.Spec.Scope.Spaces
	default:
		return
	}

	if len(slice) == 0 {
		(*valueSet)["*"] = append((*valueSet)["*"], wsa)
	} else {
		for _, value := range slice {
			(*valueSet)[value] = append((*valueSet)[value], wsa)
		}
	}
}

// generateAllScopeCombinations generates all possible scope combinations
func generateAllScopeCombinations(projects, environments, tenants, steps, spaces map[string]struct{}) []Scope {
	capacity := len(projects) * len(environments) * len(tenants) * len(steps) * len(spaces)
	scopes := make([]Scope, 0, capacity)

	for project := range projects {
		for environment := range environments {
			for tenant := range tenants {
				for step := range steps {
					for space := range spaces {
						scope := Scope{
							Project:     project,
							Environment: environment,
							Tenant:      tenant,
							Step:        step,
							Space:       space,
						}
						scopes = append(scopes, scope)
					}
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
		hasMatchingScopeValue(wsa.Spec.Scope.Steps, scope.Step) &&
		hasMatchingScopeValue(wsa.Spec.Scope.Spaces, scope.Space)
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
