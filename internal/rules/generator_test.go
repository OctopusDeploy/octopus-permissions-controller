package rules

import (
	"slices"
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

func NewFindAllPossibleResourceScopesTest(name string) FindAllPossibleResourceScopesTest {
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
		NewFindAllPossibleResourceScopesTest("Simple Scopes and WSAs"),
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

func Test_MultipleWSAsWithTheSameScopeOnlyCreateOneServiceAccount(t *testing.T) {
	// This test ensures that even if multiple scopes map to the same set of WSAs,
	// only one service account is created for that set of WSAs.
	scopeMap := map[Scope]map[string]*v1beta1.WorkloadServiceAccount{
		{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
			"wsa1": &v1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
			},
		},
		{Project: "proj2", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
			"wsa1": &v1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
			},
		},
	}
	_, _, wsaToSANames, _ := GenerateServiceAccountMappings(scopeMap)

	assert.Contains(t, wsaToSANames, "wsa1")
	assert.Equal(t, len(wsaToSANames["wsa1"]), 1)
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
		name     string
		wsaNames []string
		want     string
	}{
		{
			name:     "Single WSA",
			wsaNames: []string{"wsa1"},
			want:     "octopus-sa-0e813084b9bcdf5e87ec36a6eeddc15e",
		},
		{
			name:     "Multiple WSAs",
			wsaNames: []string{"wsa1", "wsa2", "wsa3"},
			want:     "octopus-sa-e712752029c0eb8f00ca307935abd62c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateServiceAccountName(slices.Values(tt.wsaNames))

			// Check the SA names are deterministic
			assert.EqualValues(t, tt.want, result, "Service account name should be deterministic")
		})
	}
}
