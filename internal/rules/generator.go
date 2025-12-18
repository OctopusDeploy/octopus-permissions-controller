package rules

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"slices"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/hashicorp/go-set/v3"
)

func getScopesForWSAs(wsaList []WSAResource) (map[Scope]map[types.NamespacedName]WSAResource, GlobalVocabulary) {
	vocabulary := buildGlobalVocabulary(wsaList)
	return computeMinimalServiceAccountScopes(wsaList, vocabulary), vocabulary
}

const WildcardValue = "*"

type DimensionIndex int

const (
	ProjectIndex DimensionIndex = iota
	ProjectGroupIndex
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
		set.New[string](0), // ProjectGroups
		set.New[string](0), // Environments
		set.New[string](0), // Tenants
		set.New[string](0), // Steps
		set.New[string](0), // Spaces
	}
}

func (v *GlobalVocabulary) GetKnownScopeCombination(scope Scope) Scope {
	knownScope := Scope{}

	if v[ProjectIndex].Contains(scope.Project) {
		knownScope.Project = scope.Project
	} else {
		knownScope.Project = WildcardValue
	}

	if v[ProjectGroupIndex].Contains(scope.ProjectGroup) {
		knownScope.ProjectGroup = scope.ProjectGroup
	} else {
		knownScope.ProjectGroup = WildcardValue
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
		addAllValuesToVocabulary(ProjectGroupIndex, scope.ProjectGroups)
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
) map[Scope]map[types.NamespacedName]WSAResource {
	if len(wsaList) == 0 {
		return make(map[Scope]map[types.NamespacedName]WSAResource)
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
		scope.ProjectGroups,
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
		case ProjectGroupIndex:
			newScope.ProjectGroup = value
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
) map[Scope]map[types.NamespacedName]WSAResource {
	result := make(map[Scope]map[types.NamespacedName]WSAResource)

	for scope, wsaIndices := range scopeIntersections {
		// Only create service accounts for scopes with at least one WSA
		if wsaIndices.Size() > 0 {
			wsaMap := make(map[types.NamespacedName]WSAResource)
			for _, wsaIndex := range wsaIndices.Slice() {
				resource := wsaList[wsaIndex]
				wsaMap[resource.GetNamespacedName()] = resource
			}
			result[scope] = wsaMap
		}
	}

	return result
}

// GroupedDimensions holds collected dimension values for a group of scopes
type GroupedDimensions struct {
	Projects     []string
	Environments []string
	Tenants      []string
	Steps        []string
	Spaces       []string
}

// groupScopesByWSASet reverses the scope map to group by WSA sets
func groupScopesByWSASet(scopeMap map[Scope]map[types.NamespacedName]WSAResource) map[string][]Scope {
	wsaSetToScopes := make(map[string][]Scope)

	for scope, wsaMap := range scopeMap {
		// Create canonical key for WSA set (sorted by string representation)
		wsaKeys := slices.Collect(maps.Keys(wsaMap))
		slices.SortFunc(wsaKeys, func(a, b types.NamespacedName) int {
			return strings.Compare(a.String(), b.String())
		})
		keyStrings := make([]string, len(wsaKeys))
		for i, k := range wsaKeys {
			keyStrings[i] = k.String()
		}
		wsaSetKey := strings.Join(keyStrings, ",")

		wsaSetToScopes[wsaSetKey] = append(wsaSetToScopes[wsaSetKey], scope)
	}

	return wsaSetToScopes
}

// collectDimensionValues gathers all unique values per dimension
func collectDimensionValues(scopes []Scope) GroupedDimensions {
	projects := set.New[string](0)
	environments := set.New[string](0)
	tenants := set.New[string](0)
	steps := set.New[string](0)
	spaces := set.New[string](0)

	for _, scope := range scopes {
		if scope.Project != "" && scope.Project != WildcardValue {
			projects.Insert(scope.Project)
		}
		if scope.Environment != "" && scope.Environment != WildcardValue {
			environments.Insert(scope.Environment)
		}
		if scope.Tenant != "" && scope.Tenant != WildcardValue {
			tenants.Insert(scope.Tenant)
		}
		if scope.Step != "" && scope.Step != WildcardValue {
			steps.Insert(scope.Step)
		}
		if scope.Space != "" && scope.Space != WildcardValue {
			spaces.Insert(scope.Space)
		}
	}

	// Convert sets to sorted slices
	projectSlice := projects.Slice()
	slices.Sort(projectSlice)
	environmentSlice := environments.Slice()
	slices.Sort(environmentSlice)
	tenantSlice := tenants.Slice()
	slices.Sort(tenantSlice)
	stepSlice := steps.Slice()
	slices.Sort(stepSlice)
	spaceSlice := spaces.Slice()
	slices.Sort(spaceSlice)

	return GroupedDimensions{
		Projects:     projectSlice,
		Environments: environmentSlice,
		Tenants:      tenantSlice,
		Steps:        stepSlice,
		Spaces:       spaceSlice,
	}
}

// GenerateServiceAccountMappings processes the scope map and generates the required mappings
// for service account creation and management.
func GenerateServiceAccountMappings(
	scopeMap map[Scope]map[types.NamespacedName]WSAResource,
) (
	map[Scope]ServiceAccountName,
	map[ServiceAccountName]map[types.NamespacedName]WSAResource,
	map[types.NamespacedName][]string,
	[]*v1.ServiceAccount,
) {
	scopeToServiceAccount := make(map[Scope]ServiceAccountName)
	serviceAccountToWSAs := make(map[ServiceAccountName]map[types.NamespacedName]WSAResource)
	uniqueServiceAccounts := make(map[ServiceAccountName]*v1.ServiceAccount)
	wsaToServiceAccountNamesSet := make(map[types.NamespacedName]*set.Set[string])

	// Group scopes by their WSA sets
	wsaSetToScopes := groupScopesByWSASet(scopeMap)

	// Create one SA per unique WSA set
	for _, scopes := range wsaSetToScopes {
		// Collect all dimension values for this group
		grouped := collectDimensionValues(scopes)

		// Generate ONE service account name for this entire group
		serviceAccountName := generateGroupedServiceAccountName(grouped)

		// Get WSA map from first scope (they're all the same for this group)
		wsaMap := scopeMap[scopes[0]]

		// Map ALL scopes in this group to the SAME service account
		for _, scope := range scopes {
			scopeToServiceAccount[scope] = serviceAccountName
		}

		// Generate service account with grouped dimensions for labels and annotations
		uniqueServiceAccounts[serviceAccountName] =
			generateServiceAccountFromGrouped(serviceAccountName, grouped)

		// Map service account to WSAs
		serviceAccountToWSAs[serviceAccountName] = wsaMap

		// Track WSA to SA mappings
		for wsaName := range wsaMap {
			if wsaSet, ok := wsaToServiceAccountNamesSet[wsaName]; ok {
				wsaSet.Insert(string(serviceAccountName))
			} else {
				newSet := set.New[string](1)
				newSet.Insert(string(serviceAccountName))
				wsaToServiceAccountNamesSet[wsaName] = newSet
			}
		}
	}

	serviceAccountsToCreate := make([]*v1.ServiceAccount, 0, len(uniqueServiceAccounts))
	for _, sa := range uniqueServiceAccounts {
		serviceAccountsToCreate = append(serviceAccountsToCreate, sa)
	}

	// Convert from set to slice for wsaToServiceAccountNames
	wsaToServiceAccountNames := make(map[types.NamespacedName][]string, len(wsaToServiceAccountNamesSet))
	for wsa, wsaSet := range wsaToServiceAccountNamesSet {
		wsaToServiceAccountNames[wsa] = wsaSet.Slice()
	}

	return scopeToServiceAccount, serviceAccountToWSAs, wsaToServiceAccountNames, serviceAccountsToCreate
}

func generateServiceAccountFromGrouped(name ServiceAccountName, grouped GroupedDimensions) *v1.ServiceAccount {
	return &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        string(name),
			Labels:      generateServiceAccountLabelsFromGrouped(grouped),
			Annotations: generateAnnotationsFromGrouped(grouped),
		},
	}
}

