package rules

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

func TestGenerateAllScopesWithPermissions_EmptyList_ReturnsNil(t *testing.T) {
	var wsas []v1beta1.WorkloadServiceAccount

	result := GenerateAllScopesWithPermissions(wsas)

	if result != nil {
		t.Errorf("Expected nil result for empty WSA list, got %v", result)
	}
}

func TestGenerateAllScopesWithPermissions_SingleWSA_GeneratesSingleScope(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("test-wsa-1",
			[]string{"web-app"},
			[]string{"development"},
			nil, nil,
			"", "pods", []string{"get", "list"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	expectedScopes := []string{
		"projects=web-app,environments=development,tenants=*,steps=*",
	}

	assertScopesMatch(t, result, expectedScopes)
}

func TestGenerateAllScopesWithPermissions_SingleWSA_GeneratesMultipleScopes(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("test-wsa-1",
			[]string{"web-app"},
			[]string{"development", "test"},
			nil, nil,
			"", "pods", []string{"get", "list"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	expectedScopes := []string{
		"projects=web-app,environments=development,tenants=*,steps=*",
		"projects=web-app,environments=test,tenants=*,steps=*",
	}

	assertScopesMatch(t, result, expectedScopes)
}

func TestGenerateAllScopesWithPermissions_DifferentOrder_GeneratesSameScopes(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("dev-prod-wsa",
			nil,
			[]string{"development", "production"},
			nil, nil,
			"", "pods", []string{"get", "list"}),
		createWSA("prod-dev-wsa",
			nil,
			[]string{"production", "development"},
			nil, nil,
			"apps", "deployments", []string{"get", "watch"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	expectedScopes := []string{
		"projects=*,environments=development,tenants=*,steps=*",
		"projects=*,environments=production,tenants=*,steps=*",
	}

	assertScopesMatch(t, result, expectedScopes)
}

func TestGenerateAllScopesWithPermissions_MultipleWSAs_GeneratesScopes(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("dev-wsa",
			nil,
			[]string{"development"},
			nil, nil,
			"", "configmaps", []string{"get", "list"}),
		createWSA("prod-wsa",
			nil,
			[]string{"production"},
			nil, nil,
			"apps", "deployments", []string{"get", "watch"}),
		createWSA("project-wsa",
			[]string{"api-service"},
			nil,
			nil, nil,
			"", "secrets", []string{"get"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	expectedScopes := []string{
		"projects=*,environments=development,tenants=*,steps=*",
		"projects=*,environments=production,tenants=*,steps=*",
		"projects=api-service,environments=*,tenants=*,steps=*",
		"projects=api-service,environments=development,tenants=*,steps=*",
		"projects=api-service,environments=production,tenants=*,steps=*",
	}

	assertScopesMatch(t, result, expectedScopes)
}

func TestGenerateAllScopesWithPermissions_WithOverlappingEnvironments_GeneratesScopes(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("dev-test-wsa",
			nil,
			[]string{"dev", "test"},
			nil, nil,
			"", "configmaps", []string{"get", "list"}),
		createWSA("test-stg-wsa",
			nil,
			[]string{"test", "stg"},
			nil, nil,
			"apps", "deployments", []string{"get", "watch"}),
		createWSA("stg-prod-wsa",
			nil,
			[]string{"stg", "prod"},
			nil, nil,
			"", "secrets", []string{"get"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	expectedScopes := []string{
		"projects=*,environments=dev,tenants=*,steps=*",
		"projects=*,environments=test,tenants=*,steps=*",
		"projects=*,environments=stg,tenants=*,steps=*",
		"projects=*,environments=prod,tenants=*,steps=*",
	}

	assertScopesMatch(t, result, expectedScopes)
}

func TestGenerateAllScopesWithPermissions_NoWildcards(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("web-dev-wsa",
			[]string{"web-app"},
			[]string{"development"},
			[]string{"tenant-a"},
			[]string{"deploy"},
			"", "configmaps", []string{"get", "list"}),
		createWSA("api-prod-wsa",
			[]string{"api-service"},
			[]string{"production"},
			[]string{"tenant-b"},
			[]string{"deploy"},
			"apps", "deployments", []string{"get", "watch"}),
		createWSA("monitoring-wsa",
			[]string{"web-app"},
			[]string{"development"},
			[]string{"tenant-a"},
			[]string{"deploy"},
			"", "pods", []string{"get", "list", "watch"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	expectedScopes := []string{
		"projects=web-app,environments=development,tenants=tenant-a,steps=deploy",
		"projects=api-service,environments=production,tenants=tenant-b,steps=deploy",
	}

	assertScopesMatch(t, result, expectedScopes)
}

func TestPermissionMerging_OverlappingEnvironments(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("dev-test-wsa",
			nil,
			[]string{"dev", "test"},
			nil, nil,
			"", "configmaps", []string{"get", "list"}),
		createWSA("test-stg-wsa",
			nil,
			[]string{"test", "stg"},
			nil, nil,
			"apps", "deployments", []string{"get", "watch"}),
		createWSA("stg-prod-wsa",
			nil,
			[]string{"stg", "prod"},
			nil, nil,
			"", "secrets", []string{"get"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	assertScopeHasPermissions(t, result, "*", "test", "*", "*",
		[]rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "watch"}},
		})

	assertScopeHasPermissions(t, result, "*", "stg", "*", "*",
		[]rbacv1.PolicyRule{
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "watch"}},
			{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
		})
}

func TestPermissionMerging_IntersectingScopes(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("web-dev-wsa",
			[]string{"web-app"},
			[]string{"development"},
			[]string{"tenant-a"},
			[]string{"deploy"},
			"", "configmaps", []string{"get", "list"}),
		createWSA("monitoring-wsa",
			[]string{"web-app"},
			[]string{"development"},
			[]string{"tenant-a"},
			[]string{"deploy"},
			"", "pods", []string{"get", "list", "watch"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	assertScopeHasPermissions(t, result, "web-app", "development", "tenant-a", "deploy",
		[]rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
		})
}

func TestPermissionMerging_NoOverlap(t *testing.T) {
	wsas := []v1beta1.WorkloadServiceAccount{
		createWSA("web-dev-wsa",
			[]string{"web-app"},
			[]string{"development"},
			[]string{"tenant-a"},
			[]string{"deploy"},
			"", "configmaps", []string{"get", "list"}),
		createWSA("api-prod-wsa",
			[]string{"api-service"},
			[]string{"production"},
			[]string{"tenant-b"},
			[]string{"deploy"},
			"apps", "deployments", []string{"get", "watch"}),
	}

	result := GenerateAllScopesWithPermissions(wsas)

	assertScopeHasPermissions(t, result, "web-app", "development", "tenant-a", "deploy",
		[]rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
		})

	assertScopeHasPermissions(t, result, "api-service", "production", "tenant-b", "deploy",
		[]rbacv1.PolicyRule{
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "watch"}},
		})
}

func createWSA(name string, projects, environments, tenants, steps []string, apiGroup, resource string, verbs []string) v1beta1.WorkloadServiceAccount {
	return v1beta1.WorkloadServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1beta1.WorkloadServiceAccountSpec{
			Scope: v1beta1.WorkloadServiceAccountScope{
				Projects:     projects,
				Environments: environments,
				Tenants:      tenants,
				Steps:        steps,
			},
			Permissions: v1beta1.WorkloadServiceAccountPermissions{
				Permissions: []rbacv1.PolicyRule{
					{
						APIGroups: []string{apiGroup},
						Resources: []string{resource},
						Verbs:     verbs,
					},
				},
			},
		},
	}
}

func assertScopesMatch(t *testing.T, result map[Scope]v1beta1.WorkloadServiceAccountPermissions, expectedScopes []string) {
	t.Helper()

	scopeStrings := make([]string, 0, len(result))
	for key := range result {
		scopeStrings = append(scopeStrings, key.String())
	}

	slices.Sort(scopeStrings)
	slices.Sort(expectedScopes)

	if diff := cmp.Diff(expectedScopes, scopeStrings); diff != "" {
		t.Errorf("Generated scopes mismatch (-expected +actual):\n%s", diff)
	}
}

func assertScopeHasPermissions(t *testing.T, result map[Scope]v1beta1.WorkloadServiceAccountPermissions, projects, environments, tenants, steps string, expectedPermissions []rbacv1.PolicyRule) {
	t.Helper()

	scope := Scope{
		Project:     projects,
		Environment: environments,
		Tenant:      tenants,
		Step:        steps,
	}

	actualPermissions, exists := result[scope]
	if !exists {
		t.Errorf("Expected scope %s to exist in results", scope.String())
		return
	}

	if diff := cmp.Diff(expectedPermissions, actualPermissions.Permissions); diff != "" {
		t.Errorf("Permissions mismatch for scope %s (-expected +actual):\n%s", scope.String(), diff)
	}
}
