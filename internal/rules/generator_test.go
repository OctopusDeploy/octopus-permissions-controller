package rules

import (
	"reflect"
	"testing"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

//func TestGenerateAllScopesWithPermissions_EmptyList_ReturnsNil(t *testing.T) {
//	var wsas []v1beta1.WorkloadServiceAccount
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	if result == nil {
//		t.Error("Expected non-nil empty map for empty WSA list, got nil")
//	}
//	if len(result) != 0 {
//		t.Errorf("Expected empty map for empty WSA list, got %v", result)
//	}
//}
//
//func TestGenerateAllScopesWithPermissions_SingleWSA_GeneratesSingleScope(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("test-wsa-1",
//			[]string{"web-app"},
//			[]string{"development"},
//			nil, nil, nil,
//			"", "pods", []string{"get", "list"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=web-app,environments=development,tenants=*,steps=*,spaces=*",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_SingleWSA_GeneratesMultipleScopes(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("test-wsa-1",
//			[]string{"web-app"},
//			[]string{"development", "test"},
//			nil, nil, nil,
//			"", "pods", []string{"get", "list"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=web-app,environments=development,tenants=*,steps=*,spaces=*",
//		"projects=web-app,environments=test,tenants=*,steps=*,spaces=*",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_DifferentOrder_GeneratesSameScopes(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("dev-prod-wsa",
//			nil,
//			[]string{"development", "production"},
//			nil, nil, nil,
//			"", "pods", []string{"get", "list"}),
//		createWSA("prod-dev-wsa",
//			nil,
//			[]string{"production", "development"},
//			nil, nil, nil,
//			"apps", "deployments", []string{"get", "watch"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=*,environments=development,tenants=*,steps=*,spaces=*",
//		"projects=*,environments=production,tenants=*,steps=*,spaces=*",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_MultipleWSAs_GeneratesScopes(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("dev-wsa",
//			nil,
//			[]string{"development"},
//			nil, nil, nil,
//			"", "configmaps", []string{"get", "list"}),
//		createWSA("prod-wsa",
//			nil,
//			[]string{"production"},
//			nil, nil, nil,
//			"apps", "deployments", []string{"get", "watch"}),
//		createWSA("project-wsa",
//			[]string{"api-service"},
//			nil,
//			nil, nil, nil,
//			"", "secrets", []string{"get"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=*,environments=development,tenants=*,steps=*,spaces=*",
//		"projects=*,environments=production,tenants=*,steps=*,spaces=*",
//		"projects=api-service,environments=*,tenants=*,steps=*,spaces=*",
//		"projects=api-service,environments=development,tenants=*,steps=*,spaces=*",
//		"projects=api-service,environments=production,tenants=*,steps=*,spaces=*",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_WithOverlappingEnvironments_GeneratesScopes(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("dev-test-wsa",
//			nil,
//			[]string{"dev", "test"},
//			nil, nil, nil,
//			"", "configmaps", []string{"get", "list"}),
//		createWSA("test-stg-wsa",
//			nil,
//			[]string{"test", "stg"},
//			nil, nil, nil,
//			"apps", "deployments", []string{"get", "watch"}),
//		createWSA("stg-prod-wsa",
//			nil,
//			[]string{"stg", "prod"},
//			nil, nil, nil,
//			"", "secrets", []string{"get"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=*,environments=dev,tenants=*,steps=*,spaces=*",
//		"projects=*,environments=test,tenants=*,steps=*,spaces=*",
//		"projects=*,environments=stg,tenants=*,steps=*,spaces=*",
//		"projects=*,environments=prod,tenants=*,steps=*,spaces=*",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_NoWildcards(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("web-dev-wsa",
//			[]string{"web-app"},
//			[]string{"development"},
//			[]string{"tenant-a"},
//			[]string{"deploy"},
//			nil,
//			"", "configmaps", []string{"get", "list"}),
//		createWSA("api-prod-wsa",
//			[]string{"api-service"},
//			[]string{"production"},
//			[]string{"tenant-b"},
//			[]string{"deploy"},
//			nil,
//			"apps", "deployments", []string{"get", "watch"}),
//		createWSA("monitoring-wsa",
//			[]string{"web-app"},
//			[]string{"development"},
//			[]string{"tenant-a"},
//			[]string{"deploy"},
//			nil,
//			"", "pods", []string{"get", "list", "watch"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=web-app,environments=development,tenants=tenant-a,steps=deploy,spaces=*",
//		"projects=api-service,environments=production,tenants=tenant-b,steps=deploy,spaces=*",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestPermissionMerging_OverlappingEnvironments(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("dev-test-wsa",
//			nil,
//			[]string{"dev", "test"},
//			nil, nil, nil,
//			"", "configmaps", []string{"get", "list"}),
//		createWSA("test-stg-wsa",
//			nil,
//			[]string{"test", "stg"},
//			nil, nil, nil,
//			"apps", "deployments", []string{"get", "watch"}),
//		createWSA("stg-prod-wsa",
//			nil,
//			[]string{"stg", "prod"},
//			nil, nil, nil,
//			"", "secrets", []string{"get"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	assertScopeHasPermissions(t, result, "*", "test", "*", "*", "*",
//		[]rbacv1.PolicyRule{
//			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
//			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "watch"}},
//		})
//
//	assertScopeHasPermissions(t, result, "*", "stg", "*", "*", "*",
//		[]rbacv1.PolicyRule{
//			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "watch"}},
//			{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
//		})
//}
//
//func TestPermissionMerging_IntersectingScopes(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("web-dev-wsa",
//			[]string{"web-app"},
//			[]string{"development"},
//			[]string{"tenant-a"},
//			[]string{"deploy"},
//			nil,
//			"", "configmaps", []string{"get", "list"}),
//		createWSA("monitoring-wsa",
//			[]string{"web-app"},
//			[]string{"development"},
//			[]string{"tenant-a"},
//			[]string{"deploy"},
//			nil,
//			"", "pods", []string{"get", "list", "watch"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	assertScopeHasPermissions(t, result, "web-app", "development", "tenant-a", "deploy", "*",
//		[]rbacv1.PolicyRule{
//			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
//			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
//		})
//}
//
//func TestPermissionMerging_NoOverlap(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("web-dev-wsa",
//			[]string{"web-app"},
//			[]string{"development"},
//			[]string{"tenant-a"},
//			[]string{"deploy"},
//			nil,
//			"", "configmaps", []string{"get", "list"}),
//		createWSA("api-prod-wsa",
//			[]string{"api-service"},
//			[]string{"production"},
//			[]string{"tenant-b"},
//			[]string{"deploy"},
//			nil,
//			"apps", "deployments", []string{"get", "watch"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	assertScopeHasPermissions(t, result, "web-app", "development", "tenant-a", "deploy", "*",
//		[]rbacv1.PolicyRule{
//			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
//		})
//
//	assertScopeHasPermissions(t, result, "api-service", "production", "tenant-b", "deploy", "*",
//		[]rbacv1.PolicyRule{
//			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "watch"}},
//		})
//}
//
//func createWSA(
//	name string, projects, environments, tenants, steps, spaces []string, apiGroup, resource string, verbs []string,
//) v1beta1.WorkloadServiceAccount {
//	return v1beta1.WorkloadServiceAccount{
//		ObjectMeta: metav1.ObjectMeta{Name: name},
//		Spec: v1beta1.WorkloadServiceAccountSpec{
//			Scope: v1beta1.WorkloadServiceAccountScope{
//				Projects:     projects,
//				Environments: environments,
//				Tenants:      tenants,
//				Steps:        steps,
//				Spaces:       spaces,
//			},
//			Permissions: v1beta1.WorkloadServiceAccountPermissions{
//				Permissions: []rbacv1.PolicyRule{
//					{
//						APIGroups: []string{apiGroup},
//						Resources: []string{resource},
//						Verbs:     verbs,
//					},
//				},
//			},
//		},
//	}
//}
//
//func assertScopesMatch(
//	t *testing.T, result map[Scope]v1beta1.WorkloadServiceAccountPermissions, expectedScopes []string,
//) {
//	t.Helper()
//
//	scopeStrings := make([]string, 0, len(result))
//	for key := range result {
//		scopeStrings = append(scopeStrings, key.String())
//	}
//
//	slices.Sort(scopeStrings)
//	slices.Sort(expectedScopes)
//
//	if diff := cmp.Diff(expectedScopes, scopeStrings); diff != "" {
//		t.Errorf("Generated scopes mismatch (-expected +actual):\n%s", diff)
//	}
//}
//
//func assertScopeHasPermissions(
//	t *testing.T, result map[Scope]v1beta1.WorkloadServiceAccountPermissions,
//	projects, environments, tenants, steps, spaces string, expectedPermissions []rbacv1.PolicyRule,
//) {
//	t.Helper()
//
//	scope := Scope{
//		Project:     projects,
//		Environment: environments,
//		Tenant:      tenants,
//		Step:        steps,
//		Space:       spaces,
//	}
//
//	actualPermissions, exists := result[scope]
//	if !exists {
//		t.Errorf("Expected scope %s to exist in results", scope.String())
//		return
//	}
//
//	if diff := cmp.Diff(expectedPermissions, actualPermissions.Permissions); diff != "" {
//		t.Errorf("Permissions mismatch for scope %s (-expected +actual):\n%s", scope.String(), diff)
//	}
//}
//
//// Space-specific test cases
//
//func TestGenerateAllScopesWithPermissions_WithSpaces_GeneratesSpaceScopes(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("space1-wsa",
//			nil, nil, nil, nil,
//			[]string{"space-1"},
//			"", "pods", []string{"get", "list"}),
//		createWSA("space2-wsa",
//			nil, nil, nil, nil,
//			[]string{"space-2"},
//			"apps", "deployments", []string{"get", "watch"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=*,environments=*,tenants=*,steps=*,spaces=space-1",
//		"projects=*,environments=*,tenants=*,steps=*,spaces=space-2",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_WithMultipleSpaces_GeneratesAllCombinations(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("multi-space-wsa",
//			[]string{"project-a"},
//			[]string{"dev"},
//			nil, nil,
//			[]string{"space-1", "space-2"},
//			"", "configmaps", []string{"get", "list"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=project-a,environments=dev,tenants=*,steps=*,spaces=space-1",
//		"projects=project-a,environments=dev,tenants=*,steps=*,spaces=space-2",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_SpacePermissionMerging(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("space1-configmaps-wsa",
//			nil, nil, nil, nil,
//			[]string{"shared-space"},
//			"", "configmaps", []string{"get", "list"}),
//		createWSA("space1-secrets-wsa",
//			nil, nil, nil, nil,
//			[]string{"shared-space"},
//			"", "secrets", []string{"get"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	assertScopeHasPermissions(t, result, "*", "*", "*", "*", "shared-space",
//		[]rbacv1.PolicyRule{
//			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
//			{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
//		})
//}
//
//func TestGenerateAllScopesWithPermissions_MixedSpacesAndOtherDimensions(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("mixed-wsa",
//			[]string{"web-app"},
//			[]string{"production"},
//			[]string{"tenant-x"},
//			nil,
//			[]string{"space-prod"},
//			"", "pods", []string{"get", "list"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=web-app,environments=production,tenants=tenant-x,steps=*,spaces=space-prod",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}
//
//func TestGenerateAllScopesWithPermissions_EmptySpaces_CreatesWildcard(t *testing.T) {
//	wsas := []v1beta1.WorkloadServiceAccount{
//		createWSA("no-spaces-wsa",
//			[]string{"project-a"},
//			nil, nil, nil, nil,
//			"", "pods", []string{"get"}),
//	}
//
//	result := generateAllScopesWithPermissions(wsas)
//
//	expectedScopes := []string{
//		"projects=project-a,environments=*,tenants=*,steps=*,spaces=*",
//	}
//
//	assertScopesMatch(t, result, expectedScopes)
//}

func Test_getAllScopeValues(t *testing.T) {
	type args struct {
		wsas []*v1beta1.WorkloadServiceAccount
	}
	testWsa := &v1beta1.WorkloadServiceAccount{
		Spec: v1beta1.WorkloadServiceAccountSpec{
			Scope: v1beta1.WorkloadServiceAccountScope{
				Projects:     []string{"project1", "project2"},
				Environments: []string{"env1", "env2"},
				Tenants:      []string{"tenant1", "tenant2"},
				Steps:        []string{"step1", "step2"},
				Spaces:       []string{"space1", "space2"},
			},
		},
	}
	testWsaWithWildcards := &v1beta1.WorkloadServiceAccount{
		Spec: v1beta1.WorkloadServiceAccountSpec{
			Scope: v1beta1.WorkloadServiceAccountScope{
				Projects:     []string{"project1"},
				Environments: []string{},
				Tenants:      []string{},
				Steps:        []string{},
				Spaces:       []string{},
			},
		},
	}
	tests := []struct {
		name             string
		args             args
		wantProjects     map[string][]*v1beta1.WorkloadServiceAccount
		wantEnvironments map[string][]*v1beta1.WorkloadServiceAccount
		wantTenants      map[string][]*v1beta1.WorkloadServiceAccount
		wantSteps        map[string][]*v1beta1.WorkloadServiceAccount
		wantSpaces       map[string][]*v1beta1.WorkloadServiceAccount
	}{
		{
			name: "Single WSA with all scope fields",
			args: args{
				wsas: []*v1beta1.WorkloadServiceAccount{testWsa},
			},
			wantProjects: map[string][]*v1beta1.WorkloadServiceAccount{
				"project1": {testWsa},
				"project2": {testWsa},
			},
			wantEnvironments: map[string][]*v1beta1.WorkloadServiceAccount{
				"env1": {testWsa},
				"env2": {testWsa},
			},
			wantTenants: map[string][]*v1beta1.WorkloadServiceAccount{
				"tenant1": {testWsa},
				"tenant2": {testWsa},
			},
			wantSteps: map[string][]*v1beta1.WorkloadServiceAccount{
				"step1": {testWsa},
				"step2": {testWsa},
			},
			wantSpaces: map[string][]*v1beta1.WorkloadServiceAccount{
				"space1": {testWsa},
				"space2": {testWsa},
			},
		},
		{
			name: "Empty WSA as well as scoped WSA",
			args: args{
				wsas: []*v1beta1.WorkloadServiceAccount{testWsa, testWsaWithWildcards},
			},
			wantProjects: map[string][]*v1beta1.WorkloadServiceAccount{
				"project1": {testWsa, testWsaWithWildcards},
				"project2": {testWsa},
			},
			wantEnvironments: map[string][]*v1beta1.WorkloadServiceAccount{
				"*":    {testWsaWithWildcards},
				"env1": {testWsa},
				"env2": {testWsa},
			},
			wantTenants: map[string][]*v1beta1.WorkloadServiceAccount{
				"*":       {testWsaWithWildcards},
				"tenant1": {testWsa},
				"tenant2": {testWsa},
			},
			wantSteps: map[string][]*v1beta1.WorkloadServiceAccount{
				"*":     {testWsaWithWildcards},
				"step1": {testWsa},
				"step2": {testWsa},
			},
			wantSpaces: map[string][]*v1beta1.WorkloadServiceAccount{
				"*":      {testWsaWithWildcards},
				"space1": {testWsa},
				"space2": {testWsa},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProjects, gotEnvironments, gotTenants, gotSteps, gotSpaces := getAllScopeValues(tt.args.wsas)
			if !reflect.DeepEqual(gotProjects, tt.wantProjects) {
				t.Errorf("getAllScopeValues() gotProjects = %v, want %v", gotProjects, tt.wantProjects)
			}
			if !reflect.DeepEqual(gotEnvironments, tt.wantEnvironments) {
				t.Errorf("getAllScopeValues() gotEnvironments = %v, want %v", gotEnvironments, tt.wantEnvironments)
			}
			if !reflect.DeepEqual(gotTenants, tt.wantTenants) {
				t.Errorf("getAllScopeValues() gotTenants = %v, want %v", gotTenants, tt.wantTenants)
			}
			if !reflect.DeepEqual(gotSteps, tt.wantSteps) {
				t.Errorf("getAllScopeValues() gotSteps = %v, want %v", gotSteps, tt.wantSteps)
			}
			if !reflect.DeepEqual(gotSpaces, tt.wantSpaces) {
				t.Errorf("getAllScopeValues() gotSpaces = %v, want %v", gotSpaces, tt.wantSpaces)
			}
		})
	}
}
