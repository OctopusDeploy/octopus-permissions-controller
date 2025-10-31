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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ClusterWorkloadServiceAccountReconciler reconciles a ClusterWorkloadServiceAccount object
type ClusterWorkloadServiceAccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Engine rules.Engine
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.octopus.com,resources=clusterworkloadserviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.octopus.com,resources=clusterworkloadserviceaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.octopus.com,resources=clusterworkloadserviceaccounts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ClusterWorkloadServiceAccountReconciler) Reconcile(
	ctx context.Context, req ctrl.Request,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	startTime := time.Now()
	defer metrics.RecordReconciliationDurationFunc("clusterworkloadserviceaccount", startTime)

	log.Info("ClusterWorkloadServiceAccount reconciliation triggered", "name", req.Name)

	cwsa, err := r.fetchResource(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cwsa == nil {
		return ctrl.Result{}, nil
	}

	if r.isBeingDeleted(cwsa) {
		return r.handleDeletion(ctx, req, cwsa)
	}

	if r.ensureFinalizer(ctx, cwsa) {
		cwsa, err = r.fetchResource(ctx, req)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileResource(ctx, cwsa); err != nil {
		r.updateStatusOnFailure(ctx, cwsa, err)
		return ctrl.Result{}, err
	}

	r.updateStatusOnSuccess(ctx, cwsa)
	log.Info("Successfully reconciled ClusterWorkloadServiceAccount")
	return ctrl.Result{}, nil
}

func (r *ClusterWorkloadServiceAccountReconciler) fetchResource(
	ctx context.Context, req ctrl.Request,
) (*agentoctopuscomv1beta1.ClusterWorkloadServiceAccount, error) {
	log := logf.FromContext(ctx)
	cwsa := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}

	if err := r.Get(ctx, req.NamespacedName, cwsa); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.V(1).Info("ClusterWorkloadServiceAccount not found, likely deleted")
			return nil, nil
		}
		log.Error(err, "failed to get ClusterWorkloadServiceAccount")
		return nil, err
	}

	return cwsa, nil
}

func (r *ClusterWorkloadServiceAccountReconciler) isBeingDeleted(cwsa *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount) bool {
	return !cwsa.DeletionTimestamp.IsZero()
}

func (r *ClusterWorkloadServiceAccountReconciler) handleDeletion(
	ctx context.Context, req ctrl.Request, cwsa *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !hasFinalizer(cwsa.GetFinalizers(), ServiceAccountCleanupFinalizer) {
		log.V(1).Info("Resource being deleted but finalizer already removed")
		return ctrl.Result{}, nil
	}

	log.Info("Cleaning up ServiceAccounts for deleted ClusterWorkloadServiceAccount")

	cwsaResource := rules.NewClusterWSAResource(cwsa)
	result, err := r.Engine.CleanupServiceAccounts(ctx, cwsaResource)
	if err != nil {
		log.Error(err, "failed to cleanup ServiceAccounts")
		return result, err
	}

	if result.RequeueAfter > 0 || result.Requeue {
		log.Info("ServiceAccounts cleanup pending, will requeue", "requeue", result)
		return result, nil
	}

	if err := r.Get(ctx, req.NamespacedName, cwsa); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.V(1).Info("ClusterWorkloadServiceAccount deleted during cleanup")
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to refetch before removing finalizer")
		return ctrl.Result{}, err
	}

	if removeFinalizer(cwsa) {
		if err := r.Update(ctx, cwsa); err != nil {
			if client.IgnoreNotFound(err) == nil {
				log.V(1).Info("ClusterWorkloadServiceAccount deleted before finalizer removal")
				return ctrl.Result{}, nil
			}
			log.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		log.Info("Successfully cleaned up ServiceAccounts and removed finalizer")
	}

	return ctrl.Result{}, nil
}

func (r *ClusterWorkloadServiceAccountReconciler) ensureFinalizer(
	ctx context.Context, cwsa *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount,
) bool {
	log := logf.FromContext(ctx)

	if addFinalizer(cwsa) {
		if err := r.Update(ctx, cwsa); err != nil {
			log.Error(err, "failed to add finalizer")
			return false
		}
		log.Info("Added finalizer to ClusterWorkloadServiceAccount")
		return true
	}
	return false
}

func (r *ClusterWorkloadServiceAccountReconciler) reconcileResource(
	ctx context.Context, cwsa *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount,
) error {
	log := logf.FromContext(ctx)
	cwsaResource := rules.NewClusterWSAResource(cwsa)

	if err := r.Engine.ReconcileResource(ctx, cwsaResource); err != nil {
		log.Error(err, "failed to reconcile ServiceAccounts from ClusterWorkloadServiceAccount")
		return err
	}

	return nil
}

func (r *ClusterWorkloadServiceAccountReconciler) updateStatusOnFailure(
	ctx context.Context, cwsa *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount, err error,
) {
	log := logf.FromContext(ctx)

	cwsa.Status.Conditions = updateCondition(cwsa.Status.Conditions, ConditionTypeReady,
		"False", ReasonReconcileFailed, err.Error())

	if statusErr := r.Status().Update(ctx, cwsa); statusErr != nil {
		log.Error(statusErr, "failed to update status after reconciliation failure")
	}
}

func (r *ClusterWorkloadServiceAccountReconciler) updateStatusOnSuccess(
	ctx context.Context, cwsa *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount,
) {
	log := logf.FromContext(ctx)

	cwsa.Status.Conditions = updateCondition(cwsa.Status.Conditions, ConditionTypeReady,
		"True", ReasonReconcileSuccess, "All ServiceAccounts, ClusterRoles, and ClusterRoleBindings successfully reconciled")

	if err := r.Status().Update(ctx, cwsa); err != nil {
		log.Error(err, "failed to update status after successful reconciliation")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterWorkloadServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}).
		Owns(&rbacv1.ClusterRoleBinding{}, builder.WithPredicates(SuppressOwnedResourceDeletes())).
		Watches(
			&corev1.ServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(r.mapServiceAccountToCWSAs),
			builder.WithPredicates(ServiceAccountDeletionPredicate()),
		).
		Named("clusterworkloadserviceaccount").
		Complete(r)
}

// mapServiceAccountToCWSAs maps ServiceAccount events to ClusterWorkloadServiceAccount reconcile requests.
// When a ServiceAccount changes (especially when marked for deletion), this triggers reconciliation
// of all related CWSAs so they can complete the two-phase deletion process (remove finalizers if safe).
func (r *ClusterWorkloadServiceAccountReconciler) mapServiceAccountToCWSAs(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	return MapServiceAccountToResources(
		ctx,
		obj,
		r.Client,
		&agentoctopuscomv1beta1.ClusterWorkloadServiceAccountList{},
		func(list *agentoctopuscomv1beta1.ClusterWorkloadServiceAccountList) []*agentoctopuscomv1beta1.ClusterWorkloadServiceAccount {
			result := make([]*agentoctopuscomv1beta1.ClusterWorkloadServiceAccount, len(list.Items))
			for i := range list.Items {
				result[i] = &list.Items[i]
			}
			return result
		},
		"ClusterWorkloadServiceAccount",
	)
}
