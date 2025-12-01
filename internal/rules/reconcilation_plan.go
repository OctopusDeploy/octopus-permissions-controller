package rules

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type ReconciliationPlan struct {
	ScopeToSA        map[Scope]ServiceAccountName
	SAToWSAMap       map[ServiceAccountName]map[types.NamespacedName]WSAResource
	WSAToSANames     map[types.NamespacedName][]ServiceAccountName
	Vocabulary       *GlobalVocabulary
	UniqueAccounts   []*v1.ServiceAccount
	AllResources     []WSAResource
	TargetNamespaces []string
}

func (rp *ReconciliationPlan) GetScopeToSA() map[Scope]ServiceAccountName {
	return rp.ScopeToSA
}

func (rp *ReconciliationPlan) GetSAToWSAMap() map[ServiceAccountName]map[types.NamespacedName]WSAResource {
	return rp.SAToWSAMap
}

func (rp *ReconciliationPlan) GetVocabulary() *GlobalVocabulary {
	return rp.Vocabulary
}
