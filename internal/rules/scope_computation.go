package rules

import (
	corev1 "k8s.io/api/core/v1"
)

// ScopeComputation defines the interface for computing scopes and service account mappings
type ScopeComputation interface {
	GetServiceAccountForScope(scope Scope) (ServiceAccountName, error)
	ComputeScopesForWSAs(wsaList []WSAResource) (map[Scope]map[string]WSAResource, GlobalVocabulary)
	GenerateServiceAccountMappings(scopeMap map[Scope]map[string]WSAResource) (
		map[Scope]ServiceAccountName,
		map[ServiceAccountName]map[string]WSAResource,
		map[string][]string,
		[]*corev1.ServiceAccount,
	)
	GetScopeToSA() map[Scope]ServiceAccountName
}

type ScopeComputationService struct {
	vocabulary *GlobalVocabulary
	scopeToSA  map[Scope]ServiceAccountName
}

func NewScopeComputationService(
	vocabulary *GlobalVocabulary, scopeToSA map[Scope]ServiceAccountName,
) ScopeComputationService {
	return ScopeComputationService{
		vocabulary: vocabulary,
		scopeToSA:  scopeToSA,
	}
}

func (s ScopeComputationService) GetServiceAccountForScope(scope Scope) (ServiceAccountName, error) {
	knownScope := (*s.vocabulary).GetKnownScopeCombination(scope)
	if sa, ok := (s.scopeToSA)[knownScope]; ok {
		return sa, nil
	}

	return "", nil
}

func (s ScopeComputationService) ComputeScopesForWSAs(wsaList []WSAResource) (map[Scope]map[string]WSAResource, GlobalVocabulary) {
	// Build global vocabulary of all possible scope values
	vocabulary := buildGlobalVocabulary(wsaList)

	// Use set theory to compute minimal service accounts needed
	// Only create service accounts where multiple WSAs could apply to the same scope
	return computeMinimalServiceAccountScopes(wsaList, vocabulary), vocabulary
}

func (s ScopeComputationService) GenerateServiceAccountMappings(scopeMap map[Scope]map[string]WSAResource) (map[Scope]ServiceAccountName, map[ServiceAccountName]map[string]WSAResource, map[string][]string, []*corev1.ServiceAccount) {
	// Delegate to the standalone function which has the grouped implementation
	return GenerateServiceAccountMappings(scopeMap)
}

// GetScopeToSA returns the current scope to service account mapping
func (s ScopeComputationService) GetScopeToSA() map[Scope]ServiceAccountName {
	if s.scopeToSA == nil {
		return make(map[Scope]ServiceAccountName)
	}
	return s.scopeToSA
}
