package rules

import (
	"context"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("ResourceManagementService", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1beta1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("GetWorkloadServiceAccounts", func() {
		Context("When retrieving WorkloadServiceAccounts", func() {
			It("Should return empty iterator when no workload service accounts exist", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				iterator, err := service.GetWorkloadServiceAccounts(context.Background())

				Expect(err).NotTo(HaveOccurred())
				Expect(iterator).NotTo(BeNil())

				count := 0
				for range iterator {
					count++
				}
				Expect(count).To(Equal(0))
			})

			It("Should return iterator with single workload service account", func() {
				workloadServiceAccounts := []v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"project-1"},
							},
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Permissions: []rbacv1.PolicyRule{
									{
										APIGroups: []string{""},
										Resources: []string{"pods"},
										Verbs:     []string{"get", "list"},
									},
								},
							},
						},
					},
				}

				var objects []client.Object
				for i := range workloadServiceAccounts {
					objects = append(objects, &workloadServiceAccounts[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(objects...).
					Build()

				service := NewResourceManagementService(fakeClient)
				iterator, err := service.GetWorkloadServiceAccounts(context.Background())

				Expect(err).NotTo(HaveOccurred())
				Expect(iterator).NotTo(BeNil())

				var wsaNames []string
				for wsa := range iterator {
					wsaNames = append(wsaNames, wsa.Name)
				}

				Expect(wsaNames).To(ConsistOf("test-wsa-1"))
			})

			It("Should return all workload service accounts", func() {
				workloadServiceAccounts := []v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Projects: []string{"project-1"},
							},
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Permissions: []rbacv1.PolicyRule{
									{
										APIGroups: []string{""},
										Resources: []string{"pods"},
										Verbs:     []string{"get", "list"},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-2",
							Namespace: "test-namespace",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Scope: v1beta1.WorkloadServiceAccountScope{
								Environments: []string{"production"},
							},
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Roles: []rbacv1.RoleRef{
									{
										Kind:     "Role",
										Name:     "test-role",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
							},
						},
					},
				}

				var objects []client.Object
				for i := range workloadServiceAccounts {
					objects = append(objects, &workloadServiceAccounts[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(objects...).
					Build()

				service := NewResourceManagementService(fakeClient)
				iterator, err := service.GetWorkloadServiceAccounts(context.Background())

				Expect(err).NotTo(HaveOccurred())
				Expect(iterator).NotTo(BeNil())

				var wsaNames []string
				for wsa := range iterator {
					wsaNames = append(wsaNames, wsa.Name)
				}

				Expect(wsaNames).To(ConsistOf("test-wsa-1", "test-wsa-2"))
			})
		})
	})

	Describe("EnsureRoles", func() {
		var rbacScheme *runtime.Scheme

		BeforeEach(func() {
			rbacScheme = runtime.NewScheme()
			Expect(rbacv1.AddToScheme(rbacScheme)).To(Succeed())
		})

		Context("When ensuring roles for WorkloadServiceAccounts", func() {
			It("Should return empty roles map for empty WSA list", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(rbacScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				createdRoles, err := service.EnsureRoles(context.Background(), []WSAResource{})

				Expect(err).NotTo(HaveOccurred())
				Expect(createdRoles).To(BeEmpty())
			})

			It("Should not create role for WSA with no permissions", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbacScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				createdRoles, err := service.EnsureRoles(context.Background(), wsaResources)

				Expect(err).NotTo(HaveOccurred())
				Expect(createdRoles).To(BeEmpty())
			})

			It("Should create role for WSA with permissions", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Permissions: []rbacv1.PolicyRule{
									{
										APIGroups: []string{""},
										Resources: []string{"pods"},
										Verbs:     []string{"get", "list"},
									},
								},
							},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbacScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				createdRoles, err := service.EnsureRoles(context.Background(), wsaResources)

				Expect(err).NotTo(HaveOccurred())
				Expect(createdRoles).To(HaveLen(1))

				for wsaKey, role := range createdRoles {
					Expect(role.Name).NotTo(BeEmpty())
					Expect(role.Name).To(ContainSubstring("octopus-role-"))
					Expect(role.Labels[ManagedByLabel]).To(Equal(ManagedByValue))

					var correspondingWSA *v1beta1.WorkloadServiceAccount
					for _, wsa := range wsaList {
						if wsa.Name == wsaKey.Name && wsa.Namespace == wsaKey.Namespace {
							correspondingWSA = wsa
							break
						}
					}
					Expect(correspondingWSA).NotTo(BeNil(), "Should find corresponding WSA for role")
					Expect(role.Rules).To(Equal(correspondingWSA.Spec.Permissions.Permissions))
				}
			})

			It("Should return existing role without creating new one", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Permissions: []rbacv1.PolicyRule{
									{
										APIGroups: []string{""},
										Resources: []string{"pods"},
										Verbs:     []string{"get", "list"},
									},
								},
							},
						},
					},
				}

				existingRoles := []rbacv1.Role{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "octopus-role-" + shortHash(`[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]`),
							Namespace: "default",
							Labels: map[string]string{
								ManagedByLabel: ManagedByValue,
							},
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{""},
								Resources: []string{"pods"},
								Verbs:     []string{"get", "list"},
							},
						},
					},
				}

				var objects []client.Object
				for i := range existingRoles {
					objects = append(objects, &existingRoles[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbacScheme).
					WithObjects(objects...).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				createdRoles, err := service.EnsureRoles(context.Background(), wsaResources)

				Expect(err).NotTo(HaveOccurred())
				Expect(createdRoles).To(HaveLen(1))
			})
		})
	})

	Describe("EnsureServiceAccounts", func() {
		var saScheme *runtime.Scheme

		BeforeEach(func() {
			saScheme = runtime.NewScheme()
			Expect(corev1.AddToScheme(saScheme)).To(Succeed())
		})

		Context("When ensuring service accounts in target namespaces", func() {
			It("Should do nothing with empty service accounts list", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(saScheme).
					Build()

				initialList := &corev1.ServiceAccountList{}
				err := fakeClient.List(context.Background(), initialList)
				Expect(err).NotTo(HaveOccurred())
				initialCount := len(initialList.Items)

				service := NewResourceManagementService(fakeClient)
				err = service.EnsureServiceAccounts(context.Background(), []*corev1.ServiceAccount{}, []string{"default"})

				Expect(err).NotTo(HaveOccurred())

				finalList := &corev1.ServiceAccountList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(HaveLen(initialCount))
			})

			It("Should create single service account in single namespace", func() {
				serviceAccounts := []*corev1.ServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-sa",
						},
					},
				}
				targetNamespaces := []string{"default"}

				fakeClient := fake.NewClientBuilder().
					WithScheme(saScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				err := service.EnsureServiceAccounts(context.Background(), serviceAccounts, targetNamespaces)

				Expect(err).NotTo(HaveOccurred())

				createdSA := &corev1.ServiceAccount{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{
					Name:      "test-sa",
					Namespace: "default",
				}, createdSA)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdSA.Namespace).To(Equal("default"))
			})

			It("Should create single service account in multiple namespaces", func() {
				serviceAccounts := []*corev1.ServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-sa",
						},
					},
				}
				targetNamespaces := []string{"default", "test-namespace", "production"}

				fakeClient := fake.NewClientBuilder().
					WithScheme(saScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				err := service.EnsureServiceAccounts(context.Background(), serviceAccounts, targetNamespaces)

				Expect(err).NotTo(HaveOccurred())

				for _, namespace := range targetNamespaces {
					createdSA := &corev1.ServiceAccount{}
					err = fakeClient.Get(context.Background(), client.ObjectKey{
						Name:      "test-sa",
						Namespace: namespace,
					}, createdSA)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdSA.Namespace).To(Equal(namespace))
				}
			})

			It("Should create multiple service accounts in multiple namespaces", func() {
				serviceAccounts := []*corev1.ServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-sa-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-sa-2",
						},
					},
				}
				targetNamespaces := []string{"default", "test-namespace"}

				fakeClient := fake.NewClientBuilder().
					WithScheme(saScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				err := service.EnsureServiceAccounts(context.Background(), serviceAccounts, targetNamespaces)

				Expect(err).NotTo(HaveOccurred())

				finalList := &corev1.ServiceAccountList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				// Should create 2 SAs x 2 namespaces = 4 total
				Expect(finalList.Items).To(HaveLen(4))
			})

			It("Should not create service account that already exists", func() {
				existingServiceAccounts := []corev1.ServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-sa",
							Namespace: "default",
						},
					},
				}

				var objects []client.Object
				for i := range existingServiceAccounts {
					objects = append(objects, &existingServiceAccounts[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(saScheme).
					WithObjects(objects...).
					Build()

				initialList := &corev1.ServiceAccountList{}
				err := fakeClient.List(context.Background(), initialList)
				Expect(err).NotTo(HaveOccurred())
				initialCount := len(initialList.Items)

				serviceAccounts := []*corev1.ServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-sa",
						},
					},
				}
				targetNamespaces := []string{"default"}

				service := NewResourceManagementService(fakeClient)
				err = service.EnsureServiceAccounts(context.Background(), serviceAccounts, targetNamespaces)

				Expect(err).NotTo(HaveOccurred())

				finalList := &corev1.ServiceAccountList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(HaveLen(initialCount)) // No new accounts created
			})
		})
	})

	Describe("EnsureRoleBindings", func() {
		var rbScheme *runtime.Scheme

		BeforeEach(func() {
			rbScheme = runtime.NewScheme()
			Expect(rbacv1.AddToScheme(rbScheme)).To(Succeed())
		})

		Context("When ensuring role bindings for WorkloadServiceAccounts", func() {
			It("Should create no role bindings for empty WSA list", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(rbScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				err := service.EnsureRoleBindings(context.Background(), []WSAResource{}, map[types.NamespacedName]rbacv1.Role{}, map[types.NamespacedName][]string{}, []string{"default"})

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(BeEmpty())
			})

			It("Should create role binding for WSA with inline permissions", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Permissions: []rbacv1.PolicyRule{
									{
										APIGroups: []string{""},
										Resources: []string{"pods"},
										Verbs:     []string{"get", "list"},
									},
								},
							},
						},
					},
				}

				createdRoles := map[types.NamespacedName]rbacv1.Role{
					{Namespace: "default", Name: "test-wsa-1"}: {
						ObjectMeta: metav1.ObjectMeta{
							Name:      "octopus-role-test",
							Namespace: "default",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{""},
								Resources: []string{"pods"},
								Verbs:     []string{"get", "list"},
							},
						},
					},
				}

				wsaToServiceAccounts := map[types.NamespacedName][]string{
					{Namespace: "default", Name: "test-wsa-1"}: {"test-sa-1"},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				err := service.EnsureRoleBindings(context.Background(), wsaResources, createdRoles, wsaToServiceAccounts, []string{"default", "test-namespace"})

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				// Should create 1 binding in the WSA's namespace (default)
				Expect(finalList.Items).To(HaveLen(1))
			})

			It("Should create role binding for WSA with explicit roles", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Roles: []rbacv1.RoleRef{
									{
										Kind:     "Role",
										Name:     "test-role",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
							},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				err := service.EnsureRoleBindings(context.Background(), wsaResources, map[types.NamespacedName]rbacv1.Role{}, map[types.NamespacedName][]string{{Namespace: "default", Name: "test-wsa-1"}: {"test-sa-1"}}, []string{"default"})

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(HaveLen(1))
			})

			It("Should create role binding for WSA with cluster roles", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								ClusterRoles: []rbacv1.RoleRef{
									{
										Kind:     "ClusterRole",
										Name:     "test-cluster-role",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
							},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				err := service.EnsureRoleBindings(context.Background(), wsaResources, map[types.NamespacedName]rbacv1.Role{}, map[types.NamespacedName][]string{{Namespace: "default", Name: "test-wsa-1"}: {"test-sa-1"}}, []string{"default"})

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(HaveLen(1))
			})

			It("Should create multiple role bindings for WSA with multiple roles", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Roles: []rbacv1.RoleRef{
									{
										Kind:     "Role",
										Name:     "test-role-1",
										APIGroup: "rbac.authorization.k8s.io",
									},
									{
										Kind:     "Role",
										Name:     "test-role-2",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
								ClusterRoles: []rbacv1.RoleRef{
									{
										Kind:     "ClusterRole",
										Name:     "test-cluster-role",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
							},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				err := service.EnsureRoleBindings(context.Background(), wsaResources, map[types.NamespacedName]rbacv1.Role{}, map[types.NamespacedName][]string{{Namespace: "default", Name: "test-wsa-1"}: {"test-sa-1"}}, []string{"default"})

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(HaveLen(3)) // 2 regular roles + 1 cluster role
			})

			It("Should not create role bindings for WSA without service accounts", func() {
				wsaList := []*v1beta1.WorkloadServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-wsa-1",
							Namespace: "default",
						},
						Spec: v1beta1.WorkloadServiceAccountSpec{
							Permissions: v1beta1.WorkloadServiceAccountPermissions{
								Roles: []rbacv1.RoleRef{
									{
										Kind:     "Role",
										Name:     "test-role",
										APIGroup: "rbac.authorization.k8s.io",
									},
								},
							},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				wsaResources := make([]WSAResource, len(wsaList))
				for i, wsa := range wsaList {
					wsaResources[i] = NewWSAResource(wsa)
				}
				err := service.EnsureRoleBindings(context.Background(), wsaResources, map[types.NamespacedName]rbacv1.Role{}, map[types.NamespacedName][]string{}, []string{"default"})

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(BeEmpty())
			})
		})
	})
	Describe("createRoleIfNeeded", func() {
		var roleScheme *runtime.Scheme

		BeforeEach(func() {
			roleScheme = runtime.NewScheme()
			Expect(rbacv1.AddToScheme(roleScheme)).To(Succeed())
		})

		Context("When creating roles conditionally", func() {
			It("Should return empty role for WSA with no permissions", func() {
				wsa := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa",
						Namespace: "default",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Permissions: v1beta1.WorkloadServiceAccountPermissions{},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(roleScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				role, err := service.createRoleIfNeeded(context.Background(), NewWSAResource(wsa))

				Expect(err).NotTo(HaveOccurred())
				Expect(role.Name).To(BeEmpty())
			})

			It("Should create new role for WSA with permissions", func() {
				wsa := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa",
						Namespace: "default",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Permissions: v1beta1.WorkloadServiceAccountPermissions{
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

				fakeClient := fake.NewClientBuilder().
					WithScheme(roleScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				role, err := service.createRoleIfNeeded(context.Background(), NewWSAResource(wsa))

				Expect(err).NotTo(HaveOccurred())
				Expect(role.Name).NotTo(BeEmpty())
				Expect(role.Name).To(ContainSubstring("octopus-role-"))
				Expect(role.Namespace).To(Equal(wsa.Namespace))
				Expect(role.Rules).To(Equal(wsa.Spec.Permissions.Permissions))
				Expect(role.Labels[ManagedByLabel]).To(Equal(ManagedByValue))
			})

			It("Should return existing role without creating new one", func() {
				wsa := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa",
						Namespace: "default",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Permissions: v1beta1.WorkloadServiceAccountPermissions{
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

				existingRoles := []rbacv1.Role{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "octopus-role-" + shortHash(`[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]`),
							Namespace: "default",
							Labels: map[string]string{
								ManagedByLabel: ManagedByValue,
							},
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{""},
								Resources: []string{"pods"},
								Verbs:     []string{"get", "list"},
							},
						},
					},
				}

				var objects []client.Object
				for i := range existingRoles {
					objects = append(objects, &existingRoles[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(roleScheme).
					WithObjects(objects...).
					Build()

				service := NewResourceManagementService(fakeClient)
				role, err := service.createRoleIfNeeded(context.Background(), NewWSAResource(wsa))

				Expect(err).NotTo(HaveOccurred())
				Expect(role.Name).NotTo(BeEmpty())
			})
		})
	})

	Describe("createServiceAccount", func() {
		var saCreateScheme *runtime.Scheme

		BeforeEach(func() {
			saCreateScheme = runtime.NewScheme()
			Expect(corev1.AddToScheme(saCreateScheme)).To(Succeed())
		})

		Context("When creating service accounts", func() {
			It("Should create new service account", func() {
				namespace := "test-namespace"
				templateServiceAccount := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sa",
						Labels: map[string]string{
							"app": "test",
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(saCreateScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				err := service.createServiceAccount(context.Background(), namespace, templateServiceAccount)

				Expect(err).NotTo(HaveOccurred())

				createdSA := &corev1.ServiceAccount{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{
					Name:      templateServiceAccount.Name,
					Namespace: namespace,
				}, createdSA)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdSA.Namespace).To(Equal(namespace))
				Expect(createdSA.Name).To(Equal(templateServiceAccount.Name))

				for key, value := range templateServiceAccount.Labels {
					Expect(createdSA.Labels[key]).To(Equal(value))
				}
			})

			It("Should not create service account that already exists", func() {
				namespace := "test-namespace"
				templateServiceAccount := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sa",
					},
				}

				existingServiceAccounts := []corev1.ServiceAccount{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-sa",
							Namespace: "test-namespace",
						},
					},
				}

				var objects []client.Object
				for i := range existingServiceAccounts {
					objects = append(objects, &existingServiceAccounts[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(saCreateScheme).
					WithObjects(objects...).
					Build()

				service := NewResourceManagementService(fakeClient)
				err := service.createServiceAccount(context.Background(), namespace, templateServiceAccount)

				Expect(err).NotTo(HaveOccurred())

				createdSA := &corev1.ServiceAccount{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{
					Name:      templateServiceAccount.Name,
					Namespace: namespace,
				}, createdSA)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("createRoleBinding", func() {
		var rbCreateScheme *runtime.Scheme

		BeforeEach(func() {
			rbCreateScheme = runtime.NewScheme()
			Expect(rbacv1.AddToScheme(rbCreateScheme)).To(Succeed())
		})

		Context("When creating role bindings", func() {
			It("Should create new role binding", func() {
				wsa := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa",
						Namespace: "default",
					},
				}

				roleRef := rbacv1.RoleRef{
					Kind:     "Role",
					Name:     "test-role",
					APIGroup: "rbac.authorization.k8s.io",
				}

				subjects := []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test-sa",
						Namespace: "default",
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbCreateScheme).
					Build()

				service := NewResourceManagementService(fakeClient)
				err := service.createRoleBinding(context.Background(), NewWSAResource(wsa), roleRef, subjects)

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				// Should create 1 role binding
				Expect(finalList.Items).To(HaveLen(1))

				var createdRB *rbacv1.RoleBinding
				for i := range finalList.Items {
					createdRB = &finalList.Items[i]
					break
				}

				Expect(createdRB).NotTo(BeNil())
				Expect(createdRB.Name).To(ContainSubstring("octopus-rb-"))
				Expect(createdRB.Namespace).To(Equal(wsa.Namespace))
				Expect(createdRB.RoleRef).To(Equal(roleRef))
				Expect(createdRB.Subjects).To(Equal(subjects))
			})

			It("Should not create role binding that already exists", func() {
				wsa := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa",
						Namespace: "default",
					},
				}

				roleRef := rbacv1.RoleRef{
					Kind:     "Role",
					Name:     "test-role",
					APIGroup: "rbac.authorization.k8s.io",
				}

				subjects := []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test-sa",
						Namespace: "default",
					},
				}

				existingRoleBindings := []rbacv1.RoleBinding{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "octopus-rb-" + shortHash("test-wsa-test-role"),
							Namespace: "default",
						},
						RoleRef: rbacv1.RoleRef{
							Kind:     "Role",
							Name:     "test-role",
							APIGroup: "rbac.authorization.k8s.io",
						},
						Subjects: []rbacv1.Subject{
							{
								Kind:      "ServiceAccount",
								Name:      "test-sa",
								Namespace: "default",
							},
						},
					},
				}

				var objects []client.Object
				for i := range existingRoleBindings {
					objects = append(objects, &existingRoleBindings[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(rbCreateScheme).
					WithObjects(objects...).
					Build()

				initialList := &rbacv1.RoleBindingList{}
				err := fakeClient.List(context.Background(), initialList)
				Expect(err).NotTo(HaveOccurred())
				initialCount := len(initialList.Items)

				service := NewResourceManagementService(fakeClient)
				err = service.createRoleBinding(context.Background(), NewWSAResource(wsa), roleRef, subjects)

				Expect(err).NotTo(HaveOccurred())

				finalList := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), finalList)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalList.Items).To(HaveLen(initialCount)) // No new bindings created
			})
		})
	})
})

var _ = Describe("shortHash", func() {
	Context("When generating short hashes", func() {
		It("Should return 32 character hash for empty string", func() {
			result := shortHash("")
			Expect(result).To(HaveLen(32))

			for _, char := range result {
				Expect((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')).To(BeTrue(),
					"Hash should only contain hex characters")
			}

			result2 := shortHash("")
			Expect(result).To(Equal(result2), "Same input should produce same hash")
		})

		It("Should return 32 character hash for simple string", func() {
			result := shortHash("test")
			Expect(result).To(HaveLen(32))

			for _, char := range result {
				Expect((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')).To(BeTrue(),
					"Hash should only contain hex characters")
			}

			result2 := shortHash("test")
			Expect(result).To(Equal(result2), "Same input should produce same hash")
		})

		It("Should return 32 character hash for complex string", func() {
			result := shortHash(`[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]`)
			Expect(result).To(HaveLen(32))

			for _, char := range result {
				Expect((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')).To(BeTrue(),
					"Hash should only contain hex characters")
			}

			result2 := shortHash(`[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]`)
			Expect(result).To(Equal(result2), "Same input should produce same hash")
		})
	})
})
