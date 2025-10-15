package rules

import (
	"context"
	"testing"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceManagementService_GetWorkloadServiceAccounts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))

	tests := []struct {
		name                    string
		workloadServiceAccounts []v1beta1.WorkloadServiceAccount
		expectedCount           int
		expectedError           bool
	}{
		{
			name:                    "no workload service accounts should return empty iterator",
			workloadServiceAccounts: []v1beta1.WorkloadServiceAccount{},
			expectedCount:           0,
			expectedError:           false,
		},
		{
			name: "single workload service account should return one item",
			workloadServiceAccounts: []v1beta1.WorkloadServiceAccount{
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
			},
			expectedCount: 1,
			expectedError: false,
		},
		{
			name: "multiple workload service accounts should return all items",
			workloadServiceAccounts: []v1beta1.WorkloadServiceAccount{
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
			},
			expectedCount: 2,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for i := range tt.workloadServiceAccounts {
				objects = append(objects, &tt.workloadServiceAccounts[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			service := NewResourceManagementService(fakeClient)
			iterator, err := service.GetWorkloadServiceAccounts(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, iterator)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, iterator)

			count := 0
			var wsaNames []string
			for wsa := range iterator {
				count++
				wsaNames = append(wsaNames, wsa.Name)
			}

			assert.Equal(t, tt.expectedCount, count)

			if tt.expectedCount > 0 {
				expectedNames := make([]string, len(tt.workloadServiceAccounts))
				for i, wsa := range tt.workloadServiceAccounts {
					expectedNames[i] = wsa.Name
				}
				assert.ElementsMatch(t, expectedNames, wsaNames)
			}
		})
	}
}

func TestResourceManagementService_EnsureRoles(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	tests := []struct {
		name          string
		wsaList       []*v1beta1.WorkloadServiceAccount
		existingRoles []rbacv1.Role
		expectedRoles int
		expectedError bool
	}{
		{
			name:          "empty WSA list should return empty roles map",
			wsaList:       []*v1beta1.WorkloadServiceAccount{},
			existingRoles: []rbacv1.Role{},
			expectedRoles: 0,
			expectedError: false,
		},
		{
			name: "WSA with no permissions should not create role",
			wsaList: []*v1beta1.WorkloadServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa-1",
						Namespace: "default",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Permissions: v1beta1.WorkloadServiceAccountPermissions{},
					},
				},
			},
			existingRoles: []rbacv1.Role{},
			expectedRoles: 0,
			expectedError: false,
		},
		{
			name: "WSA with permissions should create role",
			wsaList: []*v1beta1.WorkloadServiceAccount{
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
			},
			existingRoles: []rbacv1.Role{},
			expectedRoles: 1,
			expectedError: false,
		},
		{
			name: "existing role should be returned without creating new one",
			wsaList: []*v1beta1.WorkloadServiceAccount{
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
			},
			existingRoles: []rbacv1.Role{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "octopus-role-" + shortHash(`[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]`),
						Namespace: "default",
						Labels: map[string]string{
							PermissionsKey: "enabled",
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
			},
			expectedRoles: 1,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for i := range tt.existingRoles {
				objects = append(objects, &tt.existingRoles[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			service := NewResourceManagementService(fakeClient)
			createdRoles, err := service.EnsureRoles(context.Background(), tt.wsaList)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, createdRoles, tt.expectedRoles)

			for wsaName, role := range createdRoles {
				assert.NotEmpty(t, role.Name)
				assert.Contains(t, role.Name, "octopus-role-")
				assert.Equal(t, role.Labels[PermissionsKey], "enabled")

				var correspondingWSA *v1beta1.WorkloadServiceAccount
				for _, wsa := range tt.wsaList {
					if wsa.Name == wsaName {
						correspondingWSA = wsa
						break
					}
				}
				require.NotNil(t, correspondingWSA, "Should find corresponding WSA for role")
				assert.Equal(t, correspondingWSA.Spec.Permissions.Permissions, role.Rules)
			}
		})
	}
}

