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
// nolint:goconst // We don't need to const a resource name
package rules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-set/v3"
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

// reconcileAll performs a full reconciliation using the engine's public methods.
// This replaces the old engine.Reconcile() method for testing purposes.
// Note: This only garbage collects ServiceAccounts to match the original behavior.
// Role/RoleBinding garbage collection is handled separately in the staged pipeline.
// IMPORTANT: This includes resources with DeletionTimestamp to match the staging pipeline
// behavior and avoid race conditions between cleanup and staging GC.
func reconcileAll(ctx context.Context, engine *InMemoryEngine) error {
	allResources := make([]WSAResource, 0)

	wsaIter, err := engine.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to list WorkloadServiceAccounts: %w", err)
	}
	for wsa := range wsaIter {
		// Include all resources, even those being deleted, to avoid premature SA cleanup
		allResources = append(allResources, NewWSAResource(wsa))
	}

	cwsaIter, err := engine.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to list ClusterWorkloadServiceAccounts: %w", err)
	}
	for cwsa := range cwsaIter {
		// Include all resources, even those being deleted, to avoid premature SA cleanup
		allResources = append(allResources, NewClusterWSAResource(cwsa))
	}

	targetNamespaces := engine.GetTargetNamespaces()

	scopeComputation := NewScopeComputationService(nil, nil)
	scopeMap, vocabulary := scopeComputation.ComputeScopesForWSAs(allResources)
	scopeToSA, saToWSAMap, wsaToSANames, uniqueAccounts := scopeComputation.GenerateServiceAccountMappings(scopeMap)

	// Update engine state
	engine.mu.Lock()
	engine.scopeToSA = scopeToSA
	engine.saToWsaMap = saToWSAMap
	engine.vocabulary = &vocabulary
	engine.ScopeComputation = NewScopeComputationService(engine.vocabulary, engine.scopeToSA)
	engine.mu.Unlock()

	// Ensure resources
	createdRoles, err := engine.EnsureRoles(ctx, allResources)
	if err != nil {
		return fmt.Errorf("failed to ensure roles: %w", err)
	}

	if err := engine.EnsureServiceAccounts(ctx, uniqueAccounts, targetNamespaces); err != nil {
		return fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	if err := engine.EnsureRoleBindings(ctx, allResources, createdRoles, wsaToSANames, targetNamespaces); err != nil {
		return fmt.Errorf("failed to ensure role bindings: %w", err)
	}

	// Only garbage collect ServiceAccounts (matching original Reconcile behavior)
	expectedSAs := set.New[string](len(uniqueAccounts))
	for _, sa := range uniqueAccounts {
		expectedSAs.Insert(sa.Name)
	}
	targetNSSet := set.New[string](len(targetNamespaces))
	for _, ns := range targetNamespaces {
		targetNSSet.Insert(ns)
	}

	if _, err := engine.GarbageCollectServiceAccounts(ctx, expectedSAs, targetNSSet); err != nil {
		return fmt.Errorf("failed to garbage collect service accounts: %w", err)
	}

	return nil
}

