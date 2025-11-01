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
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type WSAResource interface {
	client.Object
	*agentoctopuscomv1beta1.WorkloadServiceAccount | *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount
}

type WSAStatus interface {
	*agentoctopuscomv1beta1.WorkloadServiceAccountStatus | *agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus
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

func ensureFinalizer[T client.Object](ctx context.Context, c client.Client, resource T) bool {
	log := logf.FromContext(ctx)

	if addFinalizer(resource) {
		if err := c.Update(ctx, resource); err != nil {
			log.Error(err, "failed to add finalizer")
			return false
		}
		log.Info("Added finalizer to resource")
		return true
	}
	return false
}

func handleDeletion[T WSAResource](
	ctx context.Context,
	c client.Client,
	req ctrl.Request,
	resource T,
	engine rules.Engine,
	wsaResource rules.WSAResource,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Entering deletion handler",
		"name", resource.GetName(),
		"namespace", resource.GetNamespace(),
		"hasFinalizer", hasFinalizer(resource.GetFinalizers(), ServiceAccountCleanupFinalizer))

	if !hasFinalizer(resource.GetFinalizers(), ServiceAccountCleanupFinalizer) {
		log.V(1).Info("Resource being deleted but finalizer already removed")
		return ctrl.Result{}, nil
	}

	log.Info("Cleaning up ServiceAccounts for deleted resource")

	result, err := engine.CleanupServiceAccounts(ctx, wsaResource)
	if err != nil {
		log.Error(err, "failed to cleanup ServiceAccounts")
		return result, err
	}

	if result.RequeueAfter > 0 {
		log.Info("ServiceAccounts cleanup pending, will requeue",
			"requeueAfter", result.RequeueAfter)
		return result, nil
	}

	log.Info("ServiceAccounts cleanup complete, removing finalizer")

	if err := c.Get(ctx, req.NamespacedName, resource); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.V(1).Info("Resource deleted during cleanup")
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to refetch before removing finalizer")
		return ctrl.Result{}, err
	}

	if removeFinalizer(resource) {
		if err := c.Update(ctx, resource); err != nil {
			if client.IgnoreNotFound(err) == nil {
				log.V(1).Info("Resource deleted before finalizer removal")
				return ctrl.Result{}, nil
			}
			log.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		log.Info("Successfully cleaned up ServiceAccounts and removed finalizer",
			"name", resource.GetName(),
			"namespace", resource.GetNamespace())
	}

	return ctrl.Result{}, nil
}

func updateStatusOnFailure[T WSAResource, S WSAStatus](
	ctx context.Context,
	c client.Client,
	resource T,
	status S,
	err error,
) {
	log := logf.FromContext(ctx)

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if getErr := c.Get(ctx, client.ObjectKeyFromObject(resource), resource); getErr != nil {
			return getErr
		}

		switch s := any(status).(type) {
		case *agentoctopuscomv1beta1.WorkloadServiceAccountStatus:
			s.Conditions = updateCondition(s.Conditions, ConditionTypeReady,
				"False", ReasonReconcileFailed, err.Error())
		case *agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus:
			s.Conditions = updateCondition(s.Conditions, ConditionTypeReady,
				"False", ReasonReconcileFailed, err.Error())
		}

		return c.Status().Update(ctx, resource)
	})

	if retryErr != nil {
		log.Error(retryErr, "failed to update status after reconciliation failure")
	}
}

func updateStatusOnSuccess[T WSAResource, S WSAStatus](
	ctx context.Context,
	c client.Client,
	resource T,
	status S,
	successMessage string,
) {
	log := logf.FromContext(ctx)

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if getErr := c.Get(ctx, client.ObjectKeyFromObject(resource), resource); getErr != nil {
			return getErr
		}

		switch s := any(status).(type) {
		case *agentoctopuscomv1beta1.WorkloadServiceAccountStatus:
			s.Conditions = updateCondition(s.Conditions, ConditionTypeReady,
				"True", ReasonReconcileSuccess, successMessage)
		case *agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus:
			s.Conditions = updateCondition(s.Conditions, ConditionTypeReady,
				"True", ReasonReconcileSuccess, successMessage)
		}

		return c.Status().Update(ctx, resource)
	})

	if retryErr != nil {
		log.Error(retryErr, "failed to update status after successful reconciliation")
	}
}

func isBeingDeleted(obj client.Object) bool {
	return !obj.GetDeletionTimestamp().IsZero()
}

func recordMetrics(resourceType string, startTime time.Time) {
	metrics.RecordReconciliationDurationFunc(resourceType, startTime)
}
