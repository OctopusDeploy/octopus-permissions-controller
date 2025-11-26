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
	"github.com/octopusdeploy/octopus-permissions-controller/internal/staging"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type WorkloadServiceAccountReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Engine         *rules.InMemoryEngine
	EventCollector *staging.EventCollector
	Recorder       record.EventRecorder
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
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
		log.V(1).Info("Resource not found", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	wsaResource := rules.NewWSAResource(wsa)

	if isBeingDeleted(wsa) {
		deleteEvent := &staging.EventInfo{
			Resource:        wsaResource,
			EventType:       staging.EventTypeDelete,
			Generation:      wsa.GetGeneration(),
			ResourceVersion: wsa.GetResourceVersion(),
			Timestamp:       time.Now(),
		}
		if r.EventCollector.AddEvent(deleteEvent) {
			log.Info("Delete event queued for batch processing", "name", wsa.Name, "namespace", wsa.Namespace)
		}

		requeue, err := handleDeletion(ctx, r.Client, r.Engine, wsa)
		if err != nil {
			return ctrl.Result{}, err
		}
		if requeue {
			return ctrl.Result{RequeueAfter: deleteCheckRequeueDelay}, nil
		}
		return ctrl.Result{}, nil
	}

	if err := ensureFinalizer(ctx, r.Client, wsa); err != nil {
		return ctrl.Result{}, err
	}

	eventType := staging.EventTypeUpdate
	if wsa.GetGeneration() == 1 {
		eventType = staging.EventTypeCreate
	}

	event := &staging.EventInfo{
		Resource:        wsaResource,
		EventType:       eventType,
		Generation:      wsa.GetGeneration(),
		ResourceVersion: wsa.GetResourceVersion(),
		Timestamp:       time.Now(),
	}

	if r.EventCollector.AddEvent(event) {
		log.V(1).Info("Event added to collector", "generation", wsa.GetGeneration())
		if r.Recorder != nil {
			r.Recorder.Event(wsa, corev1.EventTypeNormal, "Queued", "Reconciliation queued for batch processing")
		}
	} else {
		log.V(1).Info("Event deduplicated", "generation", wsa.GetGeneration())
	}

	log.Info("Successfully queued WorkloadServiceAccount for reconciliation")
	return ctrl.Result{}, nil
}

func (r *WorkloadServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.WorkloadServiceAccount{},
			builder.WithPredicates(GenerationOrDeletePredicate())).
		Owns(&rbacv1.ClusterRole{}, builder.WithPredicates(OwnedResourcePredicate(), GenerationOrDeletePredicate())).
		Owns(&rbacv1.ClusterRoleBinding{}, builder.WithPredicates(OwnedResourcePredicate(), GenerationOrDeletePredicate())).
		Watches(
			&corev1.ServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(r.mapServiceAccountToWSAs),
			builder.WithPredicates(ManagedServiceAccountPredicate(), GenerationOrDeletePredicate()),
		).
		Named("workloadserviceaccount").
		Complete(r)
}

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
