package rules

import (
	"testing"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScopeComputationService_GetServiceAccountForScope(t *testing.T) {
	vocab := NewGlobalVocabulary()
	vocab[ProjectIndex].Insert("project1")
	vocab[EnvironmentIndex].Insert("env1")
	vocab[TenantIndex].Insert("tenant1")
	vocab[StepIndex].Insert("step1")
	vocab[SpaceIndex].Insert("space1")

	scopeToSA := map[Scope]ServiceAccountName{
		{Project: "project1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: "octopus-sa-test1",
		{Project: "*", Environment: "*", Tenant: "*", Step: "*", Space: "*"}:           "octopus-sa-wildcard",
	}

	tests := []struct {
		name      string
		scope     Scope
		wantSA    ServiceAccountName
		wantError bool
	}{
		{
			name:      "exact match should return service account",
			scope:     Scope{Project: "project1", Environment: "env1", Tenant: "any", Step: "any", Space: "any"},
			wantSA:    "octopus-sa-test1",
			wantError: false,
		},
		{
			name:      "wildcard match should return wildcard service account",
			scope:     Scope{Project: "unknown", Environment: "unknown", Tenant: "unknown", Step: "unknown", Space: "unknown"},
			wantSA:    "octopus-sa-wildcard",
			wantError: false,
		},
		{
			name:      "no match when no wildcard mapping exists should return empty string",
			scope:     Scope{Project: "nonexistent", Environment: "nonexistent", Tenant: "nonexistent", Step: "nonexistent", Space: "nonexistent"},
			wantSA:    "octopus-sa-wildcard",
			wantError: false,
		},
	}

	service := NewScopeComputationService(&vocab, &scopeToSA)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var testService ScopeComputationService
			if tt.name == "no match when no wildcard mapping exists should return empty string" {
				noWildcardScopeToSA := map[Scope]ServiceAccountName{
					{Project: "project1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: "octopus-sa-test1",
				}
				testService = NewScopeComputationService(&vocab, &noWildcardScopeToSA)
			} else {
				testService = service
			}

			sa, err := testService.GetServiceAccountForScope(tt.scope)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.name == "no match when no wildcard mapping exists should return empty string" {
					assert.Equal(t, ServiceAccountName(""), sa)
				} else {
					assert.Equal(t, tt.wantSA, sa)
				}
			}
		})
	}
}

func TestScopeComputationService_ComputeScopesForWSAs(t *testing.T) {
	tests := []struct {
		name               string
		wsaList            []WSAResource
		expectedScopeCount int
		minExpectedScopes  int
	}{
		{
			name:               "empty WSA list should return empty result",
			wsaList:            []WSAResource{},
			expectedScopeCount: 0,
			minExpectedScopes:  0,
		},
		{
			name: "single WSA should generate scopes",
			wsaList: []WSAResource{
				NewWSAResource(&v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects:     []string{"project1"},
							Environments: []string{"env1"},
						},
					},
				}),
			},
			expectedScopeCount: -1,
			minExpectedScopes:  1,
		},
		{
			name: "multiple WSAs with overlapping scopes",
			wsaList: []WSAResource{
				NewWSAResource(&v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects:     []string{"project1"},
							Environments: []string{"env1"},
						},
					},
				}),
				NewWSAResource(&v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects:     []string{"project1"},
							Environments: []string{"env2"},
						},
					},
				}),
			},
			expectedScopeCount: -1,
			minExpectedScopes:  1,
		},
		{
			name: "WSA with empty scope (wildcard)",
			wsaList: []WSAResource{
				NewWSAResource(&v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: "wsa-wildcard"},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{},
					},
				}),
			},
			expectedScopeCount: -1,
			minExpectedScopes:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewScopeComputationService(nil, nil)
			scopeMap, vocabulary := service.ComputeScopesForWSAs(tt.wsaList)

			assert.NotNil(t, vocabulary)

			if tt.expectedScopeCount >= 0 {
				assert.Equal(t, tt.expectedScopeCount, len(scopeMap))
			} else {
				assert.GreaterOrEqual(t, len(scopeMap), tt.minExpectedScopes)
			}

			for scope, wsaMap := range scopeMap {
				assert.NotEmpty(t, wsaMap, "Each scope should have at least one WSA: %v", scope)

				for wsaName, wsa := range wsaMap {
					found := false
					for _, originalWSA := range tt.wsaList {
						if originalWSA.GetName() == wsaName && originalWSA == wsa {
							found = true
							break
						}
					}
					assert.True(t, found, "WSA %s should be from original list", wsaName)
				}
			}

			if len(tt.wsaList) > 0 {
				hasActualScopes := false
				for _, wsa := range tt.wsaList {
					scope := wsa.GetScope()
					if len(scope.Projects) > 0 || len(scope.Environments) > 0 || len(scope.Tenants) > 0 || len(scope.Steps) > 0 || len(scope.Spaces) > 0 {
						hasActualScopes = true
						break
					}
				}

				if hasActualScopes {
					foundValues := false
					for i := ProjectIndex; i < MaxDimensionIndex; i++ {
						if vocabulary[i].Size() > 0 {
							foundValues = true
							break
						}
					}
					assert.True(t, foundValues, "Vocabulary should contain some values when WSAs with actual scopes are provided")
				}
			}
		})
	}
}

