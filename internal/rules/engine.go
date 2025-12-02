package rules

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	internaltypes "github.com/octopusdeploy/octopus-permissions-controller/internal/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const namespaceCacheKey = "namespaces"

type Scope = internaltypes.Scope

type AgentName string

type Namespace string

type ServiceAccountName string

type Engine interface {
	ResourceManagement
	NamespaceDiscovery
	ScopeComputation
	ApplyBatchPlan(ctx context.Context, plan BatchPlan) error
	CleanupServiceAccounts(ctx context.Context, deletingResource WSAResource) (ctrl.Result, error)
}

type InMemoryEngine struct {
	scopeToSA        map[Scope]ServiceAccountName
	vocabulary       *GlobalVocabulary
	saToWsaMap       map[ServiceAccountName]map[types.NamespacedName]WSAResource
	targetNamespaces []string
	namespaceCache   *expirable.LRU[string, []string]
	client           client.Client
	mu               *sync.RWMutex // Protects in-memory state
	ScopeComputation
	ResourceManagement
	NamespaceDiscovery
}

func NewInMemoryEngine(
	controllerClient client.Client,
	scheme *runtime.Scheme,
	targetNamespaceRegex *regexp.Regexp,
	namespaceCacheTTL time.Duration,
) InMemoryEngine {
	vocab := NewGlobalVocabulary()
	engine := InMemoryEngine{
		scopeToSA:          make(map[Scope]ServiceAccountName),
		targetNamespaces:   []string{},
		namespaceCache:     expirable.NewLRU[string, []string](1, nil, namespaceCacheTTL),
		client:             controllerClient,
		vocabulary:         &vocab,
		saToWsaMap:         make(map[ServiceAccountName]map[types.NamespacedName]WSAResource),
		mu:                 &sync.RWMutex{},
		ResourceManagement: NewResourceManagementServiceWithScheme(controllerClient, scheme),
		NamespaceDiscovery: NamespaceDiscoveryService{TargetNamespaceRegex: targetNamespaceRegex},
	}
	engine.ScopeComputation = NewScopeComputationService(engine.vocabulary, engine.scopeToSA)
	return engine
}

func NewInMemoryEngineWithNamespaces(
	controllerClient client.Client, scheme *runtime.Scheme, targetNamespaces []string,
) InMemoryEngine {
	vocab := NewGlobalVocabulary()
	engine := InMemoryEngine{
		scopeToSA:          make(map[Scope]ServiceAccountName),
		targetNamespaces:   targetNamespaces,
		namespaceCache:     nil,
		client:             controllerClient,
		vocabulary:         &vocab,
		saToWsaMap:         make(map[ServiceAccountName]map[types.NamespacedName]WSAResource),
		mu:                 &sync.RWMutex{},
		ResourceManagement: NewResourceManagementServiceWithScheme(controllerClient, scheme),
		NamespaceDiscovery: NamespaceDiscoveryService{},
	}
	engine.ScopeComputation = NewScopeComputationService(engine.vocabulary, engine.scopeToSA)
	return engine
}

func (i *InMemoryEngine) GetTargetNamespaces() []string {
	return i.targetNamespaces
}

func (i *InMemoryEngine) SetGCTracker(tracker GCTrackerInterface) {
	if rms, ok := i.ResourceManagement.(interface{ SetGCTracker(GCTrackerInterface) }); ok {
		rms.SetGCTracker(tracker)
	}
}

func (i *InMemoryEngine) GetOrDiscoverTargetNamespaces(ctx context.Context) ([]string, error) {
	// Set as nil to skip caching if target namespaces are explicitly provided
	if i.namespaceCache == nil {
		return i.targetNamespaces, nil
	}

	if namespaces, ok := i.namespaceCache.Get(namespaceCacheKey); ok {
		return namespaces, nil
	}

	namespaces, err := i.DiscoverTargetNamespaces(ctx, i.client)
	if err != nil {
		return nil, fmt.Errorf("failed to discover target namespaces: %w", err)
	}

	i.namespaceCache.Add(namespaceCacheKey, namespaces)
	return namespaces, nil
}

type BatchPlan interface {
	GetScopeToSA() map[Scope]ServiceAccountName
	GetSAToWSAMap() map[ServiceAccountName]map[types.NamespacedName]WSAResource
	GetVocabulary() *GlobalVocabulary
}

