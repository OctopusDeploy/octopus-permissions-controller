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
		WithDefaulter(&WorkloadServiceAccountCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-agent-octopus-com-v1beta1-workloadserviceaccount,mutating=true,failurePolicy=fail,sideEffects=None,groups=agent.octopus.com,resources=workloadserviceaccounts,verbs=create;update,versions=v1beta1,name=mworkloadserviceaccount-v1beta1.kb.io,admissionReviewVersions=v1

// WorkloadServiceAccountCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind WorkloadServiceAccount when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WorkloadServiceAccountCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &WorkloadServiceAccountCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind WorkloadServiceAccount.
func (d *WorkloadServiceAccountCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	workloadserviceaccount, ok := obj.(*agentoctopuscomv1beta1.WorkloadServiceAccount)

	if !ok {
		return fmt.Errorf("expected an WorkloadServiceAccount object but got %T", obj)
	}
	workloadserviceaccountlog.Info("Defaulting for WorkloadServiceAccount", "name", workloadserviceaccount.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-agent-octopus-com-v1beta1-workloadserviceaccount,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.octopus.com,resources=workloadserviceaccounts,verbs=create;update,versions=v1beta1,name=vworkloadserviceaccount-v1beta1.kb.io,admissionReviewVersions=v1

// WorkloadServiceAccountCustomValidator struct is responsible for validating the WorkloadServiceAccount resource
// when it is created, updated, or deleted.
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
	if len(scope.Projects)+len(scope.Environments)+len(scope.Tenants)+len(scope.Steps) == 0 {
		return nil, fmt.Errorf("at least one scope must be defined (projects, environments, tenants, or steps)")
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type WorkloadServiceAccount.
func (v *WorkloadServiceAccountCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	workloadserviceaccount, ok := newObj.(*agentoctopuscomv1beta1.WorkloadServiceAccount)
	if !ok {
		return nil, fmt.Errorf("expected a WorkloadServiceAccount object for the newObj but got %T", newObj)
	}
	workloadserviceaccountlog.Info("Validation for WorkloadServiceAccount upon update", "name", workloadserviceaccount.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type WorkloadServiceAccount.
func (v *WorkloadServiceAccountCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workloadserviceaccount, ok := obj.(*agentoctopuscomv1beta1.WorkloadServiceAccount)
	if !ok {
		return nil, fmt.Errorf("expected a WorkloadServiceAccount object but got %T", obj)
	}
	workloadserviceaccountlog.Info("Validation for WorkloadServiceAccount upon deletion", "name", workloadserviceaccount.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
