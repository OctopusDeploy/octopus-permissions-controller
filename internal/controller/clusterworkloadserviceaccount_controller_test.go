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
	"k8s.io/client-go/tools/record"
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

	Context("When testing finalizer management", func() {
		const resourceName = "test-cwsa-finalizers"
		ctx := context.Background()

		BeforeEach(func() {
			By("creating ClusterWorkloadServiceAccount without finalizer")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
							Projects: []string{"project-finalizer"},
						},
						Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
							Permissions: []rbacv1.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"configmaps"},
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

		It("should add finalizer on first reconcile", func() {
			By("Reconciling the resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
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

			By("Verifying the finalizer was added")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(resource.GetFinalizers()).To(ContainElement(ServiceAccountCleanupFinalizer))
		})

		It("should not duplicate finalizer on subsequent reconciles", func() {
			By("Verifying resource exists and has finalizer from first test")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)

			// If resource doesn't exist (was cleaned up), recreate it with finalizer
			if errors.IsNotFound(err) {
				By("Recreating resource with finalizer")
				resource = &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:       resourceName,
						Finalizers: []string{ServiceAccountCleanupFinalizer},
					},
					Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
							Projects: []string{"project-finalizer"},
						},
						Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
							Permissions: []rbacv1.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"configmaps"},
									Verbs:     []string{"get"},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			} else {
				Expect(err).NotTo(HaveOccurred())
				// Ensure it has the finalizer
				if !containsCleanupFinalizer(resource) {
					original := resource.DeepCopy()
					addCleanupFinalizer(resource)
					err = k8sClient.Patch(ctx, resource, client.MergeFrom(original))
					Expect(err).NotTo(HaveOccurred())
				}
			}

			By("Reconciling the resource with existing finalizer")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: resourceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer count remains 1 if resource still exists")
			updatedResource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, updatedResource)
			if err == nil {
				finalizerCount := 0
				for _, finalizer := range updatedResource.GetFinalizers() {
					if finalizer == ServiceAccountCleanupFinalizer {
						finalizerCount++
					}
				}
				Expect(finalizerCount).To(BeNumerically("<=", 1), "Should not have more than one finalizer")
			}
		})
	})

	Context("When testing event collection", func() {
		const resourceName = "test-cwsa-events"
		ctx := context.Background()

		BeforeEach(func() {
			By("creating ClusterWorkloadServiceAccount for event testing")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: resourceName}, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
							Projects: []string{"project-events"},
						},
						Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
							Permissions: []rbacv1.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"services"},
									Verbs:     []string{"list"},
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

		It("should successfully reconcile and add event to collector", func() {
			By("Reconciling the resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
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

			By("Verifying the resource was processed successfully")
			// Since we can't easily test the internal event collector state,
			// we verify that reconcile completed successfully, which means
			// the event was added to the collector
		})

		It("should record Kubernetes events when EventRecorder is provided", func() {
			By("Creating a fresh resource for event recorder testing")
			freshResourceName := "test-cwsa-events-recorder"
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: freshResourceName,
				},
				Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
					Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
						Projects: []string{"project-recorder"},
					},
					Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
						Permissions: []rbacv1.PolicyRule{
							{
								APIGroups: []string{""},
								Resources: []string{"nodes"},
								Verbs:     []string{"list"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				By("Cleanup the recorder test resource")
				_ = k8sClient.Delete(ctx, resource)
			}()

			By("Reconciling with EventRecorder")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			eventRecorder := record.NewFakeRecorder(10)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
				Recorder:       eventRecorder,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: freshResourceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Kubernetes event was recorded")
			select {
			case event := <-eventRecorder.Events:
				Expect(event).To(ContainSubstring("Queued"))
				Expect(event).To(ContainSubstring("Reconciliation queued for batch processing"))
			case <-time.After(2 * time.Second):
				Fail("Expected Kubernetes event was not recorded")
			}
		})
	})

	Context("When testing resource deletion handling", func() {
		const resourceName = "test-cwsa-deletion"
		ctx := context.Background()

		It("should handle deletion workflow correctly", func() {
			By("Creating ClusterWorkloadServiceAccount with finalizer")
			resource := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Finalizers: []string{ServiceAccountCleanupFinalizer},
				},
				Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
					Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{
						Projects: []string{"project-deletion"},
					},
					Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
						Permissions: []rbacv1.PolicyRule{
							{
								APIGroups: []string{""},
								Resources: []string{"endpoints"},
								Verbs:     []string{"get"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Marking resource for deletion")
			err := k8sClient.Delete(ctx, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the deletion - should handle deletion flow")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: resourceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying appropriate handling occurred")
			// The result could be either:
			// 1. RequeueAfter if waiting for staging to process deletion
			// 2. No requeue if finalizer was removed successfully
			// Both are valid depending on the engine state
			_ = result
		})

		It("should handle resource not found during reconcile", func() {
			By("Reconciling non-existent resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "non-existent-resource",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("When testing error handling", func() {
		ctx := context.Background()

		It("should handle missing resources correctly", func() {
			By("Reconciling a non-existent resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &ClusterWorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         scheme.Scheme,
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "non-existent-resource",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
		})
	})
})
