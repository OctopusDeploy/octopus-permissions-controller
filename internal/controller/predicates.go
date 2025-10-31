package controller

import (
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func ServiceAccountDeletionPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSA, oldOk := e.ObjectOld.(*corev1.ServiceAccount)
			newSA, newOk := e.ObjectNew.(*corev1.ServiceAccount)
			if !oldOk || !newOk {
				return false
			}

			oldDeleting := !oldSA.DeletionTimestamp.IsZero()
			newDeleting := !newSA.DeletionTimestamp.IsZero()
			oldHasFinalizer := hasSAFinalizer(oldSA)
			newHasFinalizer := hasSAFinalizer(newSA)

			return (!oldDeleting && newDeleting) || (oldHasFinalizer && !newHasFinalizer)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
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

func IsServiceAccountBeingDeleted(obj interface{}) bool {
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return false
	}
	return !sa.DeletionTimestamp.IsZero()
}
