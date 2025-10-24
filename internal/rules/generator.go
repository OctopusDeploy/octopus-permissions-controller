package rules

import (
	"crypto/sha256"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hashicorp/go-set/v3"
)

func getScopesForWSAs(wsaList []WSAResource) (map[Scope]map[string]WSAResource, GlobalVocabulary) {
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

func buildGlobalVocabulary(resources []WSAResource) GlobalVocabulary {
	vocabulary := NewGlobalVocabulary()

	for _, resource := range resources {
		scope := resource.GetScope()

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
	wsaList []WSAResource, vocabulary GlobalVocabulary,
) map[Scope]map[string]WSAResource {
	if len(wsaList) == 0 {
		return make(map[Scope]map[string]WSAResource)
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
func computeWSACoverage(resource WSAResource, vocabulary GlobalVocabulary) *set.Set[Scope] {
	// For each dimension, determine the set of values this WSA covers
	dimensionCoverage := [MaxDimensionIndex]*set.Set[string]{}

	scope := resource.GetScope()
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
	scopeIntersections map[Scope]*set.Set[int], wsaList []WSAResource,
) map[Scope]map[string]WSAResource {
	result := make(map[Scope]map[string]WSAResource)

	for scope, wsaIndices := range scopeIntersections {
		// Only create service accounts for scopes with at least one WSA
		if wsaIndices.Size() > 0 {
			wsaMap := make(map[string]WSAResource)
			for _, wsaIndex := range wsaIndices.Slice() {
				resource := wsaList[wsaIndex]
				wsaMap[resource.GetName()] = resource
			}
			result[scope] = wsaMap
		}
	}

	return result
}

// GenerateServiceAccountMappings processes the scope map and generates the required mappings
// for service account creation and management.
func GenerateServiceAccountMappings(
	scopeMap map[Scope]map[string]WSAResource,
) (
	map[Scope]ServiceAccountName,
	map[ServiceAccountName]map[string]WSAResource,
	map[string][]string,
	[]*v1.ServiceAccount,
) {
	scopeToServiceAccount := make(map[Scope]ServiceAccountName)
	serviceAccountToWSAs := make(map[ServiceAccountName]map[string]WSAResource)
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

// Constants for metadata keys (used in both labels and annotations)
const (
	MetadataNamespace = "agent.octopus.com"
	PermissionsKey    = MetadataNamespace + "/permissions"
	ProjectKey        = MetadataNamespace + "/project"
	EnvironmentKey    = MetadataNamespace + "/environment"
	TenantKey         = MetadataNamespace + "/tenant"
	StepKey           = MetadataNamespace + "/step"
	SpaceKey          = MetadataNamespace + "/space"
)

// shortHash generates a hash of a string for use in labels and names
func shortHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", hash)[:32] // Use first 32 characters (128 bits)
}

// IsOctopusManaged checks if a resource is managed by the Octopus controller
func IsOctopusManaged(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	permissions, exists := labels[PermissionsKey]
	return exists && permissions == "enabled"
}

// generateServiceAccountName generates a ServiceAccountName based on the given scope
func generateServiceAccountName(wsaNames iter.Seq[string]) ServiceAccountName {
	hash := shortHash(strings.Join(slices.Collect(wsaNames), "-"))
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
