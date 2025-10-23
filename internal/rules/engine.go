package rules

import (
	"context"
	"fmt"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
}

type InMemoryEngine struct {
	scopeToSA        map[Scope]ServiceAccountName
	vocabulary       *GlobalVocabulary
	saToWsaMap       map[ServiceAccountName]map[string]WSAResource
	targetNamespaces []string
	lookupNamespaces bool
	client           client.Client
	ScopeComputation
	ResourceManagement
	NamespaceDiscovery
}


func NewInMemoryEngine(controllerClient client.Client) InMemoryEngine {
	engine := InMemoryEngine{
		scopeToSA:          make(map[Scope]ServiceAccountName),
		targetNamespaces:   []string{},
		lookupNamespaces:   true,
		client:             controllerClient,
		vocabulary:         &GlobalVocabulary{},
		saToWsaMap:         make(map[ServiceAccountName]map[string]WSAResource),
		ResourceManagement: NewResourceManagementService(controllerClient),
		NamespaceDiscovery: NamespaceDiscoveryService{},
	}
	engine.ScopeComputation = NewScopeComputationService(engine.vocabulary, &engine.scopeToSA)
	return engine
}

func NewInMemoryEngineWithNamespaces(controllerClient client.Client, targetNamespaces []string) InMemoryEngine {
	engine := InMemoryEngine{
		scopeToSA:          make(map[Scope]ServiceAccountName),
		targetNamespaces:   targetNamespaces,
		lookupNamespaces:   len(targetNamespaces) == 0,
		client:             controllerClient,
		vocabulary:         &GlobalVocabulary{},
		saToWsaMap:         make(map[ServiceAccountName]map[string]WSAResource),
		ResourceManagement: NewResourceManagementService(controllerClient),
		NamespaceDiscovery: NamespaceDiscoveryService{},
	}
	engine.ScopeComputation = NewScopeComputationService(engine.vocabulary, &engine.scopeToSA)
	return engine
}

func (i *InMemoryEngine) Reconcile(ctx context.Context) error {
	// Fetch WorkloadServiceAccounts
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

	for scope, sa := range scopeToSaNameMap {
		i.scopeToSA[scope] = sa
	}
	for sa, wsa := range saToWsaMap {
		i.saToWsaMap[sa] = wsa
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

	return nil
}
