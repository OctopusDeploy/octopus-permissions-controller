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

	"github.com/octopusdeploy/octopus-permissions-controller/internal/metrics"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
)

// WorkloadServiceAccountReconciler reconciles a WorkloadServiceAccount object
type WorkloadServiceAccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Engine rules.Engine
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *WorkloadServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	startTime := time.Now()
	controllerType := "workloadserviceaccount"
	defer metrics.RecordReconciliationDurationFunc(controllerType, startTime)

	log.Info("WorkloadServiceAccount reconciliation triggered", "name", req.Name, "namespace", req.Namespace)

	// Fetch the WorkloadServiceAccount instance
	wsa := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
	if err := r.Get(ctx, req.NamespacedName, wsa); err != nil {
		// Resource not found or error fetching
		if client.IgnoreNotFound(err) == nil {
			// Owned resources trigger reconciles when deleted but the parent WSA will probably be gone first
			log.V(1).Info("WorkloadServiceAccount not found, likely deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get WorkloadServiceAccount")
		return ctrl.Result{}, err
	}

	// Handle deletion with finalizer
	if !wsa.DeletionTimestamp.IsZero() {
		// Resource is being deleted
		if hasFinalizer(wsa.GetFinalizers(), ServiceAccountCleanupFinalizer) {
			log.Info("Cleaning up ServiceAccounts for deleted WorkloadServiceAccount")

			// Delete ServiceAccounts that are no longer needed by any WSA/cWSA
			// Pass the resource being deleted so it can be excluded from the calculation
			wsaResource := rules.NewWSAResource(wsa)
			result, err := r.Engine.CleanupServiceAccounts(ctx, wsaResource)
			if err != nil {
				log.Error(err, "failed to cleanup ServiceAccounts")
				return result, err
			}
			// Check if we need to requeue (e.g., ServiceAccounts still in use)
			if result.RequeueAfter > 0 || result.Requeue {
				log.Info("ServiceAccounts cleanup pending, will requeue", "requeue", result)
				return result, nil
			}

			// Refetch the object to get the latest ResourceVersion before removing finalizer
			// This prevents "Precondition failed" errors from stale objects
			if err := r.Get(ctx, req.NamespacedName, wsa); err != nil {
				if client.IgnoreNotFound(err) == nil {
					// Object was deleted while we were cleaning up
					log.V(1).Info("WorkloadServiceAccount deleted during cleanup")
					return ctrl.Result{}, nil
				}
				log.Error(err, "failed to refetch before removing finalizer")
				return ctrl.Result{}, err
			}

			// Double-check finalizer still exists after refetch
			if removeFinalizer(wsa) {
				if err := r.Update(ctx, wsa); err != nil {
					if client.IgnoreNotFound(err) == nil {
						log.V(1).Info("WorkloadServiceAccount deleted before finalizer removal")
						return ctrl.Result{}, nil
					}
					log.Error(err, "failed to remove finalizer")
					return ctrl.Result{}, err
				}
				log.Info("Successfully cleaned up ServiceAccounts and removed finalizer")
			}
		} else {
			log.V(1).Info("WorkloadServiceAccount being deleted but finalizer already removed")
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if addFinalizer(wsa) {
		if err := r.Update(ctx, wsa); err != nil {
			log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		log.Info("Added finalizer to WorkloadServiceAccount")

		// Refetch the object after update to get the latest ResourceVersion
		if err := r.Get(ctx, req.NamespacedName, wsa); err != nil {
			log.Error(err, "failed to refetch WorkloadServiceAccount after finalizer update")
			return ctrl.Result{}, err
		}
	}

	// Perform incremental reconciliation for this specific resource
	wsaResource := rules.NewWSAResource(wsa)
	if err := r.Engine.ReconcileResource(ctx, wsaResource); err != nil {
		log.Error(err, "failed to reconcile ServiceAccounts from WorkloadServiceAccount")

		// Set Ready=False condition on failure
		apimeta.SetStatusCondition(&wsa.Status.Conditions, metav1.Condition{
			Type:    ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonReconcileFailed,
			Message: err.Error(),
		})
		if statusErr := r.Status().Update(ctx, wsa); statusErr != nil {
			log.Error(statusErr, "failed to update status after reconciliation failure")
		}

		return ctrl.Result{}, err
	}

	// Set Ready=True condition on success
	apimeta.SetStatusCondition(&wsa.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonReconcileSuccess,
		Message: "All ServiceAccounts, Roles, and RoleBindings successfully reconciled",
	})
	if err := r.Status().Update(ctx, wsa); err != nil && !apierrors.IsConflict(err) {
		log.Error(err, "failed to update status after successful reconciliation")
		// Don't return error - reconciliation was successful even if status update failed
	}

	log.Info("Successfully reconciled WorkloadServiceAccounts")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.WorkloadServiceAccount{}).
		Owns(&rbacv1.Role{}, builder.WithPredicates(predicate.Funcs{
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Don't reconcile on owned resource deletion
				// The owner is already being deleted, so no need to reconcile
				return false
			},
		})).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(predicate.Funcs{
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Don't reconcile on owned resource deletion
				// The owner is already being deleted, so no need to reconcile
				return false
			},
		})).
		Watches(
			&corev1.ServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(r.mapServiceAccountToWSAs),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					// Don't reconcile on SA creation
					return false
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					// Only reconcile if deletion state changed
					oldSA, oldOk := e.ObjectOld.(*corev1.ServiceAccount)
					newSA, newOk := e.ObjectNew.(*corev1.ServiceAccount)
					if !oldOk || !newOk {
						return false
					}

					// Only trigger if SA just got marked for deletion OR finalizer was removed
					oldDeleting := !oldSA.DeletionTimestamp.IsZero()
					newDeleting := !newSA.DeletionTimestamp.IsZero()
					oldHasFinalizer := hasSAFinalizer(oldSA)
					newHasFinalizer := hasSAFinalizer(newSA)

					return (!oldDeleting && newDeleting) || (oldHasFinalizer && !newHasFinalizer)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					// Don't reconcile on actual deletion (already handled by Update)
					return false
				},
			}),
		).
		Named("workloadserviceaccount").
		Complete(r)
}

