package rules

import (
	"testing"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FindAllPossibleResourceScopesTest struct {
	Name       string
	Vocabulary GlobalVocabulary
	WsaList    []*v1beta1.WorkloadServiceAccount
	Want       map[Scope]map[string]v1beta1.WorkloadServiceAccount
}

func NewBasicTest(name string) FindAllPossibleResourceScopesTest {
	test := FindAllPossibleResourceScopesTest{
		Name:       name,
		Vocabulary: GlobalVocabulary{},
		WsaList:    nil,
		Want:       nil,
	}

	// Create a vocabulary
	vocab := NewGlobalVocabulary()
	vocab[ProjectIndex].InsertSlice([]string{"project1", "project2"})
	vocab[EnvironmentIndex].InsertSlice([]string{"env1", "env2"})
	vocab[TenantIndex].InsertSlice([]string{"tenant1", "tenant2"})
	vocab[StepIndex].InsertSlice([]string{"step1", "step2"})
	vocab[SpaceIndex].InsertSlice([]string{"space1", "space2"})
	test.Vocabulary = vocab

	// Generate the role list
	wsaList := []*v1beta1.WorkloadServiceAccount{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
			Spec: v1beta1.WorkloadServiceAccountSpec{
				Scope: v1beta1.WorkloadServiceAccountScope{
					Environments: []string{"env1", "env2"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
			Spec: v1beta1.WorkloadServiceAccountSpec{
				Scope: v1beta1.WorkloadServiceAccountScope{
					Projects: []string{"proj2"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wsa3"},
			Spec: v1beta1.WorkloadServiceAccountSpec{
				Scope: v1beta1.WorkloadServiceAccountScope{
					Spaces: []string{"space1"},
				},
			},
		},
	}
	test.WsaList = wsaList

	return test
}

func Test_getScopesForWSAs(t *testing.T) {
	tests := []FindAllPossibleResourceScopesTest{
		NewBasicTest("test 1"),
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			result := getScopesForWSAs(tt.WsaList)
			// Verify we get some results
			assert.NotEmpty(t, result, "getScopesForWSAs should return non-empty result")

			// Verify each scope has at least one WSA mapped to it
			for scope, wsaMap := range result {
				assert.NotEmpty(t, wsaMap, "Each scope should have at least one WSA: %v", scope)
			}
		})
	}
}

func Test_GenerateServiceAccountMappings(t *testing.T) {
	tests := []struct {
		name                string
		scopeMap            map[Scope]map[string]*v1beta1.WorkloadServiceAccount
		wantScopes          int
		wantServiceAccounts int
	}{
		{
			name: "single scope with one WSA",
			scopeMap: map[Scope]map[string]*v1beta1.WorkloadServiceAccount{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": &v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					},
				},
			},
			wantScopes:          1,
			wantServiceAccounts: 1,
		},
		{
			name: "multiple scopes with overlapping WSAs",
			scopeMap: map[Scope]map[string]*v1beta1.WorkloadServiceAccount{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": &v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					},
					"wsa2": &v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
					},
				},
				{Project: "proj2", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": &v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					},
				},
			},
			wantScopes:          2,
			wantServiceAccounts: 2,
		},
		{
			name: "identical scopes (should create one service account)",
			scopeMap: map[Scope]map[string]*v1beta1.WorkloadServiceAccount{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": &v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					},
				},
			},
			wantScopes:          1,
			wantServiceAccounts: 1,
		},
		{
			name:                "empty scope map",
			scopeMap:            map[Scope]map[string]*v1beta1.WorkloadServiceAccount{},
			wantScopes:          0,
			wantServiceAccounts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scopeToSA, saToWSAs, wsaToSANames, serviceAccounts := GenerateServiceAccountMappings(tt.scopeMap)

			// Verify the number of scopes matches
			assert.Equal(t, tt.wantScopes, len(scopeToSA), "Number of scopes should match")

			// Verify the number of unique service accounts matches
			assert.Equal(t, tt.wantServiceAccounts, len(serviceAccounts), "Number of service accounts should match")

			// Verify consistency between maps
			for scope, sa := range scopeToSA {
				// Check service account exists in the slice
				found := false
				for _, serviceAccount := range serviceAccounts {
					if serviceAccount.Name == string(sa) {
						found = true
						break
					}
				}
				assert.True(t, found, "Service account from scope map should exist in service accounts slice")

				wsaMap, exists := saToWSAs[sa]
				assert.True(t, exists, "Service account should have WSA mapping")
				assert.NotEmpty(t, wsaMap, "Service account should have at least one WSA")

				// Verify WSAs in the mapping exist in the original scope
				originalWSAs := tt.scopeMap[scope]
				for wsaName, wsa := range wsaMap {
					originalWSA, wsaExists := originalWSAs[wsaName]
					assert.True(t, wsaExists, "WSA %s should exist in original scope %v", wsaName, scope)
					assert.Equal(t, originalWSA, wsa, "WSA pointer should match original")
				}

				// Verify reverse mapping (WSA to service account names)
				for wsaName := range wsaMap {
					saNames, exists := wsaToSANames[wsaName]
					assert.True(t, exists, "WSA %s should have service account mapping", wsaName)
					assert.Contains(t, saNames, string(sa), "WSA should map back to service account")
				}
			}

			// Verify service account names follow the expected pattern
			for _, sa := range serviceAccounts {
				assert.Contains(t, sa.Name, "octopus-sa-", "Service account name should follow the expected pattern")
				assert.True(t, len(sa.Name) > len("octopus-sa-"), "Service account name should have a hash suffix")

				// Verify service account has proper metadata
				assert.NotNil(t, sa.Labels, "Service account should have labels")
				assert.NotNil(t, sa.Annotations, "Service account should have annotations")
			}
		})
	}
}

func Test_generateServiceAccountName(t *testing.T) {
	tests := []struct {
		name       string
		scope      Scope
		wantPrefix string
	}{
		{
			name: "basic scope",
			scope: Scope{
				Project:     "proj1",
				Environment: "env1",
				Tenant:      "*",
				Step:        "*",
				Space:       "*",
			},
			wantPrefix: "octopus-sa-",
		},
		{
			name:       "empty scope",
			scope:      Scope{},
			wantPrefix: "octopus-sa-",
		},
		{
			name: "complex scope",
			scope: Scope{
				Project:     "my-project",
				Environment: "production",
				Tenant:      "customer-a",
				Step:        "deploy-app",
				Space:       "main-space",
			},
			wantPrefix: "octopus-sa-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateServiceAccountName(tt.scope)

			// Check that it starts with the expected prefix
			assert.True(t, len(string(result)) > len(tt.wantPrefix), "Service account name should be longer than just the prefix")
			assert.Contains(t, string(result), tt.wantPrefix, "Service account name should contain the expected prefix")

			// Check that the same scope generates the same name (deterministic)
			result2 := generateServiceAccountName(tt.scope)
			assert.Equal(t, result, result2, "Same scope should generate same service account name")

			// Check that different scopes generate different names
			differentScope := tt.scope
			differentScope.Project = tt.scope.Project + "-different"
			differentResult := generateServiceAccountName(differentScope)
			assert.NotEqual(t, result, differentResult, "Different scopes should generate different service account names")
		})
	}
}
