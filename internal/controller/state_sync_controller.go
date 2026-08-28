package controller

import (
	"context"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/reconciliation"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// StateSyncReconciler triggers a state refresh when a WSA or CWSA changes. Unlike the
// reconcilers that write to the cluster, it runs on every replica, because the pod
// webhook is served by non-leaders too.
type StateSyncReconciler struct {
	Syncer *reconciliation.StateSyncer
}

// Reconcile ignores the request; the syncer rebuilds everything, so which resource
// changed doesn't matter.
func (r *StateSyncReconciler) Reconcile(_ context.Context, _ ctrl.Request) (ctrl.Result, error) {
	r.Syncer.Trigger()
	return ctrl.Result{}, nil
}

func (r *StateSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// both kinds have a status subresource, so filtering on generation keeps the leader's
	// status writes from rebuilding state on every replica
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.WorkloadServiceAccount{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(
			&agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(enqueueSelf),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{NeedLeaderElection: new(false)}).
		Named("statesync").
		Complete(r)
}

func enqueueSelf(_ context.Context, obj client.Object) []reconcile.Request {
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()},
	}}
}
