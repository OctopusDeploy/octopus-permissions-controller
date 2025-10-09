package rules

import (
	"context"
	"fmt"
	"slices"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentName string

type Namespace string

type ServiceAccountName string

type Scope struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Tenant      string `json:"tenant"`
	Step        string `json:"step"`
	Space       string `json:"space"`
}

type Engine interface {
	ResourceManagement
	NamespaceDiscovery
	ScopeComputation
	Reconcile(ctx context.Context) error
	// TODO: figure out the best way to retrieve this for the ScopeComputation.GetServiceACcountForScope inputs
	GetVocabulary() GlobalVocabulary
	GetScopeToServiceAccountMap() map[Scope]ServiceAccountName
}

type InMemoryEngine struct {
	scopeToSA        map[Scope]ServiceAccountName
	vocabulary       GlobalVocabulary
	saToWsaMap       map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount
	targetNamespaces []string
	lookupNamespaces bool
	client           client.Client
	ScopeComputation
	ResourceManagement
	NamespaceDiscovery
}

func (s *Scope) IsEmpty() bool {
	return s.Project == "" && s.Environment == "" && s.Tenant == "" && s.Step == "" && s.Space == ""
}

func (s *Scope) String() string {
	return fmt.Sprintf("projects=%s,environments=%s,tenants=%s,steps=%s,spaces=%s",
		s.Project,
		s.Environment,
		s.Tenant,
		s.Step,
		s.Space)
}

func NewInMemoryEngine(controllerClient client.Client) InMemoryEngine {
	return InMemoryEngine{
		scopeToSA:          make(map[Scope]ServiceAccountName),
		targetNamespaces:   []string{},
		lookupNamespaces:   true,
		client:             controllerClient,
		ScopeComputation:   ScopeComputationService{},
		ResourceManagement: NewResourceManagementService(controllerClient),
		NamespaceDiscovery: NamespaceDiscoveryService{},
	}
}

func NewInMemoryEngineWithNamespaces(controllerClient client.Client, targetNamespaces []string) InMemoryEngine {
	return InMemoryEngine{
		scopeToSA:          make(map[Scope]ServiceAccountName),
		targetNamespaces:   targetNamespaces,
		lookupNamespaces:   len(targetNamespaces) == 0,
		client:             controllerClient,
		ScopeComputation:   ScopeComputationService{},
		ResourceManagement: NewResourceManagementService(controllerClient),
		NamespaceDiscovery: NamespaceDiscoveryService{},
	}
}

func (i *InMemoryEngine) GetVocabulary() GlobalVocabulary {
	return i.vocabulary
}

func (i *InMemoryEngine) GetScopeToServiceAccountMap() map[Scope]ServiceAccountName {
	return i.scopeToSA
}

func (i *InMemoryEngine) Reconcile(ctx context.Context) error {
	wsaEnumerable, err := i.GetWorkloadServiceAccounts(ctx)
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

	var wsaList = slices.Collect(wsaEnumerable)
	scopeMap, vocabulary := i.ComputeScopesForWSAs(wsaList)
	i.vocabulary = vocabulary

	// Generate service accounts
	scopeToSaNameMap, saToWsaMap, wsaToServiceAccountNames, uniqueServiceAccounts := i.GenerateServiceAccountMappings(scopeMap)

	i.scopeToSA = scopeToSaNameMap
	i.saToWsaMap = saToWsaMap

	createdRoles, err := i.EnsureRoles(ctx, wsaList)
	if err != nil {
		return fmt.Errorf("failed to ensure roles: %w", err)
	}

	err = i.EnsureServiceAccounts(ctx, uniqueServiceAccounts, i.targetNamespaces)
	if err != nil {
		return fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	// For each WSA, bind the roles or created roles to the service accounts in each target namespace
	if bindErr := i.EnsureRoleBindings(ctx, wsaList, createdRoles, wsaToServiceAccountNames, i.targetNamespaces); bindErr != nil {
		return fmt.Errorf("failed to ensure role bindings: %w", bindErr)
	}

	return nil
}
