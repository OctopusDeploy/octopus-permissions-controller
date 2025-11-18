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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type WorkloadServiceAccountReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Engine         rules.Engine
	EventCollector *staging.EventCollector
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

	event := &staging.EventInfo{
		Resource:        wsaResource,
		EventType:       staging.EventTypeUpdate,
		Generation:      wsa.GetGeneration(),
		ResourceVersion: wsa.GetResourceVersion(),
		Timestamp:       time.Now(),
	}

	if r.EventCollector.AddEvent(event) {
		log.V(1).Info("Event added to collector", "generation", wsa.GetGeneration())
	} else {
		log.V(1).Info("Event deduplicated", "generation", wsa.GetGeneration())
	}

	updateStatusOnSuccess(ctx, r.Client, wsa, &wsa.Status,
		"Event queued for batch reconciliation")
	log.Info("Successfully reconciled WorkloadServiceAccount")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.WorkloadServiceAccount{}).
		Named("workloadserviceaccount").
		Complete(r)
}
