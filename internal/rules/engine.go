package rules

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/types"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Scope = types.Scope

type AgentName string

type Namespace string

type ServiceAccountName string

type Engine interface {
	ResourceManagement
	NamespaceDiscovery
	ScopeComputation
	ApplyBatchPlan(ctx context.Context, plan interface{}) error
	CleanupServiceAccounts(ctx context.Context, deletingResource WSAResource) (ctrl.Result, error)
}

type InMemoryEngine struct {
	scopeToSA        map[Scope]ServiceAccountName
	vocabulary       *GlobalVocabulary
	saToWsaMap       map[ServiceAccountName]map[string]WSAResource
	targetNamespaces []string
	lookupNamespaces bool
	client           client.Client
	mu               *sync.RWMutex // Protects in-memory state
	ScopeComputation
	ResourceManagement
	NamespaceDiscovery
}

func NewInMemoryEngine(
	controllerClient client.Client, scheme *runtime.Scheme, targetNamespaceRegex *regexp.Regexp,
) InMemoryEngine {
	vocab := NewGlobalVocabulary()
	engine := InMemoryEngine{
		scopeToSA:          make(map[Scope]ServiceAccountName),
		targetNamespaces:   []string{},
		lookupNamespaces:   true,
		client:             controllerClient,
		vocabulary:         &vocab,
		saToWsaMap:         make(map[ServiceAccountName]map[string]WSAResource),
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
		lookupNamespaces:   len(targetNamespaces) == 0,
		client:             controllerClient,
		vocabulary:         &vocab,
		saToWsaMap:         make(map[ServiceAccountName]map[string]WSAResource),
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

func (i *InMemoryEngine) GetOrDiscoverTargetNamespaces(ctx context.Context) ([]string, error) {
	i.mu.RLock()
	targetNamespaces := i.targetNamespaces
	lookupNamespaces := i.lookupNamespaces
	i.mu.RUnlock()

	if len(targetNamespaces) > 0 {
		return targetNamespaces, nil
	}

	if lookupNamespaces {
		namespaces, err := i.DiscoverTargetNamespaces(ctx, i.client)
		if err != nil {
			return nil, fmt.Errorf("failed to discover target namespaces: %w", err)
		}
		return namespaces, nil
	}

	return targetNamespaces, nil
}

func (i *InMemoryEngine) ApplyBatchPlan(ctx context.Context, plan interface{}) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	logger := log.FromContext(ctx).WithName("applyBatchPlan")

	type reconciliationPlan interface {
		GetScopeToSA() map[Scope]ServiceAccountName
		GetSAToWSAMap() map[ServiceAccountName]map[string]WSAResource
		GetVocabulary() *GlobalVocabulary
	}

	rp, ok := plan.(reconciliationPlan)
	if !ok {
		return fmt.Errorf("invalid plan type")
	}

	planScopeToSA := rp.GetScopeToSA()
	planSAToWSAMap := rp.GetSAToWSAMap()
	planVocab := rp.GetVocabulary()

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
// The wsaKey should be a namespaced name string (namespace/name or just name for cluster-scoped).
func (i *InMemoryEngine) IsWSAInMaps(wsaKey string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, wsaMap := range i.saToWsaMap {
		for key := range wsaMap {
			if key == wsaKey {
				return true
			}
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
		"name", deletingResource.GetName(),
		"namespace", deletingResource.GetNamespace(),
		"isClusterScoped", deletingResource.IsClusterScoped())

	return ctrl.Result{}, nil
}
