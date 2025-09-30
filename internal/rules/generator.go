package rules

import (
	"maps"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hashicorp/go-set/v3"
)

func getScopesForWSAs(wsaList []*v1beta1.WorkloadServiceAccount) (map[Scope]map[string]*v1beta1.WorkloadServiceAccount, GlobalVocabulary) {
	// Build global vocabulary of all possible scope values
	vocabulary := buildGlobalVocabulary(wsaList)

	// Use set theory to compute minimal service accounts needed
	// Only create service accounts where multiple WSAs could apply to the same scope
	return computeMinimalServiceAccountScopes(wsaList, vocabulary), vocabulary
}

const WildcardValue = "*"

type DimensionIndex int

const (
	ProjectIndex DimensionIndex = iota
	EnvironmentIndex
	TenantIndex
	StepIndex
	SpaceIndex
	MaxDimensionIndex // Must be last - used for various looping through dimensions
)

// GlobalVocabulary holds the set of known values for each dimension
// Indexes correspond to DimensionIndex constants
// e.g. [ProjectIndex] holds the set of known projects from all WSAs
type GlobalVocabulary [MaxDimensionIndex]*set.Set[string]

func NewGlobalVocabulary() GlobalVocabulary {
	return GlobalVocabulary{
		set.New[string](0), // Projects
		set.New[string](0), // Environments
		set.New[string](0), // Tenants
		set.New[string](0), // Steps
		set.New[string](0), // Spaces
	}
}

func (v *GlobalVocabulary) GetKnownScopeCombination(scope Scope) Scope {
	// For each dimension, if the value is known, keep it; otherwise, set to wildcard
	knownScope := Scope{}

	if v[ProjectIndex].Contains(scope.Project) {
		knownScope.Project = scope.Project
	} else {
		knownScope.Project = WildcardValue
	}

	if v[EnvironmentIndex].Contains(scope.Environment) {
		knownScope.Environment = scope.Environment
	} else {
		knownScope.Environment = WildcardValue
	}

	if v[TenantIndex].Contains(scope.Tenant) {
		knownScope.Tenant = scope.Tenant
	} else {
		knownScope.Tenant = WildcardValue
	}

	if v[StepIndex].Contains(scope.Step) {
		knownScope.Step = scope.Step
	} else {
		knownScope.Step = WildcardValue
	}

	if v[SpaceIndex].Contains(scope.Space) {
		knownScope.Space = scope.Space
	} else {
		knownScope.Space = WildcardValue
	}

	return knownScope
}

func buildGlobalVocabulary(rules []*v1beta1.WorkloadServiceAccount) GlobalVocabulary {
	vocabulary := NewGlobalVocabulary()

	for _, rule := range rules {
		scope := rule.Spec.Scope

		// Add all values from each dimension to our vocabulary
		addAllValuesToVocabulary := func(dimensionIndex DimensionIndex, values []string) {
			for _, value := range values {
				vocabulary[dimensionIndex].Insert(value)
			}
		}

		addAllValuesToVocabulary(ProjectIndex, scope.Projects)
		addAllValuesToVocabulary(EnvironmentIndex, scope.Environments)
		addAllValuesToVocabulary(TenantIndex, scope.Tenants)
		addAllValuesToVocabulary(StepIndex, scope.Steps)
		addAllValuesToVocabulary(SpaceIndex, scope.Spaces)
	}

	return vocabulary
}

// computeMinimalServiceAccountScopes uses set theory to compute the minimal set of service accounts needed
// It creates service accounts only for scope intersections where multiple WSAs could apply
func computeMinimalServiceAccountScopes(
	wsaList []*v1beta1.WorkloadServiceAccount, vocabulary GlobalVocabulary,
) map[Scope]map[string]*v1beta1.WorkloadServiceAccount {
	if len(wsaList) == 0 {
		return make(map[Scope]map[string]*v1beta1.WorkloadServiceAccount)
	}

	// Create sets for each WSA representing all scopes it covers
	wsaCoverageSets := make([]*set.Set[Scope], len(wsaList))
	for i, wsa := range wsaList {
		wsaCoverageSets[i] = computeWSACoverage(wsa, vocabulary)
	}

	// Find all scope intersections where multiple WSAs overlap
	scopeIntersections := findScopeIntersections(wsaCoverageSets)

	// Build final mapping of scopes to WSAs
	return buildScopeToWSAMapping(scopeIntersections, wsaList)
}