// Constants for metadata keys (used in both labels and annotations)
const (
	MetadataNamespace = "agent.octopus.com"
	PermissionsKey    = MetadataNamespace + "/permissions"
	ProjectKey        = MetadataNamespace + "/project"
	ProjectGroupKey   = MetadataNamespace + "/project-group"
	EnvironmentKey    = MetadataNamespace + "/environment"
	TenantKey         = MetadataNamespace + "/tenant"
	StepKey           = MetadataNamespace + "/step"
	SpaceKey          = MetadataNamespace + "/space"
	// ManagedByLabel is the standard Kubernetes label for tracking resource ownership
	ManagedByLabel = "app.kubernetes.io/managed-by"
	// ManagedByValue is the value set on the managed-by label for resources created by this controller
	ManagedByValue = "octopus-permissions-controller"
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
	managedBy, exists := labels[ManagedByLabel]
	return exists && managedBy == ManagedByValue
}

// generateGroupedServiceAccountName creates SA name from grouped dimensions
func generateGroupedServiceAccountName(grouped GroupedDimensions) ServiceAccountName {
	// Build canonical string representation
	parts := []string{}

	if len(grouped.Spaces) > 0 {
		parts = append(parts, "spaces:"+strings.Join(grouped.Spaces, "+"))
	}
	if len(grouped.Projects) > 0 {
		parts = append(parts, "projects:"+strings.Join(grouped.Projects, "+"))
	}
	if len(grouped.Environments) > 0 {
		parts = append(parts, "environments:"+strings.Join(grouped.Environments, "+"))
	}
	if len(grouped.Tenants) > 0 {
		parts = append(parts, "tenants:"+strings.Join(grouped.Tenants, "+"))
	}
	if len(grouped.Steps) > 0 {
		parts = append(parts, "steps:"+strings.Join(grouped.Steps, "+"))
	}

	if len(parts) == 0 {
		parts = append(parts, "wildcard")
	}

	scopeString := strings.Join(parts, "|")
	hash := shortHash(scopeString)

	return ServiceAccountName(fmt.Sprintf("octopus-sa-%s", hash))
}

