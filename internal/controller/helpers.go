/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/metrics"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	deleteCheckRequeueDelay = 2 * time.Second
)

type WSAResource interface {
	client.Object
	*agentoctopuscomv1beta1.WorkloadServiceAccount | *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount
}

func fetchResource[T WSAResource](ctx context.Context, c client.Client, req ctrl.Request, resource T) (T, error) {
	log := logf.FromContext(ctx)

	if err := c.Get(ctx, req.NamespacedName, resource); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.V(1).Info("Resource not found, likely deleted")
			var zero T
			return zero, nil
		}
		log.Error(err, "failed to get resource")
		return resource, err
	}

	return resource, nil
}

// handleDeletion manages the deletion flow for WSA/cWSA resources.
// It waits for staging to process the deletion before removing the finalizer.
// Returns (requeue, error) where requeue indicates if the reconcile should be retried.
func handleDeletion[T WSAResource](
	ctx context.Context,
	c client.Client,
	engine *rules.InMemoryEngine,
	resource T,
) (bool, error) {
	log := logf.FromContext(ctx)

	if !containsCleanupFinalizer(resource) {
		log.V(1).Info("Resource has no cleanup finalizer, allowing deletion",
			"name", resource.GetName(),
			"namespace", resource.GetNamespace())
		return false, nil
	}

	// Use full namespace/name key to correctly identify the resource.
	// For cluster-scoped resources (namespace is empty), this will just be the name.
	wsaKey := resource.GetName()
	if ns := resource.GetNamespace(); ns != "" {
		wsaKey = ns + "/" + resource.GetName()
	}

	// Wait for staging to process this deletion.
	// Staging excludes resources with DeletionTimestamp and will update saToWsaMap.
	// GC runs as part of the staging batch and will delete orphaned SAs.
	if engine.IsWSAInMaps(wsaKey) {
		log.Info("Waiting for staging to process deletion",
			"key", wsaKey)
		return true, nil
	}

	// Staging has processed. Safe to remove the finalizer.
	original := resource.DeepCopyObject().(client.Object)
	removeCleanupFinalizer(resource)

	if err := c.Patch(ctx, resource, client.MergeFrom(original)); err != nil {
		log.Error(err, "failed to remove finalizer")
		return false, err
	}

	log.Info("Cleanup complete, removed finalizer",
		"name", resource.GetName(),
		"namespace", resource.GetNamespace())
	return false, nil
}

func isBeingDeleted(obj client.Object) bool {
	return !obj.GetDeletionTimestamp().IsZero()
}

func ensureFinalizer[T WSAResource](ctx context.Context, c client.Client, resource T) error {
	if containsCleanupFinalizer(resource) {
		return nil
	}

	log := logf.FromContext(ctx)
	original := resource.DeepCopyObject().(client.Object)
	addCleanupFinalizer(resource)

	if err := c.Patch(ctx, resource, client.MergeFrom(original)); err != nil {
		log.Error(err, "failed to add finalizer")
		return err
	}

	log.V(1).Info("Added cleanup finalizer", "name", resource.GetName(), "namespace", resource.GetNamespace())
	return nil
}

func recordMetrics(resourceType string, startTime time.Time) {
	metrics.RecordReconciliationDurationFunc(resourceType, startTime)
}
