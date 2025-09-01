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
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Pod Webhook", func() {
	var (
		pod       *corev1.Pod
		podScope  rules.Scope
		agentName rules.AgentName = "default"
	)

	BeforeEach(func() {
		pod = &corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				Labels: map[string]string{
					EnabledLabelKey: "true",
				},
				Annotations: map[string]string{
					ProjectAnnotationKey:     "my-project",
					EnvironmentAnnotationKey: "my-environment",
					TenantAnnotationKey:      "my-tenant",
					StepAnnotationKey:        "my-step",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:1.7.9",
					},
				},
				ServiceAccountName: "not-overridden",
			},
			Status: corev1.PodStatus{},
		}
		podScope = rules.Scope{
			Project:     "my-project",
			Environment: "my-environment",
			Tenant:      "my-tenant",
			Step:        "my-step",
		}
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
	})

	Context("When label is set", func() {
		It("Should inject a service account", func() {
			By("By creating a pod")

			mockCall := mockEngine.On("GetServiceAccountForScope", podScope, agentName).Return(rules.ServiceAccountName("overridden"), nil)
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			var actualPod corev1.Pod
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &actualPod)).To(Succeed())
			Expect(actualPod.Spec.ServiceAccountName).To(Equal("overridden"))
			mockEngine.AssertExpectations(GinkgoT())
			mockCall.Unset()
		})
		It("Should not inject a service account", func() {
			By("When no matching scope exists")
			mockCall := mockEngine.On("GetServiceAccountForScope", podScope, agentName).Return(rules.ServiceAccountName(""), nil)
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			var actualPod corev1.Pod
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &actualPod)).To(Succeed())
			Expect(actualPod.Spec.ServiceAccountName).To(Equal("not-overridden"))
			mockEngine.AssertExpectations(GinkgoT())
			mockCall.Unset()
		})
	})
})
