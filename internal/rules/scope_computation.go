package rules

import (
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// ScopeComputation defines the interface for computing scopes and service account mappings
type ScopeComputation interface {
	GetServiceAccountForScope(scope Scope) (ServiceAccountName, error)
	ComputeScopesForWSAs(wsaList []*v1beta1.WorkloadServiceAccount) (map[Scope]map[string]*v1beta1.WorkloadServiceAccount, GlobalVocabulary)
	GenerateServiceAccountMappings(scopeMap map[Scope]map[string]*v1beta1.WorkloadServiceAccount) (
		map[Scope]ServiceAccountName,
		map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount,
		map[string][]string,
		[]*corev1.ServiceAccount,
	)
}

type ScopeComputationService struct{}

func (ScopeComputationService) GetServiceAccountForScope(scope Scope) (ServiceAccountName, error) {
	//TODO implement me
	panic("implement me")
}

func (ScopeComputationService) ComputeScopesForWSAs(wsaList []*v1beta1.WorkloadServiceAccount) (map[Scope]map[string]*v1beta1.WorkloadServiceAccount, GlobalVocabulary) {
	//TODO implement me
	panic("implement me")
}

func (ScopeComputationService) GenerateServiceAccountMappings(scopeMap map[Scope]map[string]*v1beta1.WorkloadServiceAccount) (map[Scope]ServiceAccountName, map[ServiceAccountName]map[string]*v1beta1.WorkloadServiceAccount, map[string][]string, []*corev1.ServiceAccount) {
	//TODO implement me
	panic("implement me")
}
