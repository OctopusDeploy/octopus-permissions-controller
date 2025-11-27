package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ServiceAccountCleanupFinalizer = "octopus.com/serviceaccount-cleanup"
	ManagedByLabel                 = "octopus.com/managed-by"
	ManagedByValue                 = "permissions-controller"

	// Condition types and reasons are defined in staging package to avoid import cycles.
	// Use staging.ConditionTypeReady, staging.ReasonSucceeded, etc.

	ReasonReconcileSuccess = "ReconcileSuccess"
	ReasonReconcileFailed  = "ReconcileFailed"
)

func containsCleanupFinalizer(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, ServiceAccountCleanupFinalizer)
}

func addCleanupFinalizer(obj client.Object) bool {
	return controllerutil.AddFinalizer(obj, ServiceAccountCleanupFinalizer)
}

func removeCleanupFinalizer(obj client.Object) bool {
	return controllerutil.RemoveFinalizer(obj, ServiceAccountCleanupFinalizer)
}