func TestResourceManagementService_EnsureServiceAccounts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name                    string
		serviceAccounts         []*corev1.ServiceAccount
		targetNamespaces        []string
		existingServiceAccounts []corev1.ServiceAccount
		expectedCreations       int
		expectedError           bool
	}{
		{
			name:              "empty service accounts should do nothing",
			serviceAccounts:   []*corev1.ServiceAccount{},
			targetNamespaces:  []string{"default"},
			expectedCreations: 0,
			expectedError:     false,
		},
		{
			name: "single service account in single namespace",
			serviceAccounts: []*corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sa",
					},
				},
			},
			targetNamespaces:  []string{"default"},
			expectedCreations: 1,
			expectedError:     false,
		},
		{
			name: "single service account in multiple namespaces",
			serviceAccounts: []*corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sa",
					},
				},
			},
			targetNamespaces:  []string{"default", "test-namespace", "production"},
			expectedCreations: 3,
			expectedError:     false,
		},
		{
			name: "multiple service accounts in multiple namespaces",
			serviceAccounts: []*corev1.ServiceAccount{
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
			},
			targetNamespaces:  []string{"default", "test-namespace"},
			expectedCreations: 4, // 2 service accounts × 2 namespaces
			expectedError:     false,
		},
		{
			name: "existing service account should not be created again",
			serviceAccounts: []*corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sa",
					},
				},
			},
			targetNamespaces: []string{"default"},
			existingServiceAccounts: []corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sa",
						Namespace: "default",
					},
				},
			},
			expectedCreations: 0, // Already exists, should not create
			expectedError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for i := range tt.existingServiceAccounts {
				objects = append(objects, &tt.existingServiceAccounts[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			initialList := &corev1.ServiceAccountList{}
			err := fakeClient.List(context.Background(), initialList)
			require.NoError(t, err)
			initialCount := len(initialList.Items)

			service := NewResourceManagementService(fakeClient)
			err = service.EnsureServiceAccounts(context.Background(), tt.serviceAccounts, tt.targetNamespaces)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			finalList := &corev1.ServiceAccountList{}
			err = fakeClient.List(context.Background(), finalList)
			require.NoError(t, err)
			finalCount := len(finalList.Items)

			assert.Equal(t, tt.expectedCreations, finalCount-initialCount)

			if tt.expectedCreations > 0 {
				for _, sa := range tt.serviceAccounts {
					for _, namespace := range tt.targetNamespaces {
						shouldExist := true
						for _, existing := range tt.existingServiceAccounts {
							if existing.Name == sa.Name && existing.Namespace == namespace {
								shouldExist = false
								break
							}
						}

						if shouldExist {
							createdSA := &corev1.ServiceAccount{}
							err := fakeClient.Get(context.Background(), client.ObjectKey{
								Name:      sa.Name,
								Namespace: namespace,
							}, createdSA)
							assert.NoError(t, err, "Service account should exist in namespace %s", namespace)
							assert.Equal(t, namespace, createdSA.Namespace)
						}
					}
				}
			}
		})
	}
}