// mapServiceAccountToWSAs maps ServiceAccount events to WorkloadServiceAccount reconcile requests.
// When a ServiceAccount changes (especially when marked for deletion), this triggers reconciliation
// of all WSAs so they can complete the two-phase deletion process (remove finalizers if safe).
func (r *WorkloadServiceAccountReconciler) mapServiceAccountToWSAs(ctx context.Context, obj client.Object) []reconcile.Request {
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return nil
	}

	log := logf.FromContext(ctx)

	// Only reconcile for ServiceAccounts managed by this controller
	if sa.Labels[rules.ManagedByLabel] != rules.ManagedByValue {
		return nil
	}

	// Only reconcile if SA is being deleted (has deletion timestamp)
	if sa.DeletionTimestamp.IsZero() {
		return nil
	}

	// List all WorkloadServiceAccounts and enqueue reconcile requests for all of them
	// This ensures that when a ServiceAccount is marked for deletion, all WSAs get a chance
	// to run their garbage collection logic and remove finalizers if safe.
	wsaList := &agentoctopuscomv1beta1.WorkloadServiceAccountList{}
	if err := r.List(ctx, wsaList); err != nil {
		log.Error(err, "failed to list WorkloadServiceAccounts for ServiceAccount mapping")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(wsaList.Items))
	for i := range wsaList.Items {
		wsa := &wsaList.Items[i]

		// Skip WSAs that are already being deleted
		if !wsa.DeletionTimestamp.IsZero() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      wsa.Name,
				Namespace: wsa.Namespace,
			},
		})
	}

	if len(requests) > 0 {
		log.V(1).Info("Enqueueing WSAs for ServiceAccount deletion",
			"serviceAccount", sa.Name,
			"namespace", sa.Namespace,
			"wsaCount", len(requests))
	}

	return requests
}
