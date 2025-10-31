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

func (r *ClusterWorkloadServiceAccountReconciler) Reconcile(
	ctx context.Context, req ctrl.Request,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	startTime := time.Now()
	defer recordMetrics("clusterworkloadserviceaccount", startTime)

	log.Info("ClusterWorkloadServiceAccount reconciliation triggered", "name", req.Name)

	cwsa := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
	err := fetchResource(ctx, r.Client, req, cwsa)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cwsa == nil {
		return ctrl.Result{}, nil
	}

	if isBeingDeleted(cwsa) {
		cwsaResource := rules.NewClusterWSAResource(cwsa)
		return handleDeletion(ctx, r.Client, req, cwsa, r.Engine, cwsaResource)
	}

	if ensureFinalizer(ctx, r.Client, cwsa) {
		err = fetchResource(ctx, r.Client, req, cwsa)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	cwsaResource := rules.NewClusterWSAResource(cwsa)
	if err := r.Engine.ReconcileResource(ctx, cwsaResource); err != nil {
		log.Error(err, "failed to reconcile ServiceAccounts from ClusterWorkloadServiceAccount")
		updateStatusOnFailure(ctx, r.Client, cwsa, &cwsa.Status, err)
		return ctrl.Result{}, err
	}

	updateStatusOnSuccess(ctx, r.Client, cwsa, &cwsa.Status,
		"All ServiceAccounts, ClusterRoles, and ClusterRoleBindings successfully reconciled")
	log.Info("Successfully reconciled ClusterWorkloadServiceAccount")
	return ctrl.Result{}, nil
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
