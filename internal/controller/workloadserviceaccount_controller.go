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
}

// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.octopus.com,resources=workloadserviceaccounts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the WorkloadServiceAccount object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *WorkloadServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var wsa agentoctopuscomv1beta1.WorkloadServiceAccount
	if err := r.Get(ctx, req.NamespacedName, &wsa); err != nil {
		log.Error(err, "unable to fetch WorkloadServiceAccount")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	for _, project := range wsa.Spec.Scope.Projects {
		log.Info("We have a project scope", "project", project)
	}

	for _, environment := range wsa.Spec.Scope.Environments {
		log.Info("We have an environment scope", "project", environment)
	}

	for _, tenant := range wsa.Spec.Scope.Tenants {
		log.Info("We have a tenant scope", "tenant", tenant)
	}

	for _, step := range wsa.Spec.Scope.Steps {
		log.Info("We have a step scope", "step", step)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentoctopuscomv1beta1.WorkloadServiceAccount{}).
		Named("workloadserviceaccount").
		Complete(r)
}
