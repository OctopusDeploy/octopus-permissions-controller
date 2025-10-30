package controller

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ServiceAccountCleanupFinalizer is added to WSA/CWSA resources to ensure
	// ServiceAccounts are cleaned up when the resource is deleted
	ServiceAccountCleanupFinalizer = "octopus.com/serviceaccount-cleanup"

	// ServiceAccountFinalizer is added to ServiceAccounts created by this controller
	// to track ownership and enable cleanup regardless of namespace
	ServiceAccountFinalizer = "octopus.com/serviceaccount"

	// ManagedByLabel is added to all resources created by this controller
	ManagedByLabel = "octopus.com/managed-by"

	// ManagedByValue is the value for the ManagedByLabel
	ManagedByValue = "permissions-controller"

	// ConditionTypeReady indicates the resource is fully operational and reconciled
	ConditionTypeReady = "Ready"

	ReasonReconcileSuccess = "ReconcileSuccess" // Used when reconciliation succeeds
	ReasonReconcileFailed  = "ReconcileFailed"  // Used when reconciliation fails
)

// hasFinalizer checks if the object has the specified finalizer
func hasFinalizer(finalizers []string, finalizer string) bool {
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// addFinalizer adds a finalizer to the object if it doesn't already exist
func addFinalizer(obj client.Object) bool {
	if !hasFinalizer(obj.GetFinalizers(), ServiceAccountCleanupFinalizer) {
		finalizers := obj.GetFinalizers()
		finalizers = append(finalizers, ServiceAccountCleanupFinalizer)
		obj.SetFinalizers(finalizers)
		return true
	}
	return false
}

// removeFinalizer removes the finalizer from the object
func removeFinalizer(obj client.Object) bool {
	finalizers := obj.GetFinalizers()
	for i, f := range finalizers {
		if f == ServiceAccountCleanupFinalizer {
			finalizers = append(finalizers[:i], finalizers[i+1:]...)
			obj.SetFinalizers(finalizers)
			return true
		}
	}
	return false
}

// hasSAFinalizer checks if a ServiceAccount has the ServiceAccount finalizer
func hasSAFinalizer(sa *corev1.ServiceAccount) bool {
	return hasFinalizer(sa.GetFinalizers(), ServiceAccountFinalizer)
}
