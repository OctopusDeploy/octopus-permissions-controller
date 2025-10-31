package rules

import (
	"context"
	"fmt"
	"regexp"
	"sync"

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
	Reconcile(ctx context.Context) error
	ReconcileResource(ctx context.Context, resource WSAResource) error
	CleanupServiceAccounts(ctx context.Context, deletingResource WSAResource) (ctrl.Result, error)
}

type InMemoryEngine struct {
	scopeToSA        map[Scope]ServiceAccountName
	vocabulary       *GlobalVocabulary
	saToWsaMap       map[ServiceAccountName]map[string]WSAResource
	wsaToScopesMap   map[string][]Scope          // Tracks previous scopes for each WSA to detect changes
	deletingSAs      map[ServiceAccountName]bool // Tracks SAs marked for deletion (with deletionTimestamp)
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
		wsaToScopesMap:     make(map[string][]Scope),
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
		wsaToScopesMap:     make(map[string][]Scope),
		deletingSAs:        make(map[ServiceAccountName]bool),
		mu:                 &sync.RWMutex{},
		ResourceManagement: NewResourceManagementServiceWithScheme(controllerClient, scheme),
		NamespaceDiscovery: NamespaceDiscoveryService{},
	}
	engine.ScopeComputation = NewScopeComputationService(engine.vocabulary, engine.scopeToSA)
	return engine
}

func (i *InMemoryEngine) Reconcile(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	wsaEnumerable, err := i.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return err
	}

	// Fetch ClusterWorkloadServiceAccounts
	cwsaEnumerable, err := i.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return err
	}

	// Get our target namespaces
	if i.lookupNamespaces {
		targetNamespaces, err := i.DiscoverTargetNamespaces(ctx, i.client)
		if err != nil {
			return fmt.Errorf("failed to discover target namespaces: %w", err)
		}
		i.targetNamespaces = targetNamespaces
	}

	// Convert both types to WSAResource interface
	allResources := make([]WSAResource, 0)
	for wsa := range wsaEnumerable {
		allResources = append(allResources, NewWSAResource(wsa))
	}
	for cwsa := range cwsaEnumerable {
		allResources = append(allResources, NewClusterWSAResource(cwsa))
	}

	scopeMap, vocabulary := i.ComputeScopesForWSAs(allResources)
	*i.vocabulary = vocabulary

	// Generate service accounts
	scopeToSaNameMap, saToWsaMap, wsaToServiceAccountNames, uniqueServiceAccounts := i.GenerateServiceAccountMappings(scopeMap)

	// Clear and rebuild maps
	maps.Clear(i.scopeToSA)
	maps.Clear(i.saToWsaMap)
	maps.Clear(i.wsaToScopesMap)

	for scope, sa := range scopeToSaNameMap {
		i.scopeToSA[scope] = sa
	}
	for sa, wsa := range saToWsaMap {
		i.saToWsaMap[sa] = wsa
	}

	// Track scopes for each WSA to enable scope change detection
	for scope, wsaMap := range scopeMap {
		for wsaName := range wsaMap {
			i.wsaToScopesMap[wsaName] = append(i.wsaToScopesMap[wsaName], scope)
		}
	}

	createdRoles, err := i.EnsureRoles(ctx, allResources)
	if err != nil {
		return fmt.Errorf("failed to ensure roles: %w", err)
	}

	err = i.EnsureServiceAccounts(ctx, uniqueServiceAccounts, i.targetNamespaces)
	if err != nil {
		return fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	// For each WSA, bind the roles or created roles to the service accounts in each target namespace
	if bindErr := i.EnsureRoleBindings(ctx, allResources, createdRoles, wsaToServiceAccountNames, i.targetNamespaces); bindErr != nil {
		return fmt.Errorf("failed to ensure role bindings: %w", bindErr)
	}

	// Garbage collect ServiceAccounts that are no longer needed
	expectedServiceAccounts := set.New[string](len(uniqueServiceAccounts))
	for _, sa := range uniqueServiceAccounts {
		expectedServiceAccounts.Insert(sa.Name)
	}
	targetNamespacesSet := set.New[string](len(i.targetNamespaces))
	for _, ns := range i.targetNamespaces {
		targetNamespacesSet.Insert(ns)
	}
	if _, gcErr := i.GarbageCollectServiceAccounts(ctx, expectedServiceAccounts, targetNamespacesSet); gcErr != nil {
		return fmt.Errorf("failed to garbage collect service accounts: %w", gcErr)
	}

	// Update the deletingSAs map to track which SAs are marked for deletion
	if err := i.syncDeletingSAs(ctx); err != nil {
		// Return error to fail reconciliation and ensure map stays in sync
		return fmt.Errorf("failed to sync deletingSAs map: %w", err)
	}

	return nil
}

