package rules

import (
	"maps"
	"slices"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

func doScopesIntersect(scopeValues1, scopeValues2 []string) bool {
	if len(scopeValues1) == 0 || len(scopeValues2) == 0 {
		return true // Empty scope means wildcard, which intersects with everything
	}

	for _, val1 := range scopeValues1 {
		for _, val2 := range scopeValues2 {
			if val1 == val2 {
				return true
			}
		}
	}

	return false
}

func doWSAsIntersect(wsa1, wsa2 v1beta1.WorkloadServiceAccountScope) bool {
	return doScopesIntersect(wsa1.Projects, wsa2.Projects) &&
		doScopesIntersect(wsa1.Environments, wsa2.Environments) &&
		doScopesIntersect(wsa1.Tenants, wsa2.Tenants) &&
		doScopesIntersect(wsa1.Steps, wsa2.Steps) &&
		doScopesIntersect(wsa1.Spaces, wsa2.Spaces)
}

func getScopes(wsaList []v1beta1.WorkloadServiceAccount) []string {
	var scopeIntersection []string

	for i, wsa := range wsaList {
		scopeIntersection = append(scopeIntersection, wsa.Name)
		scopeIntersection = append(scopeIntersection, getScopesRecursive(wsaList, i+1, wsa.Spec.Scope, wsa.Name)...)
	}
	return scopeIntersection
}

func getScopesRecursive(wsaList []v1beta1.WorkloadServiceAccount, start int, wsaScope v1beta1.WorkloadServiceAccountScope, mergedName string) []string {
	var scopeIntersection []string

	for i := start; i < len(wsaList); i++ {
		if doWSAsIntersect(wsaList[i].Spec.Scope, wsaScope) {
			scopeIntersection = append(scopeIntersection, mergedName+wsaList[i].Name)
			mergedScope := mergeWSAScopes(wsaList[i].Spec.Scope, wsaScope)

			scopeIntersection = append(scopeIntersection, getScopesRecursive(wsaList, i+1, mergedScope, mergedName+wsaList[i].Name)...)
		}
	}
	return scopeIntersection
}

func mergeWSAScopes(wsa1, wsa2 v1beta1.WorkloadServiceAccountScope) v1beta1.WorkloadServiceAccountScope {
	return v1beta1.WorkloadServiceAccountScope{
		Projects:     mergeScope(wsa1.Projects, wsa2.Projects),
		Environments: mergeScope(wsa1.Environments, wsa2.Environments),
		Tenants:      mergeScope(wsa1.Tenants, wsa2.Tenants),
		Steps:        mergeScope(wsa1.Steps, wsa2.Steps),
		Spaces:       mergeScope(wsa1.Spaces, wsa2.Spaces),
	}
}

func mergeScope(scope1, scope2 []string) []string {
	scopeMap := make(map[string]struct{})
	for _, s := range scope1 {
		scopeMap[s] = struct{}{}
	}
	for _, s := range scope2 {
		scopeMap[s] = struct{}{}
	}

	return slices.Collect(maps.Keys(scopeMap))
}
