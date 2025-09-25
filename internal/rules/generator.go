package rules

import (
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

type ScopeType int

const (
	ScopeProjects ScopeType = iota
	ScopeEnvironments
	ScopeTenants
	ScopeSteps
	ScopeSpaces
)

func getScopesForWSAs(wsaList []v1beta1.WorkloadServiceAccount) map[Scope]map[string]v1beta1.WorkloadServiceAccount {
	return map[Scope]map[string]v1beta1.WorkloadServiceAccount{
		Scope{Space: "*", Project: "Project1", Environment: "*", Tenant: "*", Step: "*"}: {
			"wsa-1": wsaList[0],
		},
	}

}

// getAllScopeValues extracts all unique scope values from all WSAs
func getAllScopeValues(wsaList []v1beta1.WorkloadServiceAccount) (projects, environments, tenants, steps, spaces map[string][]v1beta1.WorkloadServiceAccount) {
	projects = make(map[string][]v1beta1.WorkloadServiceAccount)
	environments = make(map[string][]v1beta1.WorkloadServiceAccount)
	tenants = make(map[string][]v1beta1.WorkloadServiceAccount)
	steps = make(map[string][]v1beta1.WorkloadServiceAccount)
	spaces = make(map[string][]v1beta1.WorkloadServiceAccount)

	for _, wsa := range wsaList {
		processScopeValues(wsa, &projects, ScopeProjects)
		processScopeValues(wsa, &environments, ScopeEnvironments)
		processScopeValues(wsa, &tenants, ScopeTenants)
		processScopeValues(wsa, &steps, ScopeSteps)
		processScopeValues(wsa, &spaces, ScopeSpaces)
	}

	return projects, environments, tenants, steps, spaces
}

// processScopeValues is a helper to collect scope values of a given scope type from a WSA
func processScopeValues(wsa v1beta1.WorkloadServiceAccount, valueSet *map[string][]v1beta1.WorkloadServiceAccount, scopeType ScopeType) {
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

// generateAllScopeCombinations generates all possible scope combinations from the sets of scope values
func generateAllScopeCombinations(projects, environments, tenants, steps, spaces map[string][]v1beta1.WorkloadServiceAccount) map[Scope]map[string]v1beta1.WorkloadServiceAccount {
	capacity := len(projects) * len(environments) * len(tenants) * len(spaces) * len(spaces)
	scopeToWSAs := make(map[Scope]map[string]v1beta1.WorkloadServiceAccount, capacity)

	for project, projectWSAs := range projects {
		for environment, environmentWSAs := range environments {
			for tenant, tenantWSAs := range tenants {
				for step, stepWSAs := range steps {
					for space, spaceWSAs := range spaces {
						scope := Scope{
							Project:     project,
							Environment: environment,
							Tenant:      tenant,
							Step:        step,
							Space:       space,
						}

						uniqueWSAMap := make(map[string]v1beta1.WorkloadServiceAccount)

						for _, wsa := range projectWSAs {
							uniqueWSAMap[wsa.Name] = wsa
						}
						for _, wsa := range environmentWSAs {
							uniqueWSAMap[wsa.Name] = wsa
						}
						for _, wsa := range tenantWSAs {
							uniqueWSAMap[wsa.Name] = wsa
						}
						for _, wsa := range stepWSAs {
							uniqueWSAMap[wsa.Name] = wsa
						}
						for _, wsa := range spaceWSAs {
							uniqueWSAMap[wsa.Name] = wsa
						}

						if len(uniqueWSAMap) > 0 {
							scopeToWSAs[scope] = uniqueWSAMap
						}
					}
				}
			}
		}
	}

	return scopeToWSAs
}
