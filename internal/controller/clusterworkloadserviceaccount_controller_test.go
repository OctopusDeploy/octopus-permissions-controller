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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/reconciliation"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
)

var _ = Describe("ClusterWorkloadServiceAccount Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-cwsa-resource"

		ctx := context.Background()

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterWorkloadServiceAccount")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
							Projects:     []string{"project-a"},
							Environments: []string{"dev"},
						},
						Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
							Permissions: []rbacv1.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"pods"},
									Verbs:     []string{"get", "list"},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance ClusterWorkloadServiceAccount")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := reconciliation.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: resourceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a resource with multiple scope dimensions", func() {
		const resourceName = "test-cwsa-multiscope"

		ctx := context.Background()

		BeforeEach(func() {
			By("creating ClusterWorkloadServiceAccount with multiple scope dimensions")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
							Projects:     []string{"project-a", "project-b"},
							Environments: []string{"dev", "staging"},
							Tenants:      []string{"tenant-1"},
							Steps:        []string{"step-1"},
							Spaces:       []string{"space-1"},
						},
						Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
							Permissions: []rbacv1.PolicyRule{
								{
									APIGroups: []string{"apps"},
									Resources: []string{"deployments"},
									Verbs:     []string{"get", "list", "watch"},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the ClusterWorkloadServiceAccount")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile with multiple scope dimensions", func() {
			By("Reconciling the resource with complex scope")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := reconciliation.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: resourceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a resource with ClusterRole references", func() {
		const resourceName = "test-cwsa-clusterroles"

		ctx := context.Background()

		BeforeEach(func() {
			By("creating ClusterWorkloadServiceAccount with ClusterRole references")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
							Projects:     []string{"project-c"},
							Environments: []string{"prod"},
						},
						Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
							ClusterRoles: []rbacv1.RoleRef{
								{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "view",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the ClusterWorkloadServiceAccount")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile with ClusterRole references", func() {
			By("Reconciling the resource with ClusterRole refs")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := reconciliation.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: resourceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling with both ClusterRoles and inline permissions", func() {
		const resourceName = "test-cwsa-mixed"

		ctx := context.Background()

		BeforeEach(func() {
			By("creating ClusterWorkloadServiceAccount with mixed permissions")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
							Projects: []string{"project-mixed"},
						},
						Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
							ClusterRoles: []rbacv1.RoleRef{
								{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "ClusterRole",
									Name:     "edit",
								},
							},
							Permissions: []rbacv1.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"secrets"},
									Verbs:     []string{"get"},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the ClusterWorkloadServiceAccount")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile with mixed permissions", func() {
			By("Reconciling the resource with both ClusterRoles and inline permissions")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := reconciliation.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: resourceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
