package rules

import (
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

type Rule struct {
	Permissions v1beta1.WorkloadServiceAccountPermissions `json:"permissions"`
}

type Engine interface {
	GetServiceAccountForScope(scope Scope, agentName AgentName) (ServiceAccountName, error)
	AddScopeRuleset(scope Scope, rule Rule, targetNamespace Namespace) error
	RemoveScopeRuleset(scope Scope, rule Rule, targetNamespace Namespace) error
}

type InMemoryEngine struct {
	rules            map[AgentName]map[Scope]ServiceAccountName
	targetNamespaces []string
	// client kubernetes.Interface
}

func (s *Scope) IsEmpty() bool {
	return s.Project == "" && s.Environment == "" && s.Tenant == "" && s.Step == "" && s.Space == ""
}

func NewInMemoryEngine(targetNamespaces []string) InMemoryEngine {
	return InMemoryEngine{
		rules:            make(map[AgentName]map[Scope]ServiceAccountName),
		targetNamespaces: targetNamespaces,
	}
}

func (i InMemoryEngine) GetServiceAccountForScope(scope Scope, agentName AgentName) (ServiceAccountName, error) {
	if agentRules, ok := i.rules[agentName]; ok {
		if sa, ok := agentRules[scope]; ok {
			return sa, nil
		}
	}
	return "", nil
}

func (i InMemoryEngine) AddScopeRuleset(scope Scope, rule Rule, targetNamespace Namespace) error {
	// TODO: Implement me
	return nil
}

func (i InMemoryEngine) RemoveScopeRuleset(scope Scope, rule Rule, targetNamespace Namespace) error {
	// TODO: Implement me
	return nil
}
