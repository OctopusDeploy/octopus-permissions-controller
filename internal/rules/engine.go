package rules

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/hashicorp/go-set/v3"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/types"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
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
	scopeToSA           map[Scope]ServiceAccountName
	vocabulary          *GlobalVocabulary
	saToWsaMap          map[ServiceAccountName]map[string]WSAResource
	deletingSAs         map[ServiceAccountName]bool // Tracks SAs marked for deletion (with deletionTimestamp)
	targetNamespaces    []string
	lookupNamespaces    bool
	lastNamespaceLookup time.Time
	client              client.Client
	mu                  *sync.RWMutex // Protects in-memory state
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
		deletingSAs:        make(map[ServiceAccountName]bool),
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
		deletingSAs:        make(map[ServiceAccountName]bool),
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

func (i *InMemoryEngine) ApplyBatchPlan(ctx context.Context, plan interface{}) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	type reconciliationPlan interface {
		GetScopeToSA() map[Scope]ServiceAccountName
		GetSAToWSAMap() map[ServiceAccountName]map[string]WSAResource
		GetVocabulary() *GlobalVocabulary
	}

	rp, ok := plan.(reconciliationPlan)
	if !ok {
		return fmt.Errorf("invalid plan type")
	}

	i.scopeToSA = rp.GetScopeToSA()
	i.saToWsaMap = rp.GetSAToWSAMap()
	if vocab := rp.GetVocabulary(); vocab != nil {
		i.vocabulary = vocab
	}

	i.ScopeComputation = NewScopeComputationService(i.vocabulary, i.scopeToSA)

	return nil
}

// GetServiceAccountForScope retrieves the service account for a given scope with proper locking.
// This method shadows the embedded ScopeComputation.GetServiceAccountForScope to ensure
// thread-safe access to the in-memory maps.
func (i *InMemoryEngine) GetServiceAccountForScope(scope Scope) (ServiceAccountName, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Get the ServiceAccount name for this scope
	saName, err := i.ScopeComputation.GetServiceAccountForScope(scope)
	if err != nil {
		return "", err
	}

	// Check if this ServiceAccount is marked for deletion
	if i.deletingSAs[saName] {
		// SA is being deleted, don't return it for new pods
		return "", nil
	}

	return saName, nil
}

// syncDeletingSAs updates the deletingSAs map by listing all managed ServiceAccounts
// and tracking which ones have a deletionTimestamp set.
// NOTE: This function assumes the caller already holds the write lock (i.mu.Lock()).
func (i *InMemoryEngine) syncDeletingSAs(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("syncDeletingSAs")

	// List all ServiceAccounts managed by this controller
	saList := &corev1.ServiceAccountList{}
	if err := i.client.List(ctx, saList, client.MatchingLabels{ManagedByLabel: ManagedByValue}); err != nil {
		return fmt.Errorf("failed to list ServiceAccounts: %w", err)
	}

	// Clear the map and rebuild it from current cluster state
	maps.Clear(i.deletingSAs)

	// Track SAs with deletionTimestamp set
	for idx := range saList.Items {
		sa := &saList.Items[idx]
		if !sa.DeletionTimestamp.IsZero() {
			i.deletingSAs[ServiceAccountName(sa.Name)] = true
			logger.V(1).Info("Tracking ServiceAccount marked for deletion",
				"name", sa.Name,
				"namespace", sa.Namespace)
		}
	}

	if len(i.deletingSAs) > 0 {
		logger.Info("Updated deletingSAs map", "count", len(i.deletingSAs))
	}

	return nil
}

// CleanupServiceAccounts performs smart cleanup of ServiceAccounts when a WSA/cWSA is deleted.
// It recomputes what ServiceAccounts are needed by all remaining resources and deletes only
// those that are no longer needed by any resource.
// The deletingResource parameter should be the resource being deleted (so it can be excluded from the calculation).
func (i *InMemoryEngine) CleanupServiceAccounts(
	ctx context.Context, deletingResource WSAResource,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("cleanupServiceAccounts")

	deletingName := deletingResource.GetName()
	deletingNamespace := deletingResource.GetNamespace()
	deletingIsClusterScoped := deletingResource.IsClusterScoped()

	logger.Info("Starting cleanup for deleted resource",
		"name", deletingName,
		"namespace", deletingNamespace,
		"isClusterScoped", deletingIsClusterScoped)

	wsaEnumerable, err := i.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get WorkloadServiceAccounts: %w", err)
	}

	cwsaEnumerable, err := i.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get ClusterWorkloadServiceAccounts: %w", err)
	}

	i.mu.RLock()
	targetNamespaces := i.targetNamespaces
	lookupNamespaces := i.lookupNamespaces
	i.mu.RUnlock()

	if lookupNamespaces {
		namespaces, err := i.DiscoverTargetNamespaces(ctx, i.client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to discover target namespaces: %w", err)
		}
		targetNamespaces = namespaces
	}

	allResources := make([]WSAResource, 0)
	for wsa := range wsaEnumerable {
		wsaResource := NewWSAResource(wsa)
		if !deletingIsClusterScoped && wsaResource.GetName() == deletingName && wsaResource.GetNamespace() == deletingNamespace {
			continue
		}
		allResources = append(allResources, wsaResource)
	}
	for cwsa := range cwsaEnumerable {
		cwsaResource := NewClusterWSAResource(cwsa)
		if deletingIsClusterScoped && cwsaResource.GetName() == deletingName {
			continue
		}
		allResources = append(allResources, cwsaResource)
	}

	logger.Info("Computed remaining resources after deletion",
		"remainingWSAs", len(allResources),
		"targetNamespaces", len(targetNamespaces))

	i.mu.RLock()
	scopeMap, _ := i.ComputeScopesForWSAs(allResources)
	_, _, _, neededServiceAccounts := i.GenerateServiceAccountMappings(scopeMap)
	i.mu.RUnlock()

	neededSet := set.New[string](len(neededServiceAccounts))
	neededNames := make([]string, 0, len(neededServiceAccounts))
	for _, sa := range neededServiceAccounts {
		neededSet.Insert(sa.Name)
		neededNames = append(neededNames, sa.Name)
	}

	logger.Info("Computed needed ServiceAccounts after deletion",
		"neededCount", len(neededNames),
		"neededSAs", neededNames)

	targetNamespacesSet := set.New[string](len(targetNamespaces))
	for _, ns := range targetNamespaces {
		targetNamespacesSet.Insert(ns)
	}

	logger.Info("Calling garbage collection")
	result, err := i.GarbageCollectServiceAccounts(ctx, neededSet, targetNamespacesSet)
	if err != nil {
		logger.Error(err, "Garbage collection failed")
		return result, err
	}

	if result.RequeueAfter > 0 {
		logger.Info("Garbage collection requires requeue",
			"requeueAfter", result.RequeueAfter)
	}

	i.mu.Lock()
	syncErr := i.syncDeletingSAs(ctx)
	i.mu.Unlock()

	if syncErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to sync deletingSAs map: %w", syncErr)
	}

	logger.Info("Cleanup complete", "result", result)
	return result, nil
}
