package rules

import (
	"context"
	"fmt"

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
	wsaList, err := i.resources.getWorkloadServiceAccounts(ctx)
	if err != nil {
		return err
	}

	_, err = i.resources.ensureRoles(wsaList)

	return nil
}
