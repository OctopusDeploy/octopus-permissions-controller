package helpers

import (
	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateTestWSA creates a WorkloadServiceAccount for testing with the given scope and permissions
func CreateTestWSA(
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

// CreateTestWSAWithRoles creates a WorkloadServiceAccount with inline permissions and role references
func CreateTestWSAWithRoles(
	name, namespace string, scopes map[string][]string, permissions []rbacv1.PolicyRule,
	roles []rbacv1.RoleRef, clusterRoles []rbacv1.RoleRef,
) *agentoctopuscomv1beta1.WorkloadServiceAccount {
	wsa := CreateTestWSA(name, namespace, scopes, permissions)
	wsa.Spec.Permissions.Roles = roles
	wsa.Spec.Permissions.ClusterRoles = clusterRoles
	return wsa
}

// CreateTestCWSA creates a ClusterWorkloadServiceAccount for testing
func CreateTestCWSA(
	name string, scopes map[string][]string, permissions []rbacv1.PolicyRule,
) *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount {
	cwsa := &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountSpec{
			Scope: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountScope{},
			Permissions: agentoctopuscomv1beta1.ClusterWorkloadServiceAccountPermissions{
				Permissions: permissions,
			},
		},
	}

	// Set scopes
	if projects, ok := scopes["projects"]; ok {
		cwsa.Spec.Scope.Projects = projects
	}
	if environments, ok := scopes["environments"]; ok {
		cwsa.Spec.Scope.Environments = environments
	}
	if tenants, ok := scopes["tenants"]; ok {
		cwsa.Spec.Scope.Tenants = tenants
	}
	if steps, ok := scopes["steps"]; ok {
		cwsa.Spec.Scope.Steps = steps
	}
	if spaces, ok := scopes["spaces"]; ok {
		cwsa.Spec.Scope.Spaces = spaces
	}

	return cwsa
}

// CreateTestCWSAWithRoles creates a ClusterWorkloadServiceAccount with inline permissions and role references
func CreateTestCWSAWithRoles(
	name string, scopes map[string][]string, permissions []rbacv1.PolicyRule,
	clusterRoles []rbacv1.RoleRef,
) *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount {
	cwsa := CreateTestCWSA(name, scopes, permissions)
	cwsa.Spec.Permissions.ClusterRoles = clusterRoles
	return cwsa
}

// FilterRoleBindingsByPrefix filters role bindings by name prefix
func FilterRoleBindingsByPrefix(roleBindings []rbacv1.RoleBinding, prefix string) []rbacv1.RoleBinding {
	var filtered []rbacv1.RoleBinding
	for _, rb := range roleBindings {
		if len(rb.Name) >= len(prefix) && rb.Name[:len(prefix)] == prefix {
			filtered = append(filtered, rb)
		}
	}
	return filtered
}

// FilterClusterRoleBindingsByPrefix filters cluster role bindings by name prefix
func FilterClusterRoleBindingsByPrefix(
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
