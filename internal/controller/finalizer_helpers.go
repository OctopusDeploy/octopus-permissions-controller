package controller

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServiceAccountCleanupFinalizer = "octopus.com/serviceaccount-cleanup"
	ServiceAccountFinalizer        = "octopus.com/serviceaccount"
	ManagedByLabel                 = "octopus.com/managed-by"
	ManagedByValue                 = "permissions-controller"
	ConditionTypeReady             = "Ready"
	ReasonReconcileSuccess         = "ReconcileSuccess"
	ReasonReconcileFailed          = "ReconcileFailed"
)

func hasFinalizer(finalizers []string, finalizer string) bool {
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func addFinalizer(obj client.Object) bool {
	if !hasFinalizer(obj.GetFinalizers(), ServiceAccountCleanupFinalizer) {
		finalizers := obj.GetFinalizers()
		finalizers = append(finalizers, ServiceAccountCleanupFinalizer)
		obj.SetFinalizers(finalizers)
		return true
	}
	return false
}

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

func hasSAFinalizer(sa *corev1.ServiceAccount) bool {
	return hasFinalizer(sa.GetFinalizers(), ServiceAccountFinalizer)
}

func updateCondition(conditions []metav1.Condition, conditionType, status, reason, message string) []metav1.Condition {
	condition := metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionStatus(status),
		Reason:  reason,
		Message: message,
	}

	for i := range conditions {
		if conditions[i].Type == conditionType {
			conditions[i] = condition
			return conditions
		}
	}

	return append(conditions, condition)
}
