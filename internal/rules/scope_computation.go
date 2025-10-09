package rules

import (
	"maps"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"

	"github.com/hashicorp/go-set/v3"
)

// ScopeComputation defines the interface for computing scopes and service account mappings
type ScopeComputation interface {
	GetServiceAccountForScope(scope Scope) (ServiceAccountName, error)
	ComputeScopesForWSAs(wsaList []*v1beta1.WorkloadServiceAccount) (map[Scope]map[string]*v1beta1.WorkloadServiceAccount, GlobalVocabulary)
	GenerateServiceAccountMappings(scopeMap map[Scope]map[string]*v1beta1.WorkloadServiceAccount) (
		map[Scope]ServiceAccountName,
		map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount,
		map[string][]string,
		[]*corev1.ServiceAccount,
	)
}

type ScopeComputationService struct {
	vocabulary *GlobalVocabulary
	scopeToSA  *map[Scope]ServiceAccountName
}

func NewScopeComputationService(vocabulary *GlobalVocabulary, scopeToSA *map[Scope]ServiceAccountName) ScopeComputationService {
	return ScopeComputationService{
		vocabulary: vocabulary,
		scopeToSA:  scopeToSA,
	}
}

func (s ScopeComputationService) GetServiceAccountForScope(scope Scope) (ServiceAccountName, error) {
	knownScope := (*s.vocabulary).GetKnownScopeCombination(scope)
	if sa, ok := (*s.scopeToSA)[knownScope]; ok {
		return sa, nil
	}

	return "", nil
}

func (s ScopeComputationService) ComputeScopesForWSAs(wsaList []*v1beta1.WorkloadServiceAccount) (map[Scope]map[string]*v1beta1.WorkloadServiceAccount, GlobalVocabulary) {
	// Build global vocabulary of all possible scope values
	vocabulary := buildGlobalVocabulary(wsaList)

	// Use set theory to compute minimal service accounts needed
	// Only create service accounts where multiple WSAs could apply to the same scope
	return computeMinimalServiceAccountScopes(wsaList, vocabulary), vocabulary
}

func (s ScopeComputationService) GenerateServiceAccountMappings(scopeMap map[Scope]map[string]*v1beta1.WorkloadServiceAccount) (map[Scope]ServiceAccountName, map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount, map[string][]string, []*corev1.ServiceAccount) {
	scopeToServiceAccount := make(map[Scope]ServiceAccountName)
	serviceAccountToWSAs := make(map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount)
	uniqueServiceAccounts := make(map[ServiceAccountName]*corev1.ServiceAccount)
	wsaToServiceAccountNamesSet := make(map[string]*set.Set[string])

	// Process each scope and its associated WSAs
	for scope, wsaMap := range scopeMap {
		// Generate service account name for this WSA
		wsaKeys := maps.Keys(wsaMap)
		serviceAccountName := generateServiceAccountName(wsaKeys)

		// Map scope to service account name
		scopeToServiceAccount[scope] = serviceAccountName

		// Track this service account as needing to be created
		uniqueServiceAccounts[serviceAccountName] = generateServiceAccount(serviceAccountName, scope)

		// Map service account to WSA names
		if existing, exists := serviceAccountToWSAs[serviceAccountName]; exists {
			maps.Copy(existing, wsaMap)
		} else {
			serviceAccountToWSAs[serviceAccountName] = wsaMap
		}

		// For each WSA, track which service accounts it maps to
		for wsaName := range wsaMap {
			if wsaSet, ok := wsaToServiceAccountNamesSet[wsaName]; ok {
				wsaSet.Insert(string(serviceAccountName))
				continue
			}
			newSet := set.New[string](1)
			newSet.Insert(string(serviceAccountName))
			wsaToServiceAccountNamesSet[wsaName] = newSet
		}
	}

	serviceAccountsToCreate := make([]*corev1.ServiceAccount, 0, len(uniqueServiceAccounts))
	for _, sa := range uniqueServiceAccounts {
		serviceAccountsToCreate = append(serviceAccountsToCreate, sa)
	}

	// Convert from set to slice for wsaToServiceAccountNames
	wsaToServiceAccountNames := make(map[string][]string, len(wsaToServiceAccountNamesSet))
	for wsa, wsaSet := range wsaToServiceAccountNamesSet {
		wsaToServiceAccountNames[wsa] = wsaSet.Slice()
	}

	return scopeToServiceAccount, serviceAccountToWSAs, wsaToServiceAccountNames, serviceAccountsToCreate
}
