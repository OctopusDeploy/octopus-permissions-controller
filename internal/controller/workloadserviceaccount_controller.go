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

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

// WorkloadServiceAccountReconciler reconciles a WorkloadServiceAccount object
type WorkloadServiceAccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Engine rules.Engine
}

// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *WorkloadServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("WorkloadServiceAccount reconciliation triggered")

	wsaList := &agentoctopuscomv1beta1.WorkloadServiceAccountList{}
	if err := r.List(ctx, wsaList, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "failed to list WorkloadServiceAccounts")
		return ctrl.Result{}, err
	}

	log.Info("Found WSAs in namespace", "count", len(wsaList.Items))

	for _, currentWSA := range wsaList.Items {
		for _, project := range currentWSA.Spec.Scope.Projects {
			log.Info("WSA has project scope", "wsa", currentWSA.Name, "project", project)
		}
		for _, environment := range currentWSA.Spec.Scope.Environments {
			log.Info("WSA has environment scope", "wsa", currentWSA.Name, "environment", environment)
		}
		for _, tenant := range currentWSA.Spec.Scope.Tenants {
			log.Info("WSA has tenant scope", "wsa", currentWSA.Name, "tenant", tenant)
		}
		for _, step := range currentWSA.Spec.Scope.Steps {
			log.Info("WSA has step scope", "wsa", currentWSA.Name, "step", step)
		}
		for _, space := range currentWSA.Spec.Scope.Spaces {
			log.Info("WSA has space scope", "wsa", currentWSA.Name, "space", space)
		}
	}

	log.Info("Successfully reconciled WorkloadServiceAccounts")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.WorkloadServiceAccount{}).
		Named("workloadserviceaccount").
		Complete(r)
}
