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

package v1

import (
	"context"
	"fmt"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	EnabledLabelKey          = "agent.octopus.com/permissions"
	ProjectAnnotationKey     = "agent.octopus.com/project"
	EnvironmentAnnotationKey = "agent.octopus.com/environment"
	TenantAnnotationKey      = "agent.octopus.com/tenant"
	StepAnnotationKey        = "agent.octopus.com/step"
	SpaceAnnotationKey       = "agent.octopus.com/space"
)

// nolint:unused
// log is for logging in this package.
var podlog = logf.Log.WithName("pod-resource")

// SetupPodWebhookWithManager registers the webhook for Pod in the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager, engine rules.Engine) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&corev1.Pod{}).
		WithDefaulter(&PodCustomDefaulter{
			engine,
		}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1

// PodCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Pod when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type PodCustomDefaulter struct {
	engine rules.Engine
}

var _ webhook.CustomDefaulter = &PodCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Pod.
func (d *PodCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected an Pod object but got %T", obj)
	}

	// Only run if labelled
	if !d.shouldRunOnPod(ctx, pod) {
		return nil
	}

	podlog.Info("Getting scope for pod", "name", pod.GetName())

	scope := getPodScope(pod)

	if scope.IsEmpty() {
		return nil
	}

	serviceAccountName, err := d.engine.GetServiceAccountForScope(scope)
	if err == nil && serviceAccountName != "" {
		podlog.Info("Setting service account for pod", "name", pod.GetName(), "originalServiceAccount", pod.Spec.ServiceAccountName, "newServiceAccount", serviceAccountName)
		pod.Spec.ServiceAccountName = string(serviceAccountName)
	}
	return err
}

func (d *PodCustomDefaulter) shouldRunOnPod(_ context.Context, p *corev1.Pod) bool {
	// This condition should always be true, as our webhook configuration only selects pods with the label.
	// this is here for safety and to allow an easy entry point for further logic if necessary
	if val, ok := p.Labels[EnabledLabelKey]; ok {
		if val == "enabled" {
			return true
		}
	}
	return false
}

func getPodScope(p *corev1.Pod) rules.Scope {
	scope := rules.Scope{}

	if project, ok := p.Annotations[ProjectAnnotationKey]; ok {
		scope.Project = project
	}
	if environment, ok := p.Annotations[EnvironmentAnnotationKey]; ok {
		scope.Environment = environment
	}
	if tenant, ok := p.Annotations[TenantAnnotationKey]; ok {
		scope.Tenant = tenant
	}
	if step, ok := p.Annotations[StepAnnotationKey]; ok {
		scope.Step = step
	}
	if space, ok := p.Annotations[SpaceAnnotationKey]; ok {
		scope.Space = space
	}

	return scope
}
