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

package rules

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/test/helpers"
)

var _ = Describe("Partial Update Advanced Tests", func() {
	var (
		engine           InMemoryEngine
		targetNamespaces []string
		testNamespace    string
		cleanNamespaces  []*corev1.Namespace
		cleanResources   []client.Object
		testCtx          context.Context
		testCancel       context.CancelFunc
		scheme           *runtime.Scheme
	)

	BeforeEach(func() {
		// Create a fresh context for each test with a timeout
		testCtx, testCancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Initialize scheme
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = rbacv1.AddToScheme(scheme)
		_ = agentoctopuscomv1beta1.AddToScheme(scheme)

		// Reset cleanup tracking
		cleanNamespaces = []*corev1.Namespace{}
		cleanResources = []client.Object{}

		// Create unique test namespaces for each test
		timestamp := time.Now().UnixNano()
		ns1Name := fmt.Sprintf("test-partial-%d", timestamp)
		ns2Name := fmt.Sprintf("test-partial2-%d", timestamp)
		testNamespace = ns1Name
		targetNamespaces = []string{ns1Name, ns2Name}

		// Create test namespaces
		for _, ns := range targetNamespaces {
			namespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: ns},
			}
			cleanNamespaces = append(cleanNamespaces, &namespace)
			Expect(k8sClient.Create(testCtx, &namespace)).To(Succeed())

			// Wait for namespace to be active
			Eventually(func() bool {
				createdNs := &corev1.Namespace{}
				err := k8sClient.Get(testCtx, client.ObjectKey{Name: ns}, createdNs)
				return err == nil && createdNs.Status.Phase == corev1.NamespaceActive
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue(), "Namespace should become active")
		}

		// Initialize engine
		engine = NewInMemoryEngineWithNamespaces(k8sClient, scheme, targetNamespaces)
	})

	AfterEach(func() {
		// Cancel the test context
		testCancel()

		// Collect all cluster-scoped resources to delete
		clusterScopedResources := []client.Object{}

		// Collect ClusterRoles
		clusterRoles := &rbacv1.ClusterRoleList{}
		if err := k8sClient.List(context.Background(), clusterRoles,
			client.MatchingLabels{ManagedByLabel: ManagedByValue}); err == nil {
			for i := range clusterRoles.Items {
				clusterScopedResources = append(clusterScopedResources, &clusterRoles.Items[i])
			}
		}

		// Collect ClusterRoleBindings
		clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
		if err := k8sClient.List(context.Background(), clusterRoleBindings); err == nil {
			for i := range clusterRoleBindings.Items {
				// Only delete our managed ClusterRoleBindings
				if clusterRoleBindings.Items[i].Name != "" &&
					len(clusterRoleBindings.Items[i].Name) > 11 &&
					clusterRoleBindings.Items[i].Name[:11] == "octopus-crb" {
					clusterScopedResources = append(clusterScopedResources, &clusterRoleBindings.Items[i])
				}
			}
		}

		// Collect CWSAs
		cwsaList := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccountList{}
		if err := k8sClient.List(context.Background(), cwsaList); err == nil {
			for i := range cwsaList.Items {
				clusterScopedResources = append(clusterScopedResources, &cwsaList.Items[i])
			}
		}

		// Add any additional cluster-scoped resources tracked in cleanResources
		clusterScopedResources = append(clusterScopedResources, cleanResources...)

		// Delete all cluster-scoped resources first
		if len(clusterScopedResources) > 0 {
			helpers.DeleteAll(cfg, k8sClient, clusterScopedResources...)
		}

		// Clean up all child resources created in the test namespaces
		namespaces := make([]client.Object, 0, len(cleanNamespaces))
		for _, ns := range cleanNamespaces {
			namespaces = append(namespaces, ns)
		}
		helpers.DeleteAll(cfg, k8sClient, namespaces...)
	})

	Context("Cascading Permissions", Serial, func() {
		It("should cascade permissions from space-level to project-level to step-level", func() {
			By("creating WSA with space-level scope")
			spacePermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"get"},
				},
			}

			wsaSpace := helpers.CreateTestWSA("wsa-space-cascade", testNamespace, map[string][]string{
				"spaces": {"cascade-space"},
			}, spacePermissions)
			Expect(k8sClient.Create(testCtx, wsaSpace)).To(Succeed())

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("creating WSA with project-level scope in same space")
			projectPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"get"},
				},
			}

			wsaProject := helpers.CreateTestWSA("wsa-project-cascade", testNamespace, map[string][]string{
				"spaces":   {"cascade-space"},
				"projects": {"cascade-project"},
			}, projectPermissions)
			Expect(k8sClient.Create(testCtx, wsaProject)).To(Succeed())

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("creating WSA with step-level scope in same space and project")
			stepPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
			}

			wsaStep := helpers.CreateTestWSA("wsa-step-cascade", testNamespace, map[string][]string{
				"spaces":   {"cascade-space"},
				"projects": {"cascade-project"},
				"steps":    {"deploy"},
			}, stepPermissions)
			Expect(k8sClient.Create(testCtx, wsaStep)).To(Succeed())

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying step-level service account exists")
			var stepSA *corev1.ServiceAccount
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for i, sa := range serviceAccounts.Items {
					if sa.Annotations[StepKey] == "deploy" &&
						sa.Annotations[ProjectKey] == "cascade-project" &&
						sa.Annotations[SpaceKey] == "cascade-space" {
						stepSA = &serviceAccounts.Items[i]
						break
					}
				}
			}
			Expect(stepSA).NotTo(BeNil(), "Step-level service account should exist")

			By("verifying step SA has role bindings from all three WSAs")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(len(wsaRoleBindings)).To(BeNumerically(">=", 3), "Should have role bindings for all three WSAs")

			// Count how many role bindings include the step SA
			rbCount := 0
			for _, rb := range wsaRoleBindings {
				for _, subject := range rb.Subjects {
					if subject.Name == stepSA.Name && subject.Namespace == stepSA.Namespace {
						rbCount++
						break
					}
				}
			}
			Expect(rbCount).To(Equal(3), "Step SA should be in role bindings for all three WSAs")

			By("updating space-level permissions and verifying step SA gets updated permissions")
			updatedSpacePermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps", "services"},
					Verbs:     []string{"get", "list"},
				},
			}

			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsaSpace.Name,
					Namespace: wsaSpace.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}

				updatedWSA.Spec.Permissions.Permissions = updatedSpacePermissions
				return k8sClient.Update(testCtx, updatedWSA)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("running reconciliation")
			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsaSpace.Name,
					Namespace: wsaSpace.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}
				return engine.ReconcileResource(testCtx, NewWSAResource(updatedWSA))
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("verifying role was updated")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			foundUpdatedRole := false
			for _, role := range roles.Items {
				if len(role.Rules) > 0 && len(role.Rules[0].Resources) == 2 {
					// Check if this is the updated space role
					foundUpdatedRole = true
					break
				}
			}
			Expect(foundUpdatedRole).To(BeTrue(), "Space role should be updated with new permissions")
		})

		It("should handle complex cascading with multiple overlapping scopes", func() {
			By("creating WSAs with various scope hierarchies")

			// Space-level WSA
			wsaSpace := helpers.CreateTestWSA("wsa-complex-space", testNamespace, map[string][]string{
				"spaces": {"multi-space"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsaSpace)).To(Succeed())

			// Space + Environment WSA
			wsaSpaceEnv := helpers.CreateTestWSA("wsa-complex-space-env", testNamespace, map[string][]string{
				"spaces":       {"multi-space"},
				"environments": {"production"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsaSpaceEnv)).To(Succeed())

			// Space + Project WSA
			wsaSpaceProject := helpers.CreateTestWSA("wsa-complex-space-proj", testNamespace, map[string][]string{
				"spaces":   {"multi-space"},
				"projects": {"web-app"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsaSpaceProject)).To(Succeed())

			// Space + Project + Environment WSA (most specific)
			wsaSpecific := helpers.CreateTestWSA("wsa-complex-specific", testNamespace, map[string][]string{
				"spaces":       {"multi-space"},
				"projects":     {"web-app"},
				"environments": {"production"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsaSpecific)).To(Succeed())

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("finding the most specific service account")
			var specificSA *corev1.ServiceAccount
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for i, sa := range serviceAccounts.Items {
					if sa.Annotations[SpaceKey] == "multi-space" &&
						sa.Annotations[ProjectKey] == "web-app" &&
						sa.Annotations[EnvironmentKey] == "production" {
						specificSA = &serviceAccounts.Items[i]
						break
					}
				}
				if specificSA != nil {
					break
				}
			}
			Expect(specificSA).NotTo(BeNil(), "Most specific service account should exist")

			By("verifying specific SA has role bindings from all applicable WSAs")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")

			// Count how many role bindings include the specific SA
			rbCount := 0
			for _, rb := range wsaRoleBindings {
				for _, subject := range rb.Subjects {
					if subject.Name == specificSA.Name && subject.Namespace == specificSA.Namespace {
						rbCount++
						break
					}
				}
			}
			// The specific SA should be in bindings for: space, space+env, space+project, and specific
			Expect(rbCount).To(Equal(4), "Specific SA should be in 4 role bindings")
		})
	})

	Context("Cross-Resource Interactions (WSA + CWSA)", Serial, func() {
		It("should apply both WSA and CWSA permissions to the same service accounts", func() {
			By("creating WSA with project scope")
			wsaPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
			}

			wsa := helpers.CreateTestWSA("cross-wsa", testNamespace, map[string][]string{
				"projects": {"cross-project"},
			}, wsaPermissions)
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("creating CWSA with same project scope")
			cwsaPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get", "list", "watch"},
				},
			}

			cwsa := helpers.CreateTestCWSA("cross-cwsa", map[string][]string{
				"projects": {"cross-project"},
			}, cwsaPermissions)
			Expect(k8sClient.Create(testCtx, cwsa)).To(Succeed())
			cleanResources = append(cleanResources, cwsa)

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying service accounts exist in target namespaces")
			var sharedSA *corev1.ServiceAccount
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty())
				if sharedSA == nil {
					sharedSA = &serviceAccounts.Items[0]
				}
			}

			By("verifying RoleBinding exists for WSA")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(wsaRoleBindings).NotTo(BeEmpty())

			foundWSARB := false
			for _, rb := range wsaRoleBindings {
				for _, subject := range rb.Subjects {
					if subject.Name == sharedSA.Name {
						foundWSARB = true
						break
					}
				}
			}
			Expect(foundWSARB).To(BeTrue(), "WSA RoleBinding should reference the service account")

			By("verifying ClusterRoleBinding exists for CWSA")
			clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
			Expect(k8sClient.List(testCtx, clusterRoleBindings)).To(Succeed())

			cwsaCRBs := helpers.FilterClusterRoleBindingsByPrefix(clusterRoleBindings.Items, "octopus-crb-")
			Expect(cwsaCRBs).NotTo(BeEmpty())

			foundCWSACRB := false
			for _, crb := range cwsaCRBs {
				for _, subject := range crb.Subjects {
					if subject.Name == sharedSA.Name {
						foundCWSACRB = true
						break
					}
				}
			}
			Expect(foundCWSACRB).To(BeTrue(), "CWSA ClusterRoleBinding should reference the service account")

			By("updating CWSA permissions and verifying they apply to service accounts")
			updatedCWSAPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments", "daemonsets"},
					Verbs:     []string{"get", "list", "watch", "create"},
				},
			}

			Eventually(func() error {
				updatedCWSA := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{Name: cwsa.Name}, updatedCWSA)
				if err != nil {
					return err
				}

				updatedCWSA.Spec.Permissions.Permissions = updatedCWSAPermissions
				return k8sClient.Update(testCtx, updatedCWSA)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("running reconciliation")
			Eventually(func() error {
				updatedCWSA := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{Name: cwsa.Name}, updatedCWSA)
				if err != nil {
					return err
				}
				return engine.ReconcileResource(testCtx, NewClusterWSAResource(updatedCWSA))
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("verifying ClusterRole was updated")
			clusterRoles := &rbacv1.ClusterRoleList{}
			Expect(k8sClient.List(testCtx, clusterRoles,
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			foundUpdatedCR := false
			for _, cr := range clusterRoles.Items {
				if len(cr.Rules) > 0 && len(cr.Rules[0].Resources) == 2 {
					foundUpdatedCR = true
					Expect(cr.Rules[0].Resources).To(ContainElements("deployments", "daemonsets"))
					break
				}
			}
			Expect(foundUpdatedCR).To(BeTrue(), "ClusterRole should be updated")
		})

		It("should handle WSA and CWSA with cascading scopes correctly", func() {
			By("creating WSA with space scope")
			wsa := helpers.CreateTestWSA("cascade-cross-wsa", testNamespace, map[string][]string{
				"spaces": {"cross-space"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("creating CWSA with space + project scope")
			cwsa := helpers.CreateTestCWSA("cascade-cross-cwsa", map[string][]string{
				"spaces":   {"cross-space"},
				"projects": {"cross-project"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, cwsa)).To(Succeed())
			cleanResources = append(cleanResources, cwsa)

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying service accounts at both scope levels")
			var spaceSA, projectSA *corev1.ServiceAccount

			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for i, sa := range serviceAccounts.Items {
					if sa.Annotations[SpaceKey] == "cross-space" {
						switch sa.Annotations[ProjectKey] {
						case "":
							spaceSA = &serviceAccounts.Items[i]
						case "cross-project":
							projectSA = &serviceAccounts.Items[i]
						}
					}
				}
			}

			Expect(spaceSA).NotTo(BeNil(), "Space-level SA should exist")
			Expect(projectSA).NotTo(BeNil(), "Project-level SA should exist")

			By("verifying project SA has both RoleBinding and ClusterRoleBinding")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			foundProjectInRB := false
			for _, rb := range wsaRoleBindings {
				for _, subject := range rb.Subjects {
					if subject.Name == projectSA.Name {
						foundProjectInRB = true
						break
					}
				}
			}
			Expect(foundProjectInRB).To(BeTrue(), "Project SA should have RoleBinding from WSA")

			clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
			Expect(k8sClient.List(testCtx, clusterRoleBindings)).To(Succeed())

			cwsaCRBs := helpers.FilterClusterRoleBindingsByPrefix(clusterRoleBindings.Items, "octopus-crb-")
			foundProjectInCRB := false
			for _, crb := range cwsaCRBs {
				for _, subject := range crb.Subjects {
					if subject.Name == projectSA.Name {
						foundProjectInCRB = true
						break
					}
				}
			}
			Expect(foundProjectInCRB).To(BeTrue(), "Project SA should have ClusterRoleBinding from CWSA")
		})
	})

	Context("Multiple Rapid Sequential Updates", Serial, func() {
		It("should handle rapid scope updates without losing data", func() {
			By("creating WSA with initial scope")
			wsa := helpers.CreateTestWSA("rapid-update-wsa", testNamespace, map[string][]string{
				"projects": {"project-1"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running initial reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("performing rapid sequential scope updates")
			for i := 2; i <= 5; i++ {
				Eventually(func() error {
					updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
					err := k8sClient.Get(testCtx, client.ObjectKey{
						Name:      wsa.Name,
						Namespace: wsa.Namespace,
					}, updatedWSA)
					if err != nil {
						return err
					}

					updatedWSA.Spec.Scope.Projects = []string{fmt.Sprintf("project-%d", i)}
					return k8sClient.Update(testCtx, updatedWSA)
				}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

				// Run reconciliation for each update
				Eventually(func() error {
					updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
					err := k8sClient.Get(testCtx, client.ObjectKey{
						Name:      wsa.Name,
						Namespace: wsa.Namespace,
					}, updatedWSA)
					if err != nil {
						return err
					}
					return engine.ReconcileResource(testCtx, NewWSAResource(updatedWSA))
				}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

				// Force cleanup of service accounts marked for deletion
				for _, ns := range targetNamespaces {
					saList := &corev1.ServiceAccountList{}
					if err := k8sClient.List(testCtx, saList,
						client.InNamespace(ns),
						client.MatchingLabels{ManagedByLabel: ManagedByValue}); err == nil {
						for j := range saList.Items {
							sa := &saList.Items[j]
							// If marked for deletion, remove finalizer to complete deletion
							if !sa.DeletionTimestamp.IsZero() && len(sa.Finalizers) > 0 {
								sa.Finalizers = []string{}
								_ = k8sClient.Update(testCtx, sa)
							}
						}
					}
				}
			}

			By("verifying final state matches last update")
			finalWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
			Expect(k8sClient.Get(testCtx, client.ObjectKey{
				Name:      wsa.Name,
				Namespace: wsa.Namespace,
			}, finalWSA)).To(Succeed())

			Expect(finalWSA.Spec.Scope.Projects).To(Equal([]string{"project-5"}))

			By("verifying service accounts reflect final scope")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty())
				for _, sa := range serviceAccounts.Items {
					Expect(sa.Annotations[ProjectKey]).To(Equal("project-5"))
				}
			}

			By("verifying no orphaned resources remain")
			// Run full reconciliation to clean up
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			// Check that only service accounts for project-5 exist
			totalSACount := 0
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())
				totalSACount += len(serviceAccounts.Items)
			}
			Expect(totalSACount).To(Equal(len(targetNamespaces)), "Should have exactly one SA per namespace")
		})

		It("should handle rapid permission updates without data loss", func() {
			By("creating WSA with initial permissions")
			wsa := helpers.CreateTestWSA("rapid-perm-wsa", testNamespace, map[string][]string{
				"projects": {"stable-project"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running initial reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("performing rapid sequential permission updates")
			permissionSets := [][]rbacv1.PolicyRule{
				{
					{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"get", "list"}},
				},
				{
					{
						APIGroups: []string{""}, Resources: []string{"pods", "services", "configmaps"},
						Verbs: []string{"get", "list", "watch"},
					},
				},
				{
					{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
				},
				{
					{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list"}},
					{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
				},
			}

			for _, permissions := range permissionSets {
				Eventually(func() error {
					updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
					err := k8sClient.Get(testCtx, client.ObjectKey{
						Name:      wsa.Name,
						Namespace: wsa.Namespace,
					}, updatedWSA)
					if err != nil {
						return err
					}

					updatedWSA.Spec.Permissions.Permissions = permissions
					return k8sClient.Update(testCtx, updatedWSA)
				}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

				// Run reconciliation
				Eventually(func() error {
					updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
					err := k8sClient.Get(testCtx, client.ObjectKey{
						Name:      wsa.Name,
						Namespace: wsa.Namespace,
					}, updatedWSA)
					if err != nil {
						return err
					}
					return engine.ReconcileResource(testCtx, NewWSAResource(updatedWSA))
				}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
			}

			By("verifying final permissions match last update")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			Expect(roles.Items).NotTo(BeEmpty())
			foundFinalPermissions := false
			for _, role := range roles.Items {
				if len(role.Rules) == 2 {
					// This should be the final permission set
					foundFinalPermissions = true
					Expect(role.Rules[0].Resources).To(ContainElement("deployments"))
					Expect(role.Rules[1].Resources).To(ContainElement("pods"))
					break
				}
			}
			Expect(foundFinalPermissions).To(BeTrue(), "Final permissions should be applied")

			By("verifying service accounts were not recreated")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				// Service accounts should still exist with same names
				Expect(serviceAccounts.Items).To(HaveLen(1), "Should have exactly one SA (not recreated)")
			}
		})

		It("should handle concurrent WSA and CWSA updates", func() {
			By("creating WSA and CWSA with same scope")
			wsa := helpers.CreateTestWSA("concurrent-wsa", testNamespace, map[string][]string{
				"projects": {"concurrent-project"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			cwsa := helpers.CreateTestCWSA("concurrent-cwsa", map[string][]string{
				"projects": {"concurrent-project"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, cwsa)).To(Succeed())
			cleanResources = append(cleanResources, cwsa)

			By("running initial reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("updating both WSA and CWSA in quick succession")
			// Update WSA
			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsa.Name,
					Namespace: wsa.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}

				updatedWSA.Spec.Scope.Environments = []string{"production"}
				return k8sClient.Update(testCtx, updatedWSA)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			// Update CWSA immediately after
			Eventually(func() error {
				updatedCWSA := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{Name: cwsa.Name}, updatedCWSA)
				if err != nil {
					return err
				}

				updatedCWSA.Spec.Scope.Environments = []string{"production"}
				return k8sClient.Update(testCtx, updatedCWSA)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("running full reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying both updates applied correctly")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				// Should have SAs for the combined scope
				foundCorrectSA := false
				for _, sa := range serviceAccounts.Items {
					if sa.Annotations[ProjectKey] == "concurrent-project" &&
						sa.Annotations[EnvironmentKey] == "production" {
						foundCorrectSA = true
						break
					}
				}
				Expect(foundCorrectSA).To(BeTrue(), "Should have SA with both project and environment")
			}

			By("verifying both RoleBinding and ClusterRoleBinding exist")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(wsaRoleBindings).NotTo(BeEmpty(), "WSA RoleBindings should exist")

			clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
			Expect(k8sClient.List(testCtx, clusterRoleBindings)).To(Succeed())

			cwsaCRBs := helpers.FilterClusterRoleBindingsByPrefix(clusterRoleBindings.Items, "octopus-crb-")
			Expect(cwsaCRBs).NotTo(BeEmpty(), "CWSA ClusterRoleBindings should exist")
		})
	})
})