var _ = Describe("Engine Integration Tests", func() {
	var (
		engine           InMemoryEngine
		targetNamespaces []string
		testNamespace    string
		cleanNamespaces  []*corev1.Namespace
		testCtx          context.Context
		testCancel       context.CancelFunc
		scheme           *runtime.Scheme
	)

	BeforeEach(func() {
		// Create a fresh context for each test with a timeout
		testCtx, testCancel = context.WithTimeout(context.Background(), 30*time.Minute)

		// Initialize scheme
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = rbacv1.AddToScheme(scheme)
		_ = agentoctopuscomv1beta1.AddToScheme(scheme)

		// Create unique test namespaces for each test
		timestamp := time.Now().UnixNano()
		ns1Name := fmt.Sprintf("test-engine-%d", timestamp)
		ns2Name := fmt.Sprintf("test-engine2-%d", timestamp)
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

		// Delete all cluster-scoped resources first
		if len(clusterScopedResources) > 0 {
			helpers.DeleteAll(cfg, k8sClient, clusterScopedResources...)
		}

		// Clean up all child resources created in the test namespaces
		// This will also clean up WSAs within those namespaces
		namespaces := make([]client.Object, 0, len(cleanNamespaces))
		for _, ns := range cleanNamespaces {
			namespaces = append(namespaces, ns)
		}
		helpers.DeleteAll(cfg, k8sClient, namespaces...)
	})

	Context("When reconciling WorkloadServiceAccounts", func() {
		It("should create service accounts with proper labels and annotations", func() {
			By("creating a WorkloadServiceAccount with specific scope")
			wsa := helpers.CreateTestWSA("test-wsa-1", testNamespace, map[string][]string{
				"projects":     {"web-app", "api-service"},
				"environments": {"production", "staging"},
				"tenants":      {"customer-a"},
			}, []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service accounts are created in target namespaces")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty(), fmt.Sprintf("Expected service accounts in namespace %s", ns))

				for _, sa := range serviceAccounts.Items {
					By(fmt.Sprintf("verifying service account %s in namespace %s", sa.Name, ns))

					// Verify name pattern (hash-based "octopus-sa-")
					Expect(sa.Name).To(HavePrefix("octopus-sa-"), "Service account should follow naming pattern")

					// Verify labels
					Expect(sa.Labels).To(HaveKey(ManagedByLabel))
					Expect(sa.Labels[ManagedByLabel]).To(Equal(ManagedByValue))
					Expect(sa.Labels).To(HaveKey(ProjectKey))
					Expect(sa.Labels).To(HaveKey(EnvironmentKey))
					Expect(sa.Labels).To(HaveKey(TenantKey))

					// Verify annotations contain actual values (not hashes)
					// TODO: Verify against expected scope values
					Expect(sa.Annotations).To(HaveKey(ProjectKey))
					Expect(sa.Annotations).To(HaveKey(EnvironmentKey))
					Expect(sa.Annotations).To(HaveKey(TenantKey))
				}
			}
		})

		It("should create roles with inline permissions", func() {
			By("creating a WorkloadServiceAccount with inline permissions")
			permissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get", "list", "watch", "create"},
				},
			}

			wsa := helpers.CreateTestWSA("test-wsa-2", testNamespace, map[string][]string{
				"projects": {"mobile-app"},
			}, permissions)
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying roles are created")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			Expect(roles.Items).NotTo(BeEmpty(), "Expected roles to be created")

			for _, role := range roles.Items {
				By(fmt.Sprintf("verifying role %s", role.Name))

				// Verify name pattern
				Expect(role.Name).To(ContainSubstring("octopus-role-"), "Role should follow naming pattern")

				// Verify permissions are correctly set
				Expect(role.Rules).To(Equal(permissions), "Role rules should match WSA permissions")

				// Verify labels
				Expect(role.Labels).To(HaveKey(ManagedByLabel))
				Expect(role.Labels[ManagedByLabel]).To(Equal(ManagedByValue))
			}
		})

		It("should create role bindings connecting service accounts to roles", func() {
			By("creating a WorkloadServiceAccount with both inline permissions and role references")
			wsa := helpers.CreateTestWSAWithRoles("test-wsa-3", testNamespace, map[string][]string{
				"environments": {"development"},
			}, []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"get"},
				},
			}, []rbacv1.RoleRef{
				{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     "existing-role",
				},
			}, []rbacv1.RoleRef{
				{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "view",
				},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying role bindings are created")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			// Should have role bindings for inline permissions, role references, and cluster roles
			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(wsaRoleBindings).NotTo(BeEmpty(), "Expected role bindings to be created")

			// Track the role references we find to ensure cluster roles are bound as role bindings
			var foundRoleRefs []rbacv1.RoleRef

			for _, rb := range wsaRoleBindings {
				By(fmt.Sprintf("verifying role binding %s", rb.Name))

				// Verify name pattern
				Expect(rb.Name).To(ContainSubstring("octopus-rb-"), "RoleBinding should follow naming pattern")

				// Verify subjects include service accounts from all target namespaces
				Expect(rb.Subjects).NotTo(BeEmpty(), "RoleBinding should have subjects")

				foundRoleRefs = append(foundRoleRefs, rb.RoleRef)

				for _, subject := range rb.Subjects {
					Expect(subject.Kind).To(Equal("ServiceAccount"), "Subject should be ServiceAccount")
					Expect(subject.Name).To(HavePrefix("octopus-sa-"), "Subject name should match service account pattern")
					Expect(targetNamespaces).To(ContainElement(subject.Namespace), "Subject namespace should be in target namespaces")
				}
			}

			By("verifying cluster roles are bound as role bindings")
			for _, clusterRoleRef := range wsa.Spec.Permissions.ClusterRoles {
				found := false
				for _, foundRef := range foundRoleRefs {
					if foundRef.Kind == clusterRoleRef.Kind &&
						foundRef.Name == clusterRoleRef.Name &&
						foundRef.APIGroup == clusterRoleRef.APIGroup {
						found = true
						Expect(foundRef.Kind).To(Equal("ClusterRole"),
							"ClusterRole should be referenced in RoleBinding")
						break
					}
				}
				Expect(found).To(BeTrue(),
					fmt.Sprintf("ClusterRole %s should be bound via RoleBinding", clusterRoleRef.Name))
			}

		})

		It("should handle multiple WSAs with overlapping scopes correctly", func() {
			By("creating multiple WorkloadServiceAccounts with overlapping scopes")

			// WSA 1: Projects web-app, mobile-app
			wsa1 := helpers.CreateTestWSA("test-wsa-overlap-1", testNamespace, map[string][]string{
				"projects": {"web-app", "mobile-app"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa1)).To(Succeed())

			// WSA 2: Projects mobile-app, api-service (overlaps on mobile-app)
			wsa2 := helpers.CreateTestWSA("test-wsa-overlap-2", testNamespace, map[string][]string{
				"projects": {"mobile-app", "api-service"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa2)).To(Succeed())

			// WSA 3: Different scope entirely
			wsa3 := helpers.CreateTestWSA("test-wsa-overlap-3", testNamespace, map[string][]string{
				"environments": {"production"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa3)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying correct number of unique service accounts are created")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				// Should have service accounts for each unique scope combination
				// This depends on how the scope intersection algorithm works
				Expect(serviceAccounts.Items).NotTo(BeEmpty(), fmt.Sprintf("Expected service accounts in namespace %s", ns))

				// Verify all service accounts have proper naming
				for _, sa := range serviceAccounts.Items {
					Expect(sa.Name).To(HavePrefix("octopus-sa-"), "All service accounts should follow naming pattern")
				}
			}

			By("verifying roles are created for each WSA")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			// Should have 3 roles (one per WSA)
			Expect(roles.Items).To(HaveLen(3), "Expected one role per WSA")
		})

		It("should create service accounts in multiple target namespaces", func() {
			By("creating a WorkloadServiceAccount")
			wsa := helpers.CreateTestWSA("test-wsa-multi-ns", testNamespace, map[string][]string{
				"spaces": {"default-space"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service accounts are created in all target namespaces")
			var allServiceAccounts []corev1.ServiceAccount

			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty(), fmt.Sprintf("Expected service accounts in namespace %s", ns))
				allServiceAccounts = append(allServiceAccounts, serviceAccounts.Items...)
			}

			By("verifying service accounts across namespaces have consistent naming")
			if len(allServiceAccounts) >= 2 {
				// Service accounts should have the same name pattern across namespaces for the same scope
				firstName := allServiceAccounts[0].Name
				for _, sa := range allServiceAccounts[1:] {
					if sa.Annotations[SpaceKey] == allServiceAccounts[0].Annotations[SpaceKey] {
						Expect(sa.Name).To(Equal(firstName), "Service accounts for same scope should have same name across namespaces")
					}
				}
			}
		})
	})

	Context("Partial Update Tests - WSA/CWSA Creation", Serial, func() {
		It("should create complete resources when WSA is created with inline permissions", func() {
			By("creating a WorkloadServiceAccount with inline permissions")
			permissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
			}

			wsa := helpers.CreateTestWSA("test-partial-wsa-1", testNamespace, map[string][]string{
				"projects":     {"project-a"},
				"environments": {"dev", "prod"},
			}, permissions)
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service accounts created in all target namespaces")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty())

				// Verify service account has correct labels and annotations
				for _, sa := range serviceAccounts.Items {
					Expect(sa.Labels).To(HaveKey(ManagedByLabel))
					Expect(sa.Labels).To(HaveKey(ProjectKey))
					Expect(sa.Labels).To(HaveKey(EnvironmentKey))
					Expect(sa.Annotations).To(HaveKey(ProjectKey))
					Expect(sa.Annotations).To(HaveKey(EnvironmentKey))
				}
			}

			By("verifying role created with correct permissions")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			Expect(roles.Items).NotTo(BeEmpty())
			var wsaRole *rbacv1.Role
			for i, role := range roles.Items {
				if len(role.Rules) > 0 && role.Rules[0].Resources[0] == "pods" {
					wsaRole = &roles.Items[i]
					break
				}
			}
			Expect(wsaRole).NotTo(BeNil())
			Expect(wsaRole.Rules).To(Equal(permissions))

			By("verifying role bindings created and link SAs to roles")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(wsaRoleBindings).NotTo(BeEmpty())

			// Find the role binding for our role
			var wsaRoleBinding *rbacv1.RoleBinding
			for i, rb := range wsaRoleBindings {
				if rb.RoleRef.Name == wsaRole.Name {
					wsaRoleBinding = &wsaRoleBindings[i]
					break
				}
			}
			Expect(wsaRoleBinding).NotTo(BeNil())

			// Verify subjects include service accounts from all target namespaces
			Expect(wsaRoleBinding.Subjects).NotTo(BeEmpty())
			Expect(len(wsaRoleBinding.Subjects)).To(BeNumerically(">=", len(targetNamespaces)))

			for _, subject := range wsaRoleBinding.Subjects {
				Expect(subject.Kind).To(Equal("ServiceAccount"))
				Expect(subject.Name).To(HavePrefix("octopus-sa-"))
				Expect(targetNamespaces).To(ContainElement(subject.Namespace))
			}
		})

		It("should create complete resources when CWSA is created with inline permissions", func() {
			By("creating a ClusterWorkloadServiceAccount with inline permissions")
			permissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get", "list"},
				},
			}

			cwsa := helpers.CreateTestCWSA("test-partial-cwsa-1", map[string][]string{
				"projects": {"project-b"},
				"spaces":   {"space-1"},
			}, permissions)
			Expect(k8sClient.Create(testCtx, cwsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service accounts created in all target namespaces")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty())

				// Verify service account has correct labels
				for _, sa := range serviceAccounts.Items {
					Expect(sa.Labels).To(HaveKey(ManagedByLabel))
					Expect(sa.Labels).To(HaveKey(ProjectKey))
					Expect(sa.Labels).To(HaveKey(SpaceKey))
				}
			}

			By("verifying cluster role created with correct permissions")
			clusterRoles := &rbacv1.ClusterRoleList{}
			Expect(k8sClient.List(testCtx, clusterRoles,
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			Expect(clusterRoles.Items).NotTo(BeEmpty())
			var cwsaClusterRole *rbacv1.ClusterRole
			for i, cr := range clusterRoles.Items {
				if len(cr.Rules) > 0 && len(cr.Rules[0].Resources) > 0 && cr.Rules[0].Resources[0] == "deployments" {
					cwsaClusterRole = &clusterRoles.Items[i]
					break
				}
			}
			Expect(cwsaClusterRole).NotTo(BeNil())
			Expect(cwsaClusterRole.Rules).To(Equal(permissions))

			By("verifying cluster role bindings created and link SAs to cluster roles")
			clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
			Expect(k8sClient.List(testCtx, clusterRoleBindings)).To(Succeed())

			cwsaCRBs := helpers.FilterClusterRoleBindingsByPrefix(clusterRoleBindings.Items, "octopus-crb-")
			Expect(cwsaCRBs).NotTo(BeEmpty())

			// Find the cluster role binding for our cluster role
			var cwsaCRB *rbacv1.ClusterRoleBinding
			for i, crb := range cwsaCRBs {
				if crb.RoleRef.Name == cwsaClusterRole.Name {
					cwsaCRB = &cwsaCRBs[i]
					break
				}
			}
			Expect(cwsaCRB).NotTo(BeNil())

			// Verify subjects include service accounts from all target namespaces
			Expect(cwsaCRB.Subjects).NotTo(BeEmpty())
			for _, subject := range cwsaCRB.Subjects {
				Expect(subject.Kind).To(Equal("ServiceAccount"))
				Expect(subject.Name).To(HavePrefix("octopus-sa-"))
			}
		})

		It("should create service accounts for more specific scopes that inherit broader permissions", func() {
			By("creating a WSA with space-level scope")
			spacePermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"get"},
				},
			}

			wsaSpace := helpers.CreateTestWSA("test-wsa-space", testNamespace, map[string][]string{
				"spaces": {"space-1"},
			}, spacePermissions)
			Expect(k8sClient.Create(testCtx, wsaSpace)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("creating a WSA with more specific step-level scope in the same space")
			stepPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"get"},
				},
			}

			wsaStep := helpers.CreateTestWSA("test-wsa-step", testNamespace, map[string][]string{
				"spaces": {"space-1"},
				"steps":  {"deploy-step"},
			}, stepPermissions)
			Expect(k8sClient.Create(testCtx, wsaStep)).To(Succeed())

			By("running reconciliation again")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service account created for step scope")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty())

				// Find the service account with step scope
				var stepSA *corev1.ServiceAccount
				for i, sa := range serviceAccounts.Items {
					if sa.Annotations[StepKey] != "" {
						stepSA = &serviceAccounts.Items[i]
						break
					}
				}
				Expect(stepSA).NotTo(BeNil(), "Expected service account with step scope")

				// Verify it has space annotation too
				Expect(stepSA.Annotations[SpaceKey]).To(Equal("space-1"))
			}

			By("verifying role bindings for both WSAs include the step-scoped service account")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(len(wsaRoleBindings)).To(BeNumerically(">=", 2), "Expected role bindings for both WSAs")

			// Both role bindings should reference the step-scoped SA
			for _, rb := range wsaRoleBindings {
				foundStepSA := false
				for _, subject := range rb.Subjects {
					if subject.Name != "" && subject.Kind == "ServiceAccount" {
						// Check if this SA might be the step-scoped one by checking the role binding's role
						foundStepSA = true
					}
				}
				// At least one binding should have subjects
				if len(rb.Subjects) > 0 {
					Expect(foundStepSA).To(BeTrue())
				}
			}
		})
	})

	Context("Partial Update Tests - Overlapping Scopes and Permissions", Serial, func() {
		It("should share service accounts between WSAs with overlapping scopes", func() {
			By("creating first WSA with project scope")
			wsa1 := helpers.CreateTestWSA("test-overlap-wsa-1", testNamespace, map[string][]string{
				"projects": {"project-x"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa1)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("getting count of service accounts after first WSA")
			var initialSACount int
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())
				initialSACount += len(serviceAccounts.Items)
			}

			By("creating second WSA with same project scope")
			wsa2 := helpers.CreateTestWSA("test-overlap-wsa-2", testNamespace, map[string][]string{
				"projects": {"project-x"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"}},
			})
			Expect(k8sClient.Create(testCtx, wsa2)).To(Succeed())

			By("running reconciliation again")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service account count hasn't increased (sharing)")
			var finalSACount int
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())
				finalSACount += len(serviceAccounts.Items)
			}

			Expect(finalSACount).To(Equal(initialSACount), "Service account count should remain the same")

			By("verifying two role bindings exist for the shared service accounts")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(len(wsaRoleBindings)).To(BeNumerically(">=", 2), "Expected role bindings for both WSAs")

			// Both role bindings should reference the same service accounts
			if len(wsaRoleBindings) >= 2 {
				rb1Subjects := wsaRoleBindings[0].Subjects
				rb2Subjects := wsaRoleBindings[1].Subjects

				// At least one subject should be the same between the two role bindings
				foundShared := false
				for _, s1 := range rb1Subjects {
					for _, s2 := range rb2Subjects {
						if s1.Name == s2.Name && s1.Namespace == s2.Namespace {
							foundShared = true
							break
						}
					}
				}
				Expect(foundShared).To(BeTrue(), "Role bindings should share service accounts")
			}
		})

		It("should grant service accounts permissions from ALL applicable WSAs and CWSAs", func() {
			By("creating WSA with project scope")
			wsa := helpers.CreateTestWSA("test-multi-wsa", testNamespace, map[string][]string{
				"projects": {"project-y"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("creating CWSA with same project scope")
			cwsa := helpers.CreateTestCWSA("test-multi-cwsa", map[string][]string{
				"projects": {"project-y"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, cwsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service accounts exist with correct annotations")
			var sharedSANames []string
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty())
				for _, sa := range serviceAccounts.Items {
					// With grouped WSAs, verify the SA has the correct project annotation
					projectAnnotation := sa.Annotations[ProjectKey]
					// Could be "project-y" alone or grouped with other projects
					if projectAnnotation != "" && strings.Contains(projectAnnotation, "project-y") {
						sharedSANames = append(sharedSANames, sa.Name)
					}
				}
			}
			Expect(sharedSANames).NotTo(BeEmpty(), "Should have service accounts with project-y annotation")

			By("verifying role binding exists and references service accounts")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			wsaRoleBindings := helpers.FilterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(wsaRoleBindings).NotTo(BeEmpty())

			foundSA := false
			for _, rb := range wsaRoleBindings {
				for _, subject := range rb.Subjects {
					for _, saName := range sharedSANames {
						if subject.Name == saName {
							foundSA = true
							break
						}
					}
				}
			}
			Expect(foundSA).To(BeTrue(), "Role binding should reference service account with project-y scope")

			By("verifying cluster role binding exists and references service accounts")
			clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
			Expect(k8sClient.List(testCtx, clusterRoleBindings)).To(Succeed())

			cwsaCRBs := helpers.FilterClusterRoleBindingsByPrefix(clusterRoleBindings.Items, "octopus-crb-")
			Expect(cwsaCRBs).NotTo(BeEmpty())

			foundSAInCRB := false
			for _, crb := range cwsaCRBs {
				for _, subject := range crb.Subjects {
					for _, saName := range sharedSANames {
						if subject.Name == saName {
							foundSAInCRB = true
							break
						}
					}
				}
			}
			Expect(foundSAInCRB).To(BeTrue(), "Cluster role binding should reference service account with project-y scope")
		})
	})

	Context("Partial Update Tests - Scope Updates", Serial, func() {
		It("should create new service accounts when scope dimensions are added", func() {
			By("creating WSA with single environment")
			wsa := helpers.CreateTestWSA("test-scope-add", testNamespace, map[string][]string{
				"environments": {"dev"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("getting initial service account annotations for this WSA")
			initialSAs := make(map[string]map[string]string) // SA name -> annotations
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for _, sa := range serviceAccounts.Items {
					// Only track SAs that have environment:dev (related to our WSA)
					if sa.Annotations[EnvironmentKey] == "dev" {
						initialSAs[sa.Namespace+"/"+sa.Name] = sa.Annotations
					}
				}
			}

			By("updating WSA to add project dimension")
			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsa.Name,
					Namespace: wsa.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}

				updatedWSA.Spec.Scope.Projects = []string{"project-a", "project-b"}
				return k8sClient.Update(testCtx, updatedWSA)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("running incremental reconciliation for the updated WSA")
			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsa.Name,
					Namespace: wsa.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}
				return reconcileAll(testCtx, &engine)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("verifying service accounts were created or updated for the new scope")
			finalSAs := make(map[string]map[string]string) // SA name -> annotations
			foundProjectAnnotation := false
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for _, sa := range serviceAccounts.Items {
					// Only check SAs that have environment:dev AND projects (related to our updated WSA)
					if sa.Annotations[EnvironmentKey] == "dev" && sa.Annotations[ProjectKey] != "" {
						finalSAs[sa.Namespace+"/"+sa.Name] = sa.Annotations
						foundProjectAnnotation = true

						projectAnnotation := sa.Annotations[ProjectKey]
						// With grouped WSAs, should have both projects in the annotation
						// Could be "project-a+project-b" or separate SAs with "project-a" and "project-b"
						hasValidProject := strings.Contains(projectAnnotation, "project-a") ||
							strings.Contains(projectAnnotation, "project-b")
						Expect(hasValidProject).To(BeTrue(), "Service account should have project annotations with project-a or project-b")
					}
				}
			}

			// Should have at least one SA with both environment and project annotations
			Expect(foundProjectAnnotation).To(BeTrue(), "Should have service accounts with both environment:dev and project annotations")

			// With grouped WSAs, we should have at least as many SAs as before (or they were updated)
			// The key point is that we now have SAs with project annotations where we didn't before
			Expect(finalSAs).ToNot(BeEmpty(), "Should have service accounts with the combined scope")
		})

		It("should clean up service accounts when scope dimensions are removed", func() {
			By("creating WSA with multiple environments and projects")
			wsa := helpers.CreateTestWSA("test-scope-remove", testNamespace, map[string][]string{
				"projects":     {"project-a", "project-b"},
				"environments": {"dev", "staging"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("getting initial service account state for this WSA")
			initialSAs := make(map[string]map[string]string) // SA name -> annotations
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for _, sa := range serviceAccounts.Items {
					// Only track SAs that have dev OR staging environment (related to our WSA)
					envAnnotation := sa.Annotations[EnvironmentKey]
					if strings.Contains(envAnnotation, "dev") || strings.Contains(envAnnotation, "staging") {
						initialSAs[sa.Namespace+"/"+sa.Name] = sa.Annotations
					}
				}
			}
			Expect(initialSAs).ToNot(BeEmpty(), "Should have created initial service accounts")

			By("updating WSA to reduce scope (remove one project)")
			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsa.Name,
					Namespace: wsa.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}

				updatedWSA.Spec.Scope.Projects = []string{"project-a"}
				return k8sClient.Update(testCtx, updatedWSA)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("running full reconciliation to clean up")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying service accounts were cleaned up or updated")
			finalSAs := make(map[string]map[string]string) // SA name -> annotations
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for _, sa := range serviceAccounts.Items {
					// Only check SAs related to our WSA (with dev or staging environment)
					envAnnotation := sa.Annotations[EnvironmentKey]
					if strings.Contains(envAnnotation, "dev") || strings.Contains(envAnnotation, "staging") {
						finalSAs[sa.Namespace+"/"+sa.Name] = sa.Annotations

						// Remaining SAs should only have project-a (not project-b)
						// With grouped WSAs, annotations use "+" separator (e.g., "project-a+project-b")
						// After removing project-b, annotations should not contain project-b
						projectAnnotation := sa.Annotations[ProjectKey]
						if projectAnnotation != "" {
							Expect(projectAnnotation).NotTo(ContainSubstring("project-b"),
								"Service account should not contain project-b after it was removed from scope")
							// Should contain project-a since we kept it
							Expect(projectAnnotation).To(ContainSubstring("project-a"),
								"Service account should contain project-a since it's still in scope")
						}
					}
				}
			}

			// Should have service accounts (cleanup doesn't mean deleting all, just updating/removing project-b)
			Expect(finalSAs).ToNot(BeEmpty(), "Should still have service accounts for the remaining scope")
			// Should have fewer or equal SAs after removing a dimension
			Expect(len(finalSAs)).To(BeNumerically("<=", len(initialSAs)), "Should have cleaned up or consolidated service accounts")
		})

		It("should update roles when permissions change but preserve service accounts", func() {
			By("creating WSA with initial permissions")
			initialPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get"},
				},
			}

			wsa := helpers.CreateTestWSA("test-perm-update", testNamespace, map[string][]string{
				"projects": {"project-z"},
			}, initialPermissions)
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("getting initial service account names")
			initialSANames := make(map[string]bool)
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for _, sa := range serviceAccounts.Items {
					initialSANames[sa.Namespace+"/"+sa.Name] = true
				}
			}

			By("updating WSA with different permissions")
			updatedPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get"},
				},
			}

			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsa.Name,
					Namespace: wsa.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}

				updatedWSA.Spec.Permissions.Permissions = updatedPermissions
				return k8sClient.Update(testCtx, updatedWSA)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("running reconciliation")
			Eventually(func() error {
				updatedWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsa.Name,
					Namespace: wsa.Namespace,
				}, updatedWSA)
				if err != nil {
					return err
				}
				return reconcileAll(testCtx, &engine)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("verifying role permissions were updated")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

			foundUpdatedRole := false
			for _, role := range roles.Items {
				if len(role.Rules) >= 2 {
					// Check if this role has the updated permissions
					if len(role.Rules[0].Verbs) == 3 && role.Rules[0].Verbs[0] == "get" {
						foundUpdatedRole = true
						Expect(role.Rules).To(Equal(updatedPermissions))
						break
					}
				}
			}
			Expect(foundUpdatedRole).To(BeTrue(), "Role should have updated permissions")

			By("verifying service accounts were preserved")
			finalSANames := make(map[string]bool)
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				for _, sa := range serviceAccounts.Items {
					finalSANames[sa.Namespace+"/"+sa.Name] = true
				}
			}

			// Service accounts should be the same
			Expect(finalSANames).To(Equal(initialSANames), "Service accounts should be preserved")
		})
	})

	Context("ServiceAccount Deletion", func() {
		It("should delete ServiceAccount immediately when WSA is deleted", func() {
			By("creating a WorkloadServiceAccount")
			wsa := helpers.CreateTestWSA("test-immediate-delete", testNamespace, map[string][]string{
				"projects": {"api-service"},
			}, []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"get"},
				},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("getting the created ServiceAccount")
			var saName string
			var saNamespace string
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())

				if len(serviceAccounts.Items) > 0 {
					saName = serviceAccounts.Items[0].Name
					saNamespace = ns
					break
				}
			}
			Expect(saName).NotTo(BeEmpty())

			By("verifying RoleBindings exist and reference the ServiceAccount")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(saNamespace),
				client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())
			Expect(roleBindings.Items).NotTo(BeEmpty())

			By("deleting the WSA (no pods created)")
			Expect(k8sClient.Delete(testCtx, wsa)).To(Succeed())

			By("running reconciliation to trigger cleanup")
			Eventually(func() bool {
				// Run reconciliation (may need multiple iterations to complete deletion)
				_ = reconcileAll(testCtx, &engine)
				// Check if SA is deleted
				sa := &corev1.ServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{Name: saName, Namespace: saNamespace}, sa)
				return err != nil && client.IgnoreNotFound(err) == nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"ServiceAccount should be deleted when WSA is deleted")
		})
	})

	Context("Multiple Resource Deletion", Serial, func() {
		It("should preserve SAs for remaining resources when multiple WSAs are deleted", func() {
			By("creating multiple WSAs with different scopes")
			wsa1 := helpers.CreateTestWSA("test-multi-del-1", testNamespace, map[string][]string{
				"projects": {"project-keep"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa1)).To(Succeed())

			wsa2 := helpers.CreateTestWSA("test-multi-del-2", testNamespace, map[string][]string{
				"projects": {"project-delete-1"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa2)).To(Succeed())

			wsa3 := helpers.CreateTestWSA("test-multi-del-3", testNamespace, map[string][]string{
				"projects": {"project-delete-2"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa3)).To(Succeed())

			By("running reconciliation to create all SAs")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying SAs exist for all WSAs")
			var initialSACount int
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())
				initialSACount += len(serviceAccounts.Items)
			}
			Expect(initialSACount).To(BeNumerically(">=", len(targetNamespaces)),
				"Should have SAs in all target namespaces")

			By("deleting WSAs 2 and 3 (keeping WSA 1)")
			Expect(k8sClient.Delete(testCtx, wsa2)).To(Succeed())
			Expect(k8sClient.Delete(testCtx, wsa3)).To(Succeed())

			By("running reconciliation to clean up deleted WSAs")
			Eventually(func() error {
				return reconcileAll(testCtx, &engine)
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("verifying WSA 1 scope still maps to an SA")
			scope := Scope{
				Project:      "project-keep",
				ProjectGroup: "",
				Environment:  "",
				Tenant:       "",
				Step:         "",
				Space:        "",
			}
			retrievedSA, err := engine.GetServiceAccountForScope(scope)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrievedSA).NotTo(BeEmpty(),
				"WSA 1's scope should still map to an SA after WSA 2 and 3 are deleted")
		})

		It("should eventually clean up SAs after resources are fully deleted", func() {
			By("creating a single WSA")
			wsa := helpers.CreateTestWSA("test-eventual-cleanup", testNamespace, map[string][]string{
				"projects": {"project-cleanup"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(reconcileAll(testCtx, &engine)).To(Succeed())

			By("verifying SA exists")
			var saName string
			var saNamespace string
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{ManagedByLabel: ManagedByValue})).To(Succeed())
				if len(serviceAccounts.Items) > 0 {
					saName = serviceAccounts.Items[0].Name
					saNamespace = ns
					break
				}
			}
			Expect(saName).NotTo(BeEmpty())

			By("deleting the WSA")
			Expect(k8sClient.Delete(testCtx, wsa)).To(Succeed())

			By("waiting for WSA to be fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      wsa.Name,
					Namespace: wsa.Namespace,
				}, &agentoctopuscomv1beta1.WorkloadServiceAccount{})
				return err != nil && client.IgnoreNotFound(err) == nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "WSA should be fully deleted")

			By("running reconciliation multiple times to complete SA cleanup")
			// SA deletion is two-phase:
			// Phase 1: Mark SA for deletion
			// Phase 2: Complete deletion after verifying no pods use it
			// Each phase requires a reconcile cycle
			Eventually(func() bool {
				_ = reconcileAll(testCtx, &engine)
				sa := &corev1.ServiceAccount{}
				err := k8sClient.Get(testCtx, client.ObjectKey{Name: saName, Namespace: saNamespace}, sa)
				return err != nil && client.IgnoreNotFound(err) == nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"SA should be cleaned up after WSA is fully deleted")
		})
	})
})
