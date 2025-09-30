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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

var _ = Describe("Engine Integration Tests", func() {
	var (
		engine           InMemoryEngine
		targetNamespaces []string
		testNamespace    string
		cleanNamespaces  []*corev1.Namespace
		testCtx          context.Context
		testCancel       context.CancelFunc
	)

	BeforeEach(func() {
		// Create a fresh context for each test with a timeout
		testCtx, testCancel = context.WithTimeout(context.Background(), 30*time.Second)

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
		engine = NewInMemoryEngine(targetNamespaces, k8sClient)
	})

	AfterEach(func() {
		// Cancel the test context
		testCancel()

		// Clean up all child resources created in the test namespaces
		namespaces := make([]client.Object, 0, len(cleanNamespaces))
		for _, ns := range cleanNamespaces {
			namespaces = append(namespaces, ns)
		}
		deleteAll(namespaces...)
	})

	Context("When reconciling WorkloadServiceAccounts", func() {
		It("should create service accounts with proper labels and annotations", func() {
			By("creating a WorkloadServiceAccount with specific scope")
			wsa := createTestWSA("test-wsa-1", testNamespace, map[string][]string{
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
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying service accounts are created in target namespaces")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{PermissionsKey: "enabled"})).To(Succeed())

				Expect(serviceAccounts.Items).NotTo(BeEmpty(), fmt.Sprintf("Expected service accounts in namespace %s", ns))

				for _, sa := range serviceAccounts.Items {
					By(fmt.Sprintf("verifying service account %s in namespace %s", sa.Name, ns))

					// Verify name pattern
					Expect(sa.Name).To(ContainSubstring("octopus-sa-"), "Service account should follow naming pattern")

					// Verify labels
					Expect(sa.Labels).To(HaveKey(PermissionsKey))
					Expect(sa.Labels[PermissionsKey]).To(Equal("enabled"))
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

			wsa := createTestWSA("test-wsa-2", testNamespace, map[string][]string{
				"projects": {"mobile-app"},
			}, permissions)
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying roles are created")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{PermissionsKey: "enabled"})).To(Succeed())

			Expect(roles.Items).NotTo(BeEmpty(), "Expected roles to be created")

			for _, role := range roles.Items {
				By(fmt.Sprintf("verifying role %s", role.Name))

				// Verify name pattern
				Expect(role.Name).To(ContainSubstring("octopus-role-"), "Role should follow naming pattern")

				// Verify permissions are correctly set
				Expect(role.Rules).To(Equal(permissions), "Role rules should match WSA permissions")

				// Verify labels
				Expect(role.Labels).To(HaveKey(PermissionsKey))
				Expect(role.Labels[PermissionsKey]).To(Equal("enabled"))
			}
		})

		It("should create role bindings connecting service accounts to roles", func() {
			By("creating a WorkloadServiceAccount with both inline permissions and role references")
			wsa := createTestWSAWithRoles("test-wsa-3", testNamespace, map[string][]string{
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
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying role bindings are created")
			roleBindings := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(testCtx, roleBindings, client.InNamespace(testNamespace))).To(Succeed())

			// Should have role bindings for inline permissions and role references
			wsaRoleBindings := filterRoleBindingsByPrefix(roleBindings.Items, "octopus-rb-")
			Expect(wsaRoleBindings).NotTo(BeEmpty(), "Expected role bindings to be created")

			for _, rb := range wsaRoleBindings {
				By(fmt.Sprintf("verifying role binding %s", rb.Name))

				// Verify name pattern
				Expect(rb.Name).To(ContainSubstring("octopus-rb-"), "RoleBinding should follow naming pattern")

				// Verify subjects include service accounts from all target namespaces
				Expect(rb.Subjects).NotTo(BeEmpty(), "RoleBinding should have subjects")

				for _, subject := range rb.Subjects {
					Expect(subject.Kind).To(Equal("ServiceAccount"), "Subject should be ServiceAccount")
					Expect(subject.Name).To(ContainSubstring("octopus-sa-"), "Subject name should match service account pattern")
					Expect(targetNamespaces).To(ContainElement(subject.Namespace), "Subject namespace should be in target namespaces")
				}
			}

			By("verifying cluster role bindings are created")
			clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
			Expect(k8sClient.List(testCtx, clusterRoleBindings)).To(Succeed())

			wsaClusterRoleBindings := filterClusterRoleBindingsByPrefix(clusterRoleBindings.Items, "octopus-crb-")
			Expect(wsaClusterRoleBindings).NotTo(BeEmpty(), "Expected cluster role bindings to be created")

			for _, crb := range wsaClusterRoleBindings {
				By(fmt.Sprintf("verifying cluster role binding %s", crb.Name))

				// Verify name pattern
				Expect(crb.Name).To(ContainSubstring("octopus-crb-"), "ClusterRoleBinding should follow naming pattern")

				// Verify role ref points to cluster role
				Expect(crb.RoleRef.Kind).To(Equal("ClusterRole"))
				Expect(crb.RoleRef.Name).To(Equal("view"))

				// Verify subjects
				Expect(crb.Subjects).NotTo(BeEmpty(), "ClusterRoleBinding should have subjects")
			}
		})

		It("should handle multiple WSAs with overlapping scopes correctly", func() {
			By("creating multiple WorkloadServiceAccounts with overlapping scopes")

			// WSA 1: Projects web-app, mobile-app
			wsa1 := createTestWSA("test-wsa-overlap-1", testNamespace, map[string][]string{
				"projects": {"web-app", "mobile-app"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa1)).To(Succeed())

			// WSA 2: Projects mobile-app, api-service (overlaps on mobile-app)
			wsa2 := createTestWSA("test-wsa-overlap-2", testNamespace, map[string][]string{
				"projects": {"mobile-app", "api-service"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa2)).To(Succeed())

			// WSA 3: Different scope entirely
			wsa3 := createTestWSA("test-wsa-overlap-3", testNamespace, map[string][]string{
				"environments": {"production"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa3)).To(Succeed())

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying correct number of unique service accounts are created")
			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{PermissionsKey: "enabled"})).To(Succeed())

				// Should have service accounts for each unique scope combination
				// This depends on how the scope intersection algorithm works
				Expect(serviceAccounts.Items).NotTo(BeEmpty(), fmt.Sprintf("Expected service accounts in namespace %s", ns))

				// Verify all service accounts have proper naming
				for _, sa := range serviceAccounts.Items {
					Expect(sa.Name).To(ContainSubstring("octopus-sa-"), "All service accounts should follow naming pattern")
				}
			}

			By("verifying roles are created for each WSA")
			roles := &rbacv1.RoleList{}
			Expect(k8sClient.List(testCtx, roles, client.InNamespace(testNamespace),
				client.MatchingLabels{PermissionsKey: "enabled"})).To(Succeed())

			// Should have 3 roles (one per WSA)
			Expect(roles.Items).To(HaveLen(3), "Expected one role per WSA")
		})

		It("should create service accounts in multiple target namespaces", func() {
			By("creating a WorkloadServiceAccount")
			wsa := createTestWSA("test-wsa-multi-ns", testNamespace, map[string][]string{
				"spaces": {"default-space"},
			}, []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
			})
			Expect(k8sClient.Create(testCtx, wsa)).To(Succeed())

			By("running reconciliation")
			Expect(engine.Reconcile(testCtx)).To(Succeed())

			By("verifying service accounts are created in all target namespaces")
			var allServiceAccounts []corev1.ServiceAccount

			for _, ns := range targetNamespaces {
				serviceAccounts := &corev1.ServiceAccountList{}
				Expect(k8sClient.List(testCtx, serviceAccounts, client.InNamespace(ns),
					client.MatchingLabels{PermissionsKey: "enabled"})).To(Succeed())

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
})

// Helper functions

func createTestWSA(
	name, namespace string, scopes map[string][]string, permissions []rbacv1.PolicyRule,
) *agentoctopuscomv1beta1.WorkloadServiceAccount {
	wsa := &agentoctopuscomv1beta1.WorkloadServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agentoctopuscomv1beta1.WorkloadServiceAccountSpec{
			Scope: agentoctopuscomv1beta1.WorkloadServiceAccountScope{},
			Permissions: agentoctopuscomv1beta1.WorkloadServiceAccountPermissions{
				Permissions: permissions,
			},
		},
	}

	// Set scopes
	if projects, ok := scopes["projects"]; ok {
		wsa.Spec.Scope.Projects = projects
	}
	if environments, ok := scopes["environments"]; ok {
		wsa.Spec.Scope.Environments = environments
	}
	if tenants, ok := scopes["tenants"]; ok {
		wsa.Spec.Scope.Tenants = tenants
	}
	if steps, ok := scopes["steps"]; ok {
		wsa.Spec.Scope.Steps = steps
	}
	if spaces, ok := scopes["spaces"]; ok {
		wsa.Spec.Scope.Spaces = spaces
	}

	return wsa
}

func createTestWSAWithRoles(
	name, namespace string, scopes map[string][]string, permissions []rbacv1.PolicyRule, roles []rbacv1.RoleRef,
	clusterRoles []rbacv1.RoleRef,
) *agentoctopuscomv1beta1.WorkloadServiceAccount {
	wsa := createTestWSA(name, namespace, scopes, permissions)
	wsa.Spec.Permissions.Roles = roles
	wsa.Spec.Permissions.ClusterRoles = clusterRoles
	return wsa
}

func filterRoleBindingsByPrefix(roleBindings []rbacv1.RoleBinding, prefix string) []rbacv1.RoleBinding {
	var filtered []rbacv1.RoleBinding
	for _, rb := range roleBindings {
		if len(rb.Name) >= len(prefix) && rb.Name[:len(prefix)] == prefix {
			filtered = append(filtered, rb)
		}
	}
	return filtered
}

func filterClusterRoleBindingsByPrefix(
	clusterRoleBindings []rbacv1.ClusterRoleBinding, prefix string,
) []rbacv1.ClusterRoleBinding {
	var filtered []rbacv1.ClusterRoleBinding
	for _, crb := range clusterRoleBindings {
		if len(crb.Name) >= len(prefix) && crb.Name[:len(prefix)] == prefix {
			filtered = append(filtered, crb)
		}
	}
	return filtered
}

func deleteAll(objs ...client.Object) {
	ctx := context.Background()
	clientGo, err := kubernetes.NewForConfig(testEnv.Config)
	Expect(err).ShouldNot(HaveOccurred())
	for _, obj := range objs {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).Should(Succeed())

		if ns, ok := obj.(*corev1.Namespace); ok {

			// Look up all namespaced resources under the discovery API
			_, apiResources, err := clientGo.Discovery().ServerGroupsAndResources()
			Expect(err).ShouldNot(HaveOccurred())
			namespacedGVKs := make(map[string]schema.GroupVersionKind)
			for _, apiResourceList := range apiResources {
				defaultGV, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
				Expect(err).ShouldNot(HaveOccurred())
				for _, r := range apiResourceList.APIResources {
					if !r.Namespaced || strings.Contains(r.Name, "/") {
						// skip non-namespaced and subresources
						continue
					}
					gvk := schema.GroupVersionKind{
						Group:   defaultGV.Group,
						Version: defaultGV.Version,
						Kind:    r.Kind,
					}
					if r.Group != "" {
						gvk.Group = r.Group
					}
					if r.Version != "" {
						gvk.Version = r.Version
					}
					namespacedGVKs[gvk.String()] = gvk
				}
			}

			// Delete all namespaced resources in this namespace
			for _, gvk := range namespacedGVKs {
				var u unstructured.Unstructured
				u.SetGroupVersionKind(gvk)
				err := k8sClient.DeleteAllOf(ctx, &u, client.InNamespace(ns.Name))
				Expect(client.IgnoreNotFound(ignoreMethodNotAllowed(err))).ShouldNot(HaveOccurred())
			}

			Eventually(func() error {
				key := client.ObjectKeyFromObject(ns)
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return client.IgnoreNotFound(err)
				}
				// remove `kubernetes` finalizer
				finalizers := []corev1.FinalizerName{}
				for _, f := range ns.Spec.Finalizers {
					if f != "kubernetes" {
						finalizers = append(finalizers, f)
					}
				}
				ns.Spec.Finalizers = finalizers

				// We have to use the k8s.io/client-go library here to expose
				// ability to patch the /finalize subresource on the namespace
				_, err = clientGo.CoreV1().Namespaces().Finalize(ctx, ns, metav1.UpdateOptions{})
				return err
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())
		}

		Eventually(func() metav1.StatusReason {
			key := client.ObjectKeyFromObject(obj)
			if err := k8sClient.Get(ctx, key, obj); err != nil {
				return errors.ReasonForError(err)
			}
			return ""
		}, 30*time.Second, 500*time.Millisecond).Should(Equal(metav1.StatusReasonNotFound))
	}
}

func ignoreMethodNotAllowed(err error) error {
	if err != nil {
		if errors.ReasonForError(err) == metav1.StatusReasonMethodNotAllowed {
			return nil
		}
	}
	return err
}
