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

func (r *WorkloadServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	startTime := time.Now()
	defer recordMetrics("workloadserviceaccount", startTime)

	log.Info("WorkloadServiceAccount reconciliation triggered", "name", req.Name, "namespace", req.Namespace)

	wsa := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
	wsa, err := fetchResource(ctx, r.Client, req, wsa)
	if err != nil {
		return ctrl.Result{}, err
	}
	if wsa == nil {
		return ctrl.Result{}, nil
	}

	if isBeingDeleted(wsa) {
		wsaResource := rules.NewWSAResource(wsa)
		return handleDeletion(ctx, r.Client, req, wsa, r.Engine, wsaResource)
	}

	if ensureFinalizer(ctx, r.Client, wsa) {
		wsa, err = fetchResource(ctx, r.Client, req, wsa)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	wsaResource := rules.NewWSAResource(wsa)
	if err := r.Engine.ReconcileResource(ctx, wsaResource); err != nil {
		log.Error(err, "failed to reconcile ServiceAccounts from WorkloadServiceAccount")
		updateStatusOnFailure(ctx, r.Client, wsa, &wsa.Status, err)
		return ctrl.Result{}, err
	}

	updateStatusOnSuccess(ctx, r.Client, wsa, &wsa.Status,
		"All ServiceAccounts, Roles, and RoleBindings successfully reconciled")
	log.Info("Successfully reconciled WorkloadServiceAccount")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.WorkloadServiceAccount{}).
		Owns(&rbacv1.Role{}, builder.WithPredicates(SuppressOwnedResourceDeletes())).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(SuppressOwnedResourceDeletes())).
		Watches(
			&corev1.ServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(r.mapServiceAccountToWSAs),
			builder.WithPredicates(ServiceAccountDeletionPredicate()),
		).
		Named("workloadserviceaccount").
		Complete(r)
}

// mapServiceAccountToWSAs maps ServiceAccount events to WorkloadServiceAccount reconcile requests.
// When a ServiceAccount changes (especially when marked for deletion), this triggers reconciliation
// of all WSAs so they can complete the two-phase deletion process (remove finalizers if safe).
func (r *WorkloadServiceAccountReconciler) mapServiceAccountToWSAs(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	return MapServiceAccountToResources(
		ctx,
		obj,
		r.Client,
		&agentoctopuscomv1beta1.WorkloadServiceAccountList{},
		func(list *agentoctopuscomv1beta1.WorkloadServiceAccountList) []*agentoctopuscomv1beta1.WorkloadServiceAccount {
			result := make([]*agentoctopuscomv1beta1.WorkloadServiceAccount, len(list.Items))
			for i := range list.Items {
				result[i] = &list.Items[i]
			}
			return result
		},
		"WorkloadServiceAccount",
	)
}