// ReconcileResource performs an incremental reconciliation for a single WorkloadServiceAccount or ClusterWorkloadServiceAccount.
// This should be called during normal reconcile loops when a specific resource changes.
func (i *InMemoryEngine) ReconcileResource(ctx context.Context, resource WSAResource) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Initialize target namespaces if not already set
	if err := i.initializeTargetNamespaces(ctx); err != nil {
		return err
	}

	targetNamespaces := i.targetNamespaces

	// Compute scopes for just this resource
	scopeMap, newVocabulary := i.ComputeScopesForWSAs([]WSAResource{resource})

	// Detect if scopes have changed for this resource
	changeInfo := i.detectScopeChanges(resource, scopeMap)

	// If scopes changed, perform full reconciliation
	if changeInfo.scopesChanged {
		return i.reconcileWithScopeChanges(ctx, targetNamespaces)
	}

	// Fast path: Only permissions changed, not scopes
	return i.reconcilePermissionsOnly(ctx, resource, scopeMap, newVocabulary, changeInfo.newScopes, targetNamespaces)
}

// initializeTargetNamespaces ensures target namespaces are initialized
func (i *InMemoryEngine) initializeTargetNamespaces(ctx context.Context) error {
	if len(i.targetNamespaces) == 0 {
		if i.lookupNamespaces {
			targetNamespaces, err := i.DiscoverTargetNamespaces(ctx, i.client)
			if err != nil {
				return fmt.Errorf("failed to discover target namespaces: %w", err)
			}
			i.targetNamespaces = targetNamespaces
		}
		// If still empty after lookup, it's an error
		if len(i.targetNamespaces) == 0 {
			return fmt.Errorf("target namespaces not initialized and discovery failed")
		}
	}
	return nil
}

// scopeChangeInfo contains information about scope changes for a resource
type scopeChangeInfo struct {
	previousScopes    []Scope
	hadPreviousScopes bool
	newScopes         []Scope
	scopesChanged     bool
}

// detectScopeChanges detects if scopes have changed for a resource
func (i *InMemoryEngine) detectScopeChanges(
	resource WSAResource, scopeMap map[Scope]map[string]WSAResource,
) scopeChangeInfo {
	wsaName := resource.GetName()
	previousScopes, hadPreviousScopes := i.wsaToScopesMap[wsaName]

	// Extract new scopes for this WSA from the scopeMap
	newScopes := make([]Scope, 0)
	for scope := range scopeMap {
		newScopes = append(newScopes, scope)
	}

	// Check if scopes have changed
	scopesChanged := !hadPreviousScopes || scopesChanged(previousScopes, newScopes)

	return scopeChangeInfo{
		previousScopes:    previousScopes,
		hadPreviousScopes: hadPreviousScopes,
		newScopes:         newScopes,
		scopesChanged:     scopesChanged,
	}
}

// reconcileWithScopeChanges performs full reconciliation when scopes have changed
func (i *InMemoryEngine) reconcileWithScopeChanges(ctx context.Context, targetNamespaces []string) error {
	allResources, err := i.fetchAllResources(ctx)
	if err != nil {
		return err
	}

	fullScopeMap, fullVocabulary := i.ComputeScopesForWSAs(allResources)
	*i.vocabulary = fullVocabulary

	scopeToSaNameMap, saToWsaMap, wsaToServiceAccountNames, neededServiceAccounts := i.GenerateServiceAccountMappings(fullScopeMap)

	i.updateInMemoryMaps(scopeToSaNameMap, saToWsaMap, fullScopeMap)

	if err := i.ensureAllRBACResources(ctx, allResources, neededServiceAccounts, wsaToServiceAccountNames, targetNamespaces); err != nil {
		return err
	}

	return i.cleanupOrphanedResources(ctx, neededServiceAccounts, targetNamespaces)
}

func (i *InMemoryEngine) fetchAllResources(ctx context.Context) ([]WSAResource, error) {
	wsaEnumerable, err := i.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get WorkloadServiceAccounts: %w", err)
	}

	cwsaEnumerable, err := i.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ClusterWorkloadServiceAccounts: %w", err)
	}

	allResources := make([]WSAResource, 0)
	for wsa := range wsaEnumerable {
		allResources = append(allResources, NewWSAResource(wsa))
	}
	for cwsa := range cwsaEnumerable {
		allResources = append(allResources, NewClusterWSAResource(cwsa))
	}

	return allResources, nil
}

func (i *InMemoryEngine) updateInMemoryMaps(
	scopeToSaNameMap map[Scope]ServiceAccountName,
	saToWsaMap map[ServiceAccountName]map[string]WSAResource,
	fullScopeMap map[Scope]map[string]WSAResource,
) {
	maps.Clear(i.scopeToSA)
	maps.Clear(i.saToWsaMap)
	maps.Clear(i.wsaToScopesMap)

	for scope, sa := range scopeToSaNameMap {
		i.scopeToSA[scope] = sa
	}
	for sa, wsaMap := range saToWsaMap {
		i.saToWsaMap[sa] = wsaMap
	}

	for scope, wsaMap := range fullScopeMap {
		for wsaName := range wsaMap {
			i.wsaToScopesMap[wsaName] = append(i.wsaToScopesMap[wsaName], scope)
		}
	}
}

