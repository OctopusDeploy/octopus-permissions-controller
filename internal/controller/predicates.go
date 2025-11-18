package controller

import (
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func ManagedResourcePredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return rules.IsOctopusManaged(obj.GetLabels())
	})
}

func OwnedResourcePredicate() predicate.Predicate {
	return predicate.Or(
		ManagedResourcePredicate(),
		predicate.Funcs{
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
		},
	)
}

func GenerationOrDeletePredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicate.Funcs{
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
		},
	)
}

func ManagedServiceAccountPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return IsManagedServiceAccount(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return IsManagedServiceAccount(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return IsManagedServiceAccount(e.Object)
		},
	}
}

func SuppressOwnedResourceDeletes() predicate.Predicate {
	return predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

func IsManagedServiceAccount(obj interface{}) bool {
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return false
	}
	return sa.Labels[rules.ManagedByLabel] == rules.ManagedByValue
}
