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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

// nolint:unused
// log is for logging in this package.
var workloadserviceaccountlog = logf.Log.WithName("workloadserviceaccount-resource")

// SetupWorkloadServiceAccountWebhookWithManager registers the webhook for WorkloadServiceAccount in the manager.
func SetupWorkloadServiceAccountWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &agentoctopuscomv1beta1.WorkloadServiceAccount{}).
		WithValidator(&WorkloadServiceAccountCustomValidator{}).
		Complete()
}

type WorkloadServiceAccountCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type WorkloadServiceAccount.
func (v *WorkloadServiceAccountCustomValidator) ValidateCreate(
	_ context.Context, obj *agentoctopuscomv1beta1.WorkloadServiceAccount,
) (admission.Warnings, error) {
	workloadserviceaccountlog.Info("Validation for WorkloadServiceAccount upon creation", "name", obj.GetName())

	scope := obj.Spec.Scope
	if len(scope.Projects)+len(scope.ProjectGroups)+len(scope.Environments)+len(scope.Tenants)+len(scope.Steps)+len(scope.Spaces) == 0 {
		return nil, fmt.Errorf("at least one scope must be defined (projects, project-groups, environments, tenants, steps, or spaces)")
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator but is not used since webhook only handles create operations.
func (v *WorkloadServiceAccountCustomValidator) ValidateUpdate(
	_ context.Context, _, _ runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator but is not used since webhook only handles create operations.
func (v *WorkloadServiceAccountCustomValidator) ValidateDelete(
	_ context.Context, _ runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