// computeWSACoverage calculates all scopes that a WSA covers using set operations
func computeWSACoverage(wsa *v1beta1.WorkloadServiceAccount, vocabulary GlobalVocabulary) *set.Set[Scope] {
	// For each dimension, determine the set of values this WSA covers
	dimensionCoverage := [MaxDimensionIndex]*set.Set[string]{}

	scope := wsa.Spec.Scope
	scopeSlices := [MaxDimensionIndex][]string{
		scope.Projects,
		scope.Environments,
		scope.Tenants,
		scope.Steps,
		scope.Spaces,
	}

	for dim := ProjectIndex; dim < MaxDimensionIndex; dim++ {
		if len(scopeSlices[dim]) == 0 {
			// Empty scope means wildcard - covers all possible values in this dimension
			dimensionCoverage[dim] = vocabulary[dim].Copy()
			dimensionCoverage[dim].Insert(WildcardValue)
		} else {
			// Constrained scope - only covers specified values
			dimensionCoverage[dim] = set.From(scopeSlices[dim])
		}
	}

	// Generate cartesian product of all dimension coverages to get all scopes covered
	coverageSet := set.New[Scope](0)
	generateScopeCombinations(0, Scope{}, dimensionCoverage, coverageSet)

	return coverageSet
}

// generateScopeCombinations recursively generates all combinations of scope values
func generateScopeCombinations(
	currentDim DimensionIndex, currentScope Scope, dimensionSets [MaxDimensionIndex]*set.Set[string],
	result *set.Set[Scope],
) {
	if currentDim == MaxDimensionIndex {
		result.Insert(currentScope)
		return
	}

	for _, value := range dimensionSets[currentDim].Slice() {
		newScope := currentScope
		switch currentDim {
		case ProjectIndex:
			newScope.Project = value
		case EnvironmentIndex:
			newScope.Environment = value
		case TenantIndex:
			newScope.Tenant = value
		case StepIndex:
			newScope.Step = value
		case SpaceIndex:
			newScope.Space = value
		}
		generateScopeCombinations(currentDim+1, newScope, dimensionSets, result)
	}
}

// findScopeIntersections finds all scopes where multiple WSAs could apply
func findScopeIntersections(coverageSets []*set.Set[Scope]) map[Scope]*set.Set[int] {
	scopeToWSAIndices := make(map[Scope]*set.Set[int])

	// For each scope covered by any WSA, track which WSAs cover it
	for wsaIndex, coverageSet := range coverageSets {
		for _, scope := range coverageSet.Slice() {
			if scopeToWSAIndices[scope] == nil {
				scopeToWSAIndices[scope] = set.New[int](0)
			}
			scopeToWSAIndices[scope].Insert(wsaIndex)
		}
	}

	return scopeToWSAIndices
}

// buildScopeToWSAMapping builds the final mapping from intersections
func buildScopeToWSAMapping(
	scopeIntersections map[Scope]*set.Set[int], wsaList []*v1beta1.WorkloadServiceAccount,
) map[Scope]map[string]*v1beta1.WorkloadServiceAccount {
	result := make(map[Scope]map[string]*v1beta1.WorkloadServiceAccount)

	for scope, wsaIndices := range scopeIntersections {
		// Only create service accounts for scopes with at least one WSA
		if wsaIndices.Size() > 0 {
			wsaMap := make(map[string]*v1beta1.WorkloadServiceAccount)
			for _, wsaIndex := range wsaIndices.Slice() {
				wsa := wsaList[wsaIndex]
				wsaMap[wsa.Name] = wsa
			}
			result[scope] = wsaMap
		}
	}

	return result
}

// GenerateServiceAccountMappings processes the scope map and generates the required mappings
// for service account creation and management.
func GenerateServiceAccountMappings(
	scopeMap map[Scope]map[string]*v1beta1.WorkloadServiceAccount,
) (
	map[Scope]ServiceAccountName,
	map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount,
	map[string][]string,
	[]*v1.ServiceAccount,
) {
	scopeToServiceAccount := make(map[Scope]ServiceAccountName)
	serviceAccountToWSAs := make(map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount)
	uniqueServiceAccounts := make(map[ServiceAccountName]*v1.ServiceAccount)
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

	serviceAccountsToCreate := make([]*v1.ServiceAccount, 0, len(uniqueServiceAccounts))
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

func generateServiceAccount(name ServiceAccountName, scope Scope) *v1.ServiceAccount {
	return &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        string(name),
			Labels:      generateServiceAccountLabels(scope),
			Annotations: generateExpectedAnnotations(scope),
		},
	}
}
