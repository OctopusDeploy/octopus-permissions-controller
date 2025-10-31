package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func MapServiceAccountToResources[T client.Object, L client.ObjectList](
	ctx context.Context,
	obj client.Object,
	c client.Client,
	listObj L,
	extractItems func(L) []T,
	resourceType string,
) []reconcile.Request {
	if !IsManagedServiceAccount(obj) || !IsServiceAccountBeingDeleted(obj) {
		return nil
	}

	log := logf.FromContext(ctx)
	if err := c.List(ctx, listObj); err != nil {
		log.Error(err, "failed to list resources for ServiceAccount mapping", "resourceType", resourceType)
		return nil
	}

	items := extractItems(listObj)
	requests := make([]reconcile.Request, 0, len(items))

	for i := range items {
		item := items[i]
		if !item.GetDeletionTimestamp().IsZero() {
			continue
		}

		namespacedName := types.NamespacedName{
			Name: item.GetName(),
		}
		if item.GetNamespace() != "" {
			namespacedName.Namespace = item.GetNamespace()
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: namespacedName,
		})
	}

	if len(requests) > 0 {
		sa := obj.(*corev1.ServiceAccount)
		log.V(1).Info("Enqueueing resources for ServiceAccount deletion",
			"serviceAccount", sa.Name,
			"namespace", sa.Namespace,
			"resourceType", resourceType,
			"count", len(requests))
	}

	return requests
}