func TestScopeComputationService_GenerateServiceAccountMappings(t *testing.T) {
	tests := []struct {
		name                string
		scopeMap            map[Scope]map[string]WSAResource
		wantScopeCount      int
		wantSACount         int
		wantWSAMappingCount int
	}{
		{
			name:                "empty scope map should return empty mappings",
			scopeMap:            map[Scope]map[string]WSAResource{},
			wantScopeCount:      0,
			wantSACount:         0,
			wantWSAMappingCount: 0,
		},
		{
			name: "single scope with one WSA",
			scopeMap: map[Scope]map[string]WSAResource{
				{Project: "project1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					}),
				},
			},
			wantScopeCount:      1,
			wantSACount:         1,
			wantWSAMappingCount: 1,
		},
		{
			name: "multiple scopes with different WSAs",
			scopeMap: map[Scope]map[string]WSAResource{
				{Project: "project1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					}),
				},
				{Project: "project2", Environment: "env2", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa2": NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
					}),
				},
			},
			wantScopeCount:      2,
			wantSACount:         2,
			wantWSAMappingCount: 2,
		},
		{
			name: "multiple scopes with overlapping WSAs",
			scopeMap: map[Scope]map[string]WSAResource{
				{Project: "project1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					}),
					"wsa2": NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
					}),
				},
				{Project: "project2", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					"wsa1": NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
					}),
				},
			},
			wantScopeCount:      2,
			wantSACount:         2,
			wantWSAMappingCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewScopeComputationService(nil, nil)
			scopeToSA, saToWSAMap, wsaToSANames, serviceAccounts := service.GenerateServiceAccountMappings(tt.scopeMap)

			assert.Equal(t, tt.wantScopeCount, len(scopeToSA), "Scope to SA mapping count should match")
			assert.Equal(t, tt.wantSACount, len(serviceAccounts), "Service account count should match")
			assert.Equal(t, tt.wantWSAMappingCount, len(wsaToSANames), "WSA to SA mapping count should match")

			for scope, sa := range scopeToSA {
				found := false
				for _, serviceAccount := range serviceAccounts {
					if serviceAccount.Name == string(sa) {
						found = true
						break
					}
				}
				assert.True(t, found, "Service account %s should exist in service accounts slice", sa)

				wsaMap, exists := saToWSAMap[sa]
				assert.True(t, exists, "Service account %s should have WSA mapping", sa)
				assert.NotEmpty(t, wsaMap, "Service account %s should have at least one WSA", sa)

				originalWSAs, scopeExists := tt.scopeMap[scope]
				assert.True(t, scopeExists, "Original scope should exist in scope map")

				for wsaName, wsa := range wsaMap {
					originalWSA, wsaExists := originalWSAs[wsaName]
					assert.True(t, wsaExists, "WSA %s should exist in original scope", wsaName)
					assert.Equal(t, originalWSA, wsa, "WSA pointer should match original")
				}
			}

			for wsaName, saNames := range wsaToSANames {
				assert.NotEmpty(t, saNames, "WSA %s should map to at least one service account", wsaName)

				for _, saName := range saNames {
					found := false
					for _, sa := range serviceAccounts {
						if sa.Name == saName {
							found = true
							break
						}
					}
					assert.True(t, found, "Service account %s should exist", saName)
				}
			}

			for _, sa := range serviceAccounts {
				assert.Contains(t, sa.Name, "octopus-sa-", "Service account name should follow pattern")
				assert.NotNil(t, sa.Labels, "Service account should have labels")
				assert.NotNil(t, sa.Annotations, "Service account should have annotations")

				assert.Contains(t, sa.Labels, "agent.octopus.com/permissions", "Service account should have permissions label")
				assert.Equal(t, "enabled", sa.Labels["agent.octopus.com/permissions"], "Permissions should be enabled")
			}
		})
	}
}

func TestScopeComputationService_Integration(t *testing.T) {
	wsaList := []WSAResource{
		NewWSAResource(&v1beta1.WorkloadServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "wsa1"},
			Spec: v1beta1.WorkloadServiceAccountSpec{
				Scope: v1beta1.WorkloadServiceAccountScope{
					Projects:     []string{"project1"},
					Environments: []string{"env1"},
				},
			},
		}),
		NewWSAResource(&v1beta1.WorkloadServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "wsa2"},
			Spec: v1beta1.WorkloadServiceAccountSpec{
				Scope: v1beta1.WorkloadServiceAccountScope{
					Projects:     []string{"project1"},
					Environments: []string{"env2"},
				},
			},
		}),
	}

	service := NewScopeComputationService(nil, nil)

	scopeMap, vocabulary := service.ComputeScopesForWSAs(wsaList)
	assert.NotEmpty(t, scopeMap, "Should generate scopes")
	assert.NotNil(t, vocabulary, "Should generate vocabulary")

	scopeToSA, saToWSAMap, wsaToSANames, serviceAccounts := service.GenerateServiceAccountMappings(scopeMap)
	assert.NotEmpty(t, scopeToSA, "Should generate scope to SA mappings")
	assert.NotEmpty(t, serviceAccounts, "Should generate service accounts")

	updatedService := NewScopeComputationService(&vocabulary, &scopeToSA)

	for scope := range scopeMap {
		sa, err := updatedService.GetServiceAccountForScope(scope)
		assert.NoError(t, err, "Should be able to get SA for computed scope")
		assert.NotEmpty(t, sa, "Should return a service account name")

		found := false
		for _, serviceAccount := range serviceAccounts {
			if serviceAccount.Name == string(sa) {
				found = true
				break
			}
		}
		assert.True(t, found, "Returned SA should exist in generated service accounts")
	}

	for _, wsa := range wsaList {
		saNames, exists := wsaToSANames[wsa.GetName()]
		assert.True(t, exists, "WSA %s should be mapped to service accounts", wsa.GetName())
		assert.NotEmpty(t, saNames, "WSA %s should have at least one service account", wsa.GetName())
	}

	for sa, wsaMap := range saToWSAMap {
		for wsaName := range wsaMap {
			saNames := wsaToSANames[wsaName]
			assert.Contains(t, saNames, string(sa), "WSA %s should map back to SA %s", wsaName, sa)
		}
	}
}
