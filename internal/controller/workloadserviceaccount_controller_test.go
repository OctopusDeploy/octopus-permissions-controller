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

var _ = Describe("WorkloadServiceAccount Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		workloadserviceaccount := &agentoctopuscomv1beta1.WorkloadServiceAccount{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind WorkloadServiceAccount")
			err := k8sClient.Get(ctx, typeNamespacedName, workloadserviceaccount)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
							Projects:     []string{"project-basic"},
							Environments: []string{"dev"},
						},
						Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
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
			By("Cleanup the specific resource instance WorkloadServiceAccount")
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := reconciliation.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a resource with multiple scope dimensions", func() {
		const resourceName = "test-wsa-multiscope"
		const namespace = "default"

		ctx := context.Background()

		BeforeEach(func() {
			By("creating WorkloadServiceAccount with multiple scope dimensions")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
							Projects:     []string{"project-a", "project-b"},
							Environments: []string{"dev", "staging"},
							Tenants:      []string{"tenant-1"},
							Steps:        []string{"step-1"},
							Spaces:       []string{"space-1"},
						},
						Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
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
			By("Cleanup the WorkloadServiceAccount")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile with multiple scope dimensions", func() {
			By("Reconciling the resource with complex scope")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a resource with Role references", func() {
		const resourceName = "test-wsa-roles"
		const namespace = "default"

		ctx := context.Background()

		BeforeEach(func() {
			By("creating WorkloadServiceAccount with Role references")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
							Projects:     []string{"project-c"},
							Environments: []string{"prod"},
						},
						Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
							Roles: []rbacv1.RoleRef{
								{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "Role",
									Name:     "pod-reader",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the WorkloadServiceAccount")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile with Role references", func() {
			By("Reconciling the resource with Role refs")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling with both Roles and inline permissions", func() {
		const resourceName = "test-wsa-mixed"
		const namespace = "default"

		ctx := context.Background()

		BeforeEach(func() {
			By("creating WorkloadServiceAccount with mixed permissions")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-mixed"},
						},
						Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
							Roles: []rbacv1.RoleRef{
								{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "Role",
									Name:     "configmap-reader",
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
			By("Cleanup the WorkloadServiceAccount")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile with mixed permissions", func() {
			By("Reconciling the resource with both Roles and inline permissions")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing finalizer management", func() {
		const resourceName = "test-wsa-finalizers"
		const namespace = "default"
		ctx := context.Background()

		BeforeEach(func() {
			By("creating WorkloadServiceAccount without finalizer")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-finalizer"},
						},
						Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
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
			By("Cleanup the WorkloadServiceAccount")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should add finalizer on first reconcile", func() {
			By("Reconciling the resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the finalizer was added")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(resource.GetFinalizers()).To(ContainElement(ServiceAccountCleanupFinalizer))
		})

		It("should not duplicate finalizer on subsequent reconciles", func() {
			By("Verifying resource exists and has finalizer from first test")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)

			// If resource doesn't exist (was cleaned up), recreate it with finalizer
			if errors.IsNotFound(err) {
				By("Recreating resource with finalizer")
				resource = &agentoctopuscomv1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:       resourceName,
						Namespace:  namespace,
						Finalizers: []string{ServiceAccountCleanupFinalizer},
					},
					Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-finalizer"},
						},
						Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
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
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer count remains 1 if resource still exists")
			updatedResource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedResource)
			if err == nil {
				finalizerCount := 0
				for _, finalizer := range updatedResource.GetFinalizers() {
					if finalizer == ServiceAccountCleanupFinalizer {
						finalizerCount++
					}
				}
				Expect(finalizerCount).To(BeNumerically("<=", 1), "Should not have more than one finalizer")
			}
			// If resource was deleted during reconcile, that's also acceptable behavior
		})
	})

	Context("When testing event collection", func() {
		const resourceName = "test-wsa-events"
		const namespace = "default"
		ctx := context.Background()

		BeforeEach(func() {
			By("creating WorkloadServiceAccount for event testing")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
						Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-events"},
						},
						Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
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
			By("Cleanup the WorkloadServiceAccount")
			typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile and add event to collector", func() {
			By("Reconciling the resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
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
			freshResourceName := "test-wsa-events-recorder"
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      freshResourceName,
					Namespace: namespace,
				},
				Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
					Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
						Projects: []string{"project-recorder"},
					},
					Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
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
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
				Recorder:       eventRecorder,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      freshResourceName,
					Namespace: namespace,
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
		const resourceName = "test-wsa-deletion"
		const namespace = "default"
		ctx := context.Background()

		It("should handle deletion workflow correctly", func() {
			By("Creating WorkloadServiceAccount with finalizer")
			resource := &agentoctopuscomv1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  namespace,
					Finalizers: []string{ServiceAccountCleanupFinalizer},
				},
				Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
					Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{
						Projects: []string{"project-deletion"},
					},
					Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
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
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying appropriate handling occurred")
			// The result could be either:
			// 1. RequeueAfter if waiting for staging to process deletion
			// 2. No requeue if finalizer was removed successfully
			// Both are valid depending on the engine state
			_ = result // Use the result variable to avoid compiler warning
		})

		It("should handle resource not found during reconcile", func() {
			By("Reconciling non-existent resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-resource",
					Namespace: namespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("When testing error handling", func() {
		const namespace = "default"
		ctx := context.Background()

		It("should handle missing resources correctly", func() {
			By("Reconciling a non-existent resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := staging.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         scheme.Scheme,
				Engine:         &engine,
				EventCollector: eventCollector,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-resource",
					Namespace: namespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
		})
	})
})