func TestResourceManagementService_EnsureRoleBindings(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	tests := []struct {
		name                 string
		wsaList              []*v1beta1.WorkloadServiceAccount
		createdRoles         map[string]rbacv1.Role
		wsaToServiceAccounts map[string][]string
		targetNamespaces     []string
		existingRoleBindings []rbacv1.RoleBinding
		expectedRoleBindings int
		expectedError        bool
	}{
		{
			name:                 "empty WSA list should create no role bindings",
			wsaList:              []*v1beta1.WorkloadServiceAccount{},
			createdRoles:         map[string]rbacv1.Role{},
			wsaToServiceAccounts: map[string][]string{},
			targetNamespaces:     []string{"default"},
			expectedRoleBindings: 0,
			expectedError:        false,
		},
		{
			name: "WSA with inline permissions should create role binding",
			wsaList: []*v1beta1.WorkloadServiceAccount{
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
			},
			createdRoles: map[string]rbacv1.Role{
				"test-wsa-1": {
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
			},
			wsaToServiceAccounts: map[string][]string{
				"test-wsa-1": {"test-sa-1"},
			},
			targetNamespaces:     []string{"default", "test-namespace"},
			expectedRoleBindings: 1, // One role binding for the inline permissions
			expectedError:        false,
		},
		{
			name: "WSA with explicit roles should create role bindings",
			wsaList: []*v1beta1.WorkloadServiceAccount{
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
			},
			createdRoles: map[string]rbacv1.Role{},
			wsaToServiceAccounts: map[string][]string{
				"test-wsa-1": {"test-sa-1"},
			},
			targetNamespaces:     []string{"default"},
			expectedRoleBindings: 1, // One role binding for the explicit role
			expectedError:        false,
		},
		{
			name: "WSA with cluster roles should create role bindings",
			wsaList: []*v1beta1.WorkloadServiceAccount{
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
			},
			createdRoles: map[string]rbacv1.Role{},
			wsaToServiceAccounts: map[string][]string{
				"test-wsa-1": {"test-sa-1"},
			},
			targetNamespaces:     []string{"default"},
			expectedRoleBindings: 1, // One role binding for the cluster role
			expectedError:        false,
		},
		{
			name: "WSA with multiple roles should create multiple role bindings",
			wsaList: []*v1beta1.WorkloadServiceAccount{
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
			},
			createdRoles: map[string]rbacv1.Role{},
			wsaToServiceAccounts: map[string][]string{
				"test-wsa-1": {"test-sa-1"},
			},
			targetNamespaces:     []string{"default"},
			expectedRoleBindings: 3, // 2 regular roles + 1 cluster role
			expectedError:        false,
		},
		{
			name: "WSA without service accounts should not create role bindings",
			wsaList: []*v1beta1.WorkloadServiceAccount{
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
			},
			createdRoles:         map[string]rbacv1.Role{},
			wsaToServiceAccounts: map[string][]string{}, // No service accounts mapped
			targetNamespaces:     []string{"default"},
			expectedRoleBindings: 0,
			expectedError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for i := range tt.existingRoleBindings {
				objects = append(objects, &tt.existingRoleBindings[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			initialList := &rbacv1.RoleBindingList{}
			err := fakeClient.List(context.Background(), initialList)
			require.NoError(t, err)
			initialCount := len(initialList.Items)

			service := NewResourceManagementService(fakeClient)
			err = service.EnsureRoleBindings(context.Background(), tt.wsaList, tt.createdRoles, tt.wsaToServiceAccounts, tt.targetNamespaces)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			finalList := &rbacv1.RoleBindingList{}
			err = fakeClient.List(context.Background(), finalList)
			require.NoError(t, err)
			finalCount := len(finalList.Items)

			assert.Equal(t, tt.expectedRoleBindings, finalCount-initialCount)

			if tt.expectedRoleBindings > 0 {
				for _, rb := range finalList.Items {
					found := false
					for _, existing := range tt.existingRoleBindings {
						if existing.Name == rb.Name && existing.Namespace == rb.Namespace {
							found = true
							break
						}
					}
					if found {
						continue
					}

					assert.Contains(t, rb.Name, "octopus-rb-")
					assert.NotEmpty(t, rb.RoleRef.Name)
					assert.NotEmpty(t, rb.Subjects)

					for _, subject := range rb.Subjects {
						assert.Equal(t, "ServiceAccount", subject.Kind)
						assert.Contains(t, tt.targetNamespaces, subject.Namespace)
					}
				}
			}
		})
	}
}
func TestResourceManagementService_createRoleIfNeeded(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	tests := []struct {
		name          string
		wsa           *v1beta1.WorkloadServiceAccount
		existingRoles []rbacv1.Role
		expectRole    bool
		expectError   bool
	}{
		{
			name: "WSA with no permissions should return empty role",
			wsa: &v1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wsa",
					Namespace: "default",
				},
				Spec: v1beta1.WorkloadServiceAccountSpec{
					Permissions: v1beta1.WorkloadServiceAccountPermissions{},
				},
			},
			existingRoles: []rbacv1.Role{},
			expectRole:    false,
			expectError:   false,
		},
		{
			name: "WSA with permissions should create new role",
			wsa: &v1beta1.WorkloadServiceAccount{
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
			},
			existingRoles: []rbacv1.Role{},
			expectRole:    true,
			expectError:   false,
		},
		{
			name: "existing role should be returned",
			wsa: &v1beta1.WorkloadServiceAccount{
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
			},
			existingRoles: []rbacv1.Role{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "octopus-role-" + shortHash(`[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]`),
						Namespace: "default",
						Labels: map[string]string{
							PermissionsKey: "enabled",
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
			},
			expectRole:  true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for i := range tt.existingRoles {
				objects = append(objects, &tt.existingRoles[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			service := NewResourceManagementService(fakeClient)
			role, err := service.createRoleIfNeeded(context.Background(), tt.wsa)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.expectRole {
				assert.NotEmpty(t, role.Name)
				assert.Contains(t, role.Name, "octopus-role-")
				assert.Equal(t, tt.wsa.Namespace, role.Namespace)
				assert.Equal(t, tt.wsa.Spec.Permissions.Permissions, role.Rules)
				assert.Equal(t, "enabled", role.Labels[PermissionsKey])
			} else {
				assert.Empty(t, role.Name)
			}
		})
	}
}

func TestResourceManagementService_createServiceAccount(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name                    string
		namespace               string
		templateServiceAccount  *corev1.ServiceAccount
		existingServiceAccounts []corev1.ServiceAccount
		expectCreation          bool
		expectError             bool
	}{
		{
			name:      "new service account should be created",
			namespace: "test-namespace",
			templateServiceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sa",
					Labels: map[string]string{
						"app": "test",
					},
				},
			},
			existingServiceAccounts: []corev1.ServiceAccount{},
			expectCreation:          true,
			expectError:             false,
		},
		{
			name:      "existing service account should not be created again",
			namespace: "test-namespace",
			templateServiceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sa",
				},
			},
			existingServiceAccounts: []corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sa",
						Namespace: "test-namespace",
					},
				},
			},
			expectCreation: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for i := range tt.existingServiceAccounts {
				objects = append(objects, &tt.existingServiceAccounts[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			service := NewResourceManagementService(fakeClient)
			err := service.createServiceAccount(context.Background(), tt.namespace, tt.templateServiceAccount)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			createdSA := &corev1.ServiceAccount{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{
				Name:      tt.templateServiceAccount.Name,
				Namespace: tt.namespace,
			}, createdSA)
			assert.NoError(t, err)
			assert.Equal(t, tt.namespace, createdSA.Namespace)
			assert.Equal(t, tt.templateServiceAccount.Name, createdSA.Name)

			for key, value := range tt.templateServiceAccount.Labels {
				assert.Equal(t, value, createdSA.Labels[key])
			}
		})
	}
}