func (i *InMemoryEngine) ApplyBatchPlan(ctx context.Context, plan BatchPlan) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	logger := log.FromContext(ctx).WithName("applyBatchPlan")

	planScopeToSA := plan.GetScopeToSA()
	planSAToWSAMap := plan.GetSAToWSAMap()
	planVocab := plan.GetVocabulary()

	oldScopeCount := len(i.scopeToSA)
	oldSACount := len(i.saToWsaMap)

	i.scopeToSA = planScopeToSA
	i.saToWsaMap = planSAToWSAMap
	if planVocab != nil {
		i.vocabulary = planVocab
	}

	i.ScopeComputation = NewScopeComputationService(i.vocabulary, i.scopeToSA)

	logger.V(1).Info("Applied batch plan",
		"oldScopeCount", oldScopeCount,
		"newScopeCount", len(i.scopeToSA),
		"oldSACount", oldSACount,
		"newSACount", len(i.saToWsaMap))

	return nil
}

// RebuildStateFromCluster reconstructs the in-memory state by querying all WSA/CWSA resources
// from the cluster and recomputing the complete scope mappings. This is useful for:
// - Controller initialization/startup
// - Recovery from state corruption
// - Debugging state inconsistencies
func (i *InMemoryEngine) RebuildStateFromCluster(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	logger := log.FromContext(ctx).WithName("rebuildState")
	logger.Info("Rebuilding state from cluster")

	allResources := make([]WSAResource, 0)

	wsaIter, err := i.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to list WorkloadServiceAccounts: %w", err)
	}

	for wsa := range wsaIter {
		if wsa.DeletionTimestamp.IsZero() {
			allResources = append(allResources, NewWSAResource(wsa))
		}
	}

	cwsaIter, err := i.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to list ClusterWorkloadServiceAccounts: %w", err)
	}

	for cwsa := range cwsaIter {
		if cwsa.DeletionTimestamp.IsZero() {
			allResources = append(allResources, NewClusterWSAResource(cwsa))
		}
	}

	logger.Info("Queried all resources from cluster", "totalResources", len(allResources))

	scopeComputation := NewScopeComputationService(nil, nil)
	scopeMap, vocabulary := scopeComputation.ComputeScopesForWSAs(allResources)

	scopeToSA, saToWSAMap, _, _ := scopeComputation.GenerateServiceAccountMappings(scopeMap)

	oldScopeCount := len(i.scopeToSA)
	oldSACount := len(i.saToWsaMap)

	i.scopeToSA = scopeToSA
	i.saToWsaMap = saToWSAMap
	i.vocabulary = &vocabulary
	i.ScopeComputation = NewScopeComputationService(i.vocabulary, i.scopeToSA)

	logger.Info("State rebuilt from cluster",
		"oldScopeCount", oldScopeCount,
		"newScopeCount", len(i.scopeToSA),
		"oldSACount", oldSACount,
		"newSACount", len(i.saToWsaMap),
		"resourcesProcessed", len(allResources))

	return nil
}

// GetServiceAccountForScope retrieves the service account for a given scope with proper locking.
// This method shadows the embedded ScopeComputation.GetServiceAccountForScope to ensure
// thread-safe access to the in-memory maps.
func (i *InMemoryEngine) GetServiceAccountForScope(scope Scope) (ServiceAccountName, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.ScopeComputation.GetServiceAccountForScope(scope)
}

// IsWSAInMaps checks if a WSA is still present in the in-memory state.
// This is used to determine if staging has processed a deletion event.
func (i *InMemoryEngine) IsWSAInMaps(wsaKey types.NamespacedName) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, wsaMap := range i.saToWsaMap {
		if _, exists := wsaMap[wsaKey]; exists {
			return true
		}
	}
	return false
}

// CleanupServiceAccounts is called when a WSA/cWSA is being deleted.
// It no longer performs GC directly - staging handles all GC to avoid race conditions
// between concurrent cleanup and staging GC runs.
// This function just logs the cleanup request and returns immediately.
// The staging batch (which includes deleting resources) will handle eventual cleanup
// once the resource is fully removed from the API server.
func (i *InMemoryEngine) CleanupServiceAccounts(
	ctx context.Context, deletingResource WSAResource,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("cleanupServiceAccounts")

	logger.Info("Resource deletion acknowledged, staging will handle cleanup",
		"resource", deletingResource.GetNamespacedName().String(),
		"isClusterScoped", deletingResource.IsClusterScoped())

	return ctrl.Result{}, nil
}
