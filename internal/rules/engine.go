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
	GetServiceAccountForScope(scope Scope) (ServiceAccountName, error)
	Reconcile(ctx context.Context) error
}

// TODO: workshop my terrible naming
//
// type NamespaceDiscoverer interface {
//     DiscoverTargetNamespaces(ctx context.Context) ([]string, error)
// }
//
// type ScopeAnalyser interface {
//     AnalyseWorkloadServiceAccounts(wsaList []*v1beta1.WorkloadServiceAccount) (ScopeAnalysisResult, error)
// }
//
// type ServiceAccountGenerator interface {
//     GenerateServiceAccountMappings(scopeMap map[Scope]map[string]*v1beta1.WorkloadServiceAccount) GenerationResult
// }
//
// type ResourceOrchestrator interface {
//     EnsureRoles(ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount) (map[string]rbacv1.Role, error)
//     EnsureServiceAccounts(ctx context.Context, accounts []*v1.ServiceAccount, namespaces []string) error
//     EnsureRoleBindings(ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount, roles map[string]rbacv1.Role, accountMappings map[string][]string, namespaces []string) error
// }

type InMemoryEngine struct {
	scopeToSA        map[Scope]ServiceAccountName                                      // move to ReconciliationState
	vocabulary       GlobalVocabulary                                                  // move to ReconciliationState
	saToWsaMap       map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount // move to ReconciliationState
	targetNamespaces []string                                                          // move to NamespaceConfig
	lookupNamespaces bool                                                              // move to NamespaceConfig
	resources        Resources                                                         // REFACTOR: Extract to ResourceOrchestrator interface
	client           client.Client
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
		scopeToSA:        make(map[Scope]ServiceAccountName),
		targetNamespaces: []string{},
		lookupNamespaces: true,
		resources:        NewResources(controllerClient),
		client:           controllerClient,
	}
}

func NewInMemoryEngineWithNamespaces(controllerClient client.Client, targetNamespaces []string) InMemoryEngine {
	return InMemoryEngine{
		scopeToSA:        make(map[Scope]ServiceAccountName),
		targetNamespaces: targetNamespaces,
		lookupNamespaces: len(targetNamespaces) == 0,
		resources:        NewResources(controllerClient),
		client:           controllerClient,
	}
}

func (i *InMemoryEngine) GetServiceAccountForScope(scope Scope) (ServiceAccountName, error) {
	knownScope := i.vocabulary.GetKnownScopeCombination(scope)
	if sa, ok := i.scopeToSA[knownScope]; ok {
		return sa, nil
	}

	return "", nil
}

func (i *InMemoryEngine) Reconcile(ctx context.Context) error {
	// Extract to -> dataRetriever.GetWorkloadServiceAccounts(ctx)
	wsaEnumerable, err := i.resources.getWorkloadServiceAccounts(ctx)
	if err != nil {
		return err
	}

	// Extract to -> namespaceDiscoverer.DiscoverTargetNamespaces(ctx)
	// Q: why does this mutate i.targetNamespaces
	if i.lookupNamespaces {
		targetNamespaces, err := DiscoverTargetNamespaces(i.client)
		if err != nil {
			return fmt.Errorf("failed to discover target namespaces: %w", err)
		}
		i.targetNamespaces = targetNamespaces
	}

	// Extract to -> scopeAnalyser.AnalyseScopes(wsaList)
	var wsaList = slices.Collect(wsaEnumerable)
	scopeMap, vocabulary := getScopesForWSAs(wsaList)
	i.vocabulary = vocabulary

	// Extract to -> serviceAccountGenerator.GenerateMappings(scopeMap)
	// maybe overkill
	scopeToSaNameMap, saToWsaMap, wsaToServiceAccountNames, uniqueServiceAccounts := GenerateServiceAccountMappings(scopeMap)

	// concurrency issues with doing this here ?
	i.scopeToSA = scopeToSaNameMap
	i.saToWsaMap = saToWsaMap

	createdRoles, err := i.resources.ensureRoles(wsaList)
	if err != nil {
		return fmt.Errorf("failed to ensure roles: %w", err)
	}

	err = i.resources.ensureServiceAccounts(uniqueServiceAccounts, i.targetNamespaces)
	if err != nil {
		return fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	// what happens if this fails ? is there rollback or can we make it atomic?
	if bindErr := i.resources.ensureRoleBindings(ctx, wsaList, createdRoles, wsaToServiceAccountNames, i.targetNamespaces); bindErr != nil {
		return fmt.Errorf("failed to ensure role bindings: %w", bindErr)
	}

	return nil
}
