package controller

import (
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerFieldOwner = "octopus-permissions-controller"

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

// ExternalChangePredicate filters out Create/Update events caused by this controller's
// Server-Side Apply operations by checking the ManagedFields metadata.
func ExternalChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return !isOnlyManagedByController(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !wasMostRecentChangeByController(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return true
		},
	}
}

// ExternalDeletePredicate filters out Delete events for resources being actively
// garbage collected by this controller.
func ExternalDeletePredicate(tracker GCTrackerInterface) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if tracker == nil {
				return true
			}
			return !tracker.IsBeingDeleted(e.Object.GetUID())
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return true
		},
	}
}

// isOnlyManagedByController returns true if all managedFields entries are from this controller.
func isOnlyManagedByController(obj client.Object) bool {
	managedFields := obj.GetManagedFields()
	if len(managedFields) == 0 {
		return false
	}

	for _, mf := range managedFields {
		if mf.Manager != controllerFieldOwner {
			return false
		}
	}
	return true
}

// wasMostRecentChangeByController returns true if the most recent managedFields
// entry (by time) was made by this controller.
func wasMostRecentChangeByController(obj client.Object) bool {
	managedFields := obj.GetManagedFields()
	if len(managedFields) == 0 {
		return false
	}

	var mostRecent *metav1.ManagedFieldsEntry
	for i := range managedFields {
		mf := &managedFields[i]
		if mf.Time == nil {
			continue
		}
		if mostRecent == nil || mostRecent.Time == nil || mf.Time.After(mostRecent.Time.Time) {
			mostRecent = mf
		}
	}

	if mostRecent == nil {
		return false
	}

	return mostRecent.Manager == controllerFieldOwner
}
