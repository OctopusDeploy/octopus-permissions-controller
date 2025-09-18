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

package v1beta1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

// nolint:unused
// log is for logging in this package.
var workloadserviceaccountlog = logf.Log.WithName("workloadserviceaccount-resource")

// SetupWorkloadServiceAccountWebhookWithManager registers the webhook for WorkloadServiceAccount in the manager.
func SetupWorkloadServiceAccountWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&agentoctopuscomv1beta1.WorkloadServiceAccount{}).
		WithValidator(&WorkloadServiceAccountCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-agent-octopus-com-v1beta1-workloadserviceaccount,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.octopus.com,resources=workloadserviceaccounts,verbs=create,versions=v1beta1,name=vworkloadserviceaccount-v1beta1.kb.io,admissionReviewVersions=v1

// WorkloadServiceAccountCustomValidator struct is responsible for validating the WorkloadServiceAccount resource
// when it is created.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkloadServiceAccountCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &WorkloadServiceAccountCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type WorkloadServiceAccount.
func (v *WorkloadServiceAccountCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	workloadserviceaccount, ok := obj.(*agentoctopuscomv1beta1.WorkloadServiceAccount)
	if !ok {
		return nil, fmt.Errorf("expected a WorkloadServiceAccount object but got %T", obj)
	}
	workloadserviceaccountlog.Info("Validation for WorkloadServiceAccount upon creation", "name", workloadserviceaccount.GetName())

	scope := workloadserviceaccount.Spec.Scope
	if len(scope.Projects)+len(scope.Environments)+len(scope.Tenants)+len(scope.Steps)+len(scope.Spaces) == 0 {
		return nil, fmt.Errorf("at least one scope must be defined (projects, environments, tenants, steps, or spaces)")
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator but is not used since webhook only handles create operations.
func (v *WorkloadServiceAccountCustomValidator) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator but is not used since webhook only handles create operations.
func (v *WorkloadServiceAccountCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