// generateServiceAccountLabelsFromGrouped generates labels from grouped dimensions
// For multi-valued dimensions, it hashes all values together
func generateServiceAccountLabelsFromGrouped(grouped GroupedDimensions) map[string]string {
	labels := map[string]string{
		ManagedByLabel: ManagedByValue,
	}

	// Hash dimension values for labels to keep them under 63 characters
	if len(grouped.Projects) > 0 {
		labels[ProjectKey] = shortHash(strings.Join(grouped.Projects, "+"))
	}
	if len(grouped.Environments) > 0 {
		labels[EnvironmentKey] = shortHash(strings.Join(grouped.Environments, "+"))
	}
	if len(grouped.Tenants) > 0 {
		labels[TenantKey] = shortHash(strings.Join(grouped.Tenants, "+"))
	}
	if len(grouped.Steps) > 0 {
		labels[StepKey] = shortHash(strings.Join(grouped.Steps, "+"))
	}
	if len(grouped.Spaces) > 0 {
		labels[SpaceKey] = shortHash(strings.Join(grouped.Spaces, "+"))
	}

	return labels
}

// generateAnnotationsFromGrouped generates annotations from grouped dimensions
// For multi-valued dimensions, joins all values with "+"
func generateAnnotationsFromGrouped(grouped GroupedDimensions) map[string]string {
	annotations := make(map[string]string)

	if len(grouped.Projects) > 0 {
		annotations[ProjectKey] = strings.Join(grouped.Projects, "+")
	}
	if len(grouped.Environments) > 0 {
		annotations[EnvironmentKey] = strings.Join(grouped.Environments, "+")
	}
	if len(grouped.Tenants) > 0 {
		annotations[TenantKey] = strings.Join(grouped.Tenants, "+")
	}
	if len(grouped.Steps) > 0 {
		annotations[StepKey] = strings.Join(grouped.Steps, "+")
	}
	if len(grouped.Spaces) > 0 {
		annotations[SpaceKey] = strings.Join(grouped.Spaces, "+")
	}

	return annotations
}