func TestResourceManagementService_createRoleBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	tests := []struct {
		name                 string
		wsa                  *v1beta1.WorkloadServiceAccount
		roleRef              rbacv1.RoleRef
		subjects             []rbacv1.Subject
		existingRoleBindings []rbacv1.RoleBinding
		expectCreation       bool
		expectError          bool
	}{
		{
			name: "new role binding should be created",
			wsa: &v1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wsa",
					Namespace: "default",
				},
			},
			roleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     "test-role",
				APIGroup: "rbac.authorization.k8s.io",
			},
			subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "test-sa",
					Namespace: "default",
				},
			},
			existingRoleBindings: []rbacv1.RoleBinding{},
			expectCreation:       true,
			expectError:          false,
		},
		{
			name: "existing role binding should not be created again",
			wsa: &v1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wsa",
					Namespace: "default",
				},
			},
			roleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     "test-role",
				APIGroup: "rbac.authorization.k8s.io",
			},
			subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "test-sa",
					Namespace: "default",
				},
			},
			existingRoleBindings: []rbacv1.RoleBinding{
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
			},
			expectCreation: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for i := range tt.existingRoleBindings {
				objects = append(objects, &tt.existingRoleBindings[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			initialList := &rbacv1.RoleBindingList{}
			err := fakeClient.List(context.Background(), initialList)
			require.NoError(t, err)
			initialCount := len(initialList.Items)

			service := NewResourceManagementService(fakeClient)
			err = service.createRoleBinding(context.Background(), tt.wsa, tt.roleRef, tt.subjects)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			finalList := &rbacv1.RoleBindingList{}
			err = fakeClient.List(context.Background(), finalList)
			require.NoError(t, err)
			finalCount := len(finalList.Items)

			if tt.expectCreation {
				assert.Equal(t, initialCount+1, finalCount, "Role binding should have been created")

				var createdRB *rbacv1.RoleBinding
				for _, rb := range finalList.Items {
					found := false
					for _, existing := range tt.existingRoleBindings {
						if existing.Name == rb.Name && existing.Namespace == rb.Namespace {
							found = true
							break
						}
					}
					if !found {
						createdRB = &rb
						break
					}
				}

				require.NotNil(t, createdRB, "Should find the created role binding")
				assert.Contains(t, createdRB.Name, "octopus-rb-")
				assert.Equal(t, tt.wsa.Namespace, createdRB.Namespace)
				assert.Equal(t, tt.roleRef, createdRB.RoleRef)
				assert.Equal(t, tt.subjects, createdRB.Subjects)
			} else {
				assert.Equal(t, initialCount, finalCount, "No new role binding should have been created")
			}
		})
	}
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int // expected length
	}{
		{
			name:     "empty string should return 32 character hash",
			input:    "",
			expected: 32,
		},
		{
			name:     "simple string should return 32 character hash",
			input:    "test",
			expected: 32,
		},
		{
			name:     "complex string should return 32 character hash",
			input:    `[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"]}]`,
			expected: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shortHash(tt.input)
			assert.Len(t, result, tt.expected)

			for _, char := range result {
				assert.True(t, (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'),
					"Hash should only contain hex characters")
			}

			result2 := shortHash(tt.input)
			assert.Equal(t, result, result2, "Same input should produce same hash")
		})
	}
}
