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

// clusterState is the complete set of mappings derived from the WSA/CWSA resources
// currently in the cluster.
type clusterState struct {
	scopeToSA     map[Scope]ServiceAccountName
	saToWSAMap    map[ServiceAccountName]map[types.NamespacedName]WSAResource
	vocabulary    GlobalVocabulary
	resourceCount int
}

// computeStateFromCluster reads no shared engine state, so callers must not hold the
// lock while calling it.
func (i *InMemoryEngine) computeStateFromCluster(ctx context.Context) (clusterState, error) {
	allResources := make([]WSAResource, 0)

	wsaIter, err := i.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return clusterState{}, fmt.Errorf("failed to list WorkloadServiceAccounts: %w", err)
	}

	for wsa := range wsaIter {
		if wsa.DeletionTimestamp.IsZero() {
			allResources = append(allResources, NewWSAResource(wsa))
		}
	}

	cwsaIter, err := i.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return clusterState{}, fmt.Errorf("failed to list ClusterWorkloadServiceAccounts: %w", err)
	}

	for cwsa := range cwsaIter {
		if cwsa.DeletionTimestamp.IsZero() {
			allResources = append(allResources, NewClusterWSAResource(cwsa))
		}
	}

	scopeComputation := NewScopeComputationService(nil, nil)
	scopeMap, vocabulary := scopeComputation.ComputeScopesForWSAs(allResources)
	scopeToSA, saToWSAMap, _, _ := scopeComputation.GenerateServiceAccountMappings(scopeMap)

	return clusterState{
		scopeToSA:     scopeToSA,
		saToWSAMap:    saToWSAMap,
		vocabulary:    vocabulary,
		resourceCount: len(allResources),
	}, nil
}

// RebuildStateFromCluster reconstructs the in-memory state by querying all WSA/CWSA resources
// from the cluster and recomputing the complete scope mappings
func (i *InMemoryEngine) RebuildStateFromCluster(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("rebuildState")
	logger.Info("Rebuilding state from cluster")

	state, err := i.computeStateFromCluster(ctx)
	if err != nil {
		return err
	}

	logger.Info("Queried all resources from cluster", "totalResources", state.resourceCount)

	// only the swap takes the lock; the webhook holds a read lock on every request
	i.mu.Lock()
	defer i.mu.Unlock()

	oldScopeCount := len(i.scopeToSA)
	oldSACount := len(i.saToWsaMap)

	i.scopeToSA = state.scopeToSA
	i.saToWsaMap = state.saToWSAMap
	i.vocabulary = &state.vocabulary
	i.ScopeComputation = NewScopeComputationService(i.vocabulary, i.scopeToSA)

	logger.Info("State rebuilt from cluster",
		"oldScopeCount", oldScopeCount,
		"newScopeCount", len(i.scopeToSA),
		"oldSACount", oldSACount,
		"newSACount", len(i.saToWsaMap),
		"resourcesProcessed", state.resourceCount)

	return nil
}

// RefreshScopeMappings recomputes only the scope-to-SA state the pod webhook reads,
// for replicas not running reconciliation. See reconciliation.StateSyncer.
//
// Leaves saToWsaMap alone deliberately: handleDeletion treats a WSA's absence from it
// as the signal that staging processed the deletion, so refreshing it here would drop
// finalizers before the leader had run GC.
func (i *InMemoryEngine) RefreshScopeMappings(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("refreshScopeMappings")

	state, err := i.computeStateFromCluster(ctx)
	if err != nil {
		return err
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	oldScopeCount := len(i.scopeToSA)

	i.scopeToSA = state.scopeToSA
	i.vocabulary = &state.vocabulary
	i.ScopeComputation = NewScopeComputationService(i.vocabulary, i.scopeToSA)

	logger.V(1).Info("Scope mappings refreshed",
		"oldScopeCount", oldScopeCount,
		"newScopeCount", len(i.scopeToSA),
		"resourcesProcessed", state.resourceCount)

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
