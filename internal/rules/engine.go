package rules

import (
	"fmt"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
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

func (s Scope) String() string {
	return fmt.Sprintf("projects=%s,environments=%s,tenants=%s,steps=%s",
		s.Project,
		s.Environment,
		s.Tenant,
		s.Step)
}

type Rule struct {
	Permissions v1beta1.WorkloadServiceAccountPermissions `json:"permissions"`
}

type Engine interface {
	GetServiceAccountForScope(scope Scope, agentName AgentName) (ServiceAccountName, error)
	RegenerateFromWSAs(wsas []v1beta1.WorkloadServiceAccount) error
}

type InMemoryEngine struct {
	rules map[AgentName]map[Scope]ServiceAccountName
	// client kubernetes.Interface
}

func (s *Scope) IsEmpty() bool {
	return s.Project == "" && s.Environment == "" && s.Tenant == "" && s.Step == "" && s.Space == ""
}

func NewInMemoryEngine() InMemoryEngine {
	return InMemoryEngine{
		rules: make(map[AgentName]map[Scope]ServiceAccountName),
	}
}

func (i *InMemoryEngine) GetServiceAccountForScope(scope Scope, agentName AgentName) (ServiceAccountName, error) {
	if agentRules, ok := i.rules[agentName]; ok {
		if sa, ok := agentRules[scope]; ok {
			return sa, nil
		}
	}
	return "", nil
}

func (i *InMemoryEngine) RegenerateFromWSAs(wsas []v1beta1.WorkloadServiceAccount) error {
	// TODO: Support scoping WSAs to specific agents
	const defaultAgent = AgentName("default")

	scopePermissionsMap := GenerateAllScopesWithPermissions(wsas)
	// TODO: Optimize by comparing with existing rules and only updating changed ones

	i.rules[defaultAgent] = make(map[Scope]ServiceAccountName)

	for scope := range scopePermissionsMap {
		serviceAccountName := GenerateServiceAccountName(scope)
		i.rules[defaultAgent][scope] = serviceAccountName
	}

	// TODO: Create or update Kubernetes resources (ServiceAccounts, Roles, RoleBindings) based on the generated rules

	return nil
}