func (i *InMemoryEngine) ensureAllRBACResources(
	ctx context.Context,
	allResources []WSAResource,
	neededServiceAccounts []*corev1.ServiceAccount,
	wsaToServiceAccountNames map[string][]string,
	targetNamespaces []string,
) error {
	createdRoles, err := i.EnsureRoles(ctx, allResources)
	if err != nil {
		return fmt.Errorf("failed to ensure roles: %w", err)
	}

	if err := i.EnsureServiceAccounts(ctx, neededServiceAccounts, targetNamespaces); err != nil {
		return fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	for _, sa := range neededServiceAccounts {
		log.FromContext(ctx).V(1).Info("Expected ServiceAccount",
			"name", sa.Name,
			"annotations", sa.Annotations)
	}

	if err := i.EnsureRoleBindings(ctx, allResources, createdRoles, wsaToServiceAccountNames, targetNamespaces); err != nil {
		return fmt.Errorf("failed to ensure role bindings: %w", err)
	}

	return nil
}

func (i *InMemoryEngine) cleanupOrphanedResources(
	ctx context.Context,
	neededServiceAccounts []*corev1.ServiceAccount,
	targetNamespaces []string,
) error {
	neededSet := set.New[string](len(neededServiceAccounts))
	for _, sa := range neededServiceAccounts {
		neededSet.Insert(sa.Name)
	}

	targetNamespacesSet := set.New[string](len(targetNamespaces))
	for _, ns := range targetNamespaces {
		targetNamespacesSet.Insert(ns)
	}

	if _, err := i.GarbageCollectServiceAccounts(ctx, neededSet, targetNamespacesSet); err != nil {
		return fmt.Errorf("failed to garbage collect service accounts: %w", err)
	}

	if err := i.syncDeletingSAs(ctx); err != nil {
		return fmt.Errorf("failed to sync deletingSAs map: %w", err)
	}

	return nil
}

// reconcilePermissionsOnly performs fast path reconciliation when only permissions changed
func (i *InMemoryEngine) reconcilePermissionsOnly(
	ctx context.Context, resource WSAResource, scopeMap map[Scope]map[string]WSAResource,
	newVocabulary GlobalVocabulary, newScopes []Scope, targetNamespaces []string,
) error {
	// Merge new vocabulary dimensions into existing vocabulary
	i.mergeVocabulary(newVocabulary)

	// Generate service account mappings for this resource
	scopeToSaNameMap, saToWsaMap, wsaToServiceAccountNames, uniqueServiceAccounts := i.GenerateServiceAccountMappings(scopeMap)

	// Update in-memory maps for this resource
	for scope, sa := range scopeToSaNameMap {
		i.scopeToSA[scope] = sa
	}
	for sa, wsaMap := range saToWsaMap {
		if i.saToWsaMap[sa] == nil {
			i.saToWsaMap[sa] = make(map[string]WSAResource)
		}
		for wsaName, wsaRes := range wsaMap {
			i.saToWsaMap[sa][wsaName] = wsaRes
		}
	}

	// Update wsaToScopesMap with current scopes
	wsaName := resource.GetName()
	i.wsaToScopesMap[wsaName] = newScopes

	// Ensure roles for this resource
	createdRoles, err := i.EnsureRoles(ctx, []WSAResource{resource})
	if err != nil {
		return fmt.Errorf("failed to ensure roles: %w", err)
	}

	// Ensure service accounts for this resource
	if err := i.EnsureServiceAccounts(ctx, uniqueServiceAccounts, targetNamespaces); err != nil {
		return fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	// Log what SAs were created for debugging (fast path)
	for _, sa := range uniqueServiceAccounts {
		log.FromContext(ctx).V(1).Info("Expected ServiceAccount (fast path)",
			"name", sa.Name,
			"annotations", sa.Annotations)
	}

	// Ensure role bindings for this resource
	if err := i.EnsureRoleBindings(ctx, []WSAResource{resource}, createdRoles, wsaToServiceAccountNames, targetNamespaces); err != nil {
		return fmt.Errorf("failed to ensure role bindings: %w", err)
	}

	return nil
}

// mergeVocabulary merges new vocabulary dimensions into the existing vocabulary
func (i *InMemoryEngine) mergeVocabulary(newVocab GlobalVocabulary) {
	// Merge each dimension by inserting all values from new vocabulary into existing
	for dimIndex := DimensionIndex(0); dimIndex < MaxDimensionIndex; dimIndex++ {
		if newVocab[dimIndex] != nil {
			for value := range newVocab[dimIndex].Items() {
				i.vocabulary[dimIndex].Insert(value)
			}
		}
	}
}

// scopesChanged checks if the scopes for a WSA have changed by comparing old and new scope sets
func scopesChanged(oldScopes, newScopes []Scope) bool {
	if len(oldScopes) != len(newScopes) {
		return true
	}

	// Create a set of old scopes for efficient lookup
	oldScopeSet := set.New[Scope](len(oldScopes))
	for _, scope := range oldScopes {
		oldScopeSet.Insert(scope)
	}

	// Check if all new scopes are in the old set
	for _, scope := range newScopes {
		if !oldScopeSet.Contains(scope) {
			return true
		}
	}

	return false
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
