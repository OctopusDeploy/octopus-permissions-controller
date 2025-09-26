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

type InMemoryEngine struct {
	scopeToSA        map[Scope]ServiceAccountName
	saToWsaMap       map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount
	targetNamespaces []string
	resources        Resources
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

func NewInMemoryEngine(targetNamespaces []string, client client.Client) InMemoryEngine {
	return InMemoryEngine{
		scopeToSA:        make(map[Scope]ServiceAccountName),
		targetNamespaces: targetNamespaces,
		resources:        NewResources(client),
	}
}

func (i *InMemoryEngine) GetServiceAccountForScope(scope Scope) (ServiceAccountName, error) {
	if sa, ok := i.scopeToSA[scope]; ok {
		return sa, nil
	}

	return "", nil
}

func (i *InMemoryEngine) Reconcile(ctx context.Context) error {
	wsaEnumerable, err := i.resources.getWorkloadServiceAccounts(ctx)
	if err != nil {
		return err
	}

	var wsaList = slices.Collect(wsaEnumerable)
	scopeMap := getScopesForWSAs(wsaList)

	// Generate service accounts
	scopeToSaNameMap, saToWsaMap, wsaToServiceAccountNames, uniqueServiceAccounts := GenerateServiceAccountMappings(scopeMap)

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

	// For each WSA, bind the roles or created roles to the service accounts in each target namespace
	if bindErr := i.resources.ensureRoleBindings(ctx, wsaList, createdRoles, wsaToServiceAccountNames, i.targetNamespaces); bindErr != nil {
		return fmt.Errorf("failed to ensure role bindings: %w", bindErr)
	}

	return nil
}
