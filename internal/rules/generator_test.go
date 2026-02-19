package rules

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	vocab[ProjectGroupIndex].InsertSlice([]string{"project-group1", "project-group2"})
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

func TestGenerator(t *testing.T) {
	t.Run("getScopesForWSAs", func(t *testing.T) {
		t.Run("returns scopes mapped to WSAs", func(t *testing.T) {
			// Creating test vocabulary and WSA list
			testData := NewFindAllPossibleResourceScopesTest("Simple Scopes and WSAs")

			// Converting WSA pointers to WSAResource interface
			resources := make([]WSAResource, len(testData.WsaList))
			for i, wsa := range testData.WsaList {
				resources[i] = NewWSAResource(wsa)
			}

			// Generating scopes for WSAs
			result, _ := getScopesForWSAs(resources)

			// Verifying results
			if len(result) == 0 {
				t.Fatal("getScopesForWSAs should return non-empty result")
			}

			for scope, wsaMap := range result {
				if len(wsaMap) == 0 {
					t.Fatalf("Each scope should have at least one WSA: %v", scope)
				}
			}
		})
	})

	t.Run("GenerateServiceAccountMappings", func(t *testing.T) {
		t.Run("creates one service account for that set of WSAs", func(t *testing.T) {
			// Creating a WSA that appears in multiple scopes
			wsa1 := &v1beta1.WorkloadServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "wsa1", Namespace: "default"},
			}
			wsa1Key := types.NamespacedName{Namespace: "default", Name: "wsa1"}
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(wsa1),
				},
				{Project: "proj2", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(wsa1),
				},
			}

			// Generating service account mappings
			_, _, wsaToSANames, _ := GenerateServiceAccountMappings(scopeMap)

			// Verifying only one service account is created
			if _, exists := wsaToSANames[wsa1Key]; !exists {
				t.Fatalf("Expected wsaToSANames to contain key %v", wsa1Key)
			}
			if len(wsaToSANames[wsa1Key]) != 1 {
				t.Fatalf("Expected wsaToSANames[%v] to have length 1, got %d", wsa1Key, len(wsaToSANames[wsa1Key]))
			}
		})

		t.Run("creates a single service account for a WSA with a single scope", func(t *testing.T) {
			wsa1, _, wsa1Key, _ := setupTestData()
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(wsa1),
				},
			}

			scopeToSA, saToWSAs, wsaToSANames, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			if len(scopeToSA) != 1 {
				t.Fatalf("Number of scopes should be 1, got %d", len(scopeToSA))
			}
			if len(serviceAccounts) != 1 {
				t.Fatalf("Number of service accounts should be 1, got %d", len(serviceAccounts))
			}
			verifyServiceAccountMappings(t, scopeToSA, saToWSAs, wsaToSANames, serviceAccounts, scopeMap)
		})

		t.Run("creates two service accounts with overlapping WSAs", func(t *testing.T) {
			wsa1, wsa2, wsa1Key, wsa2Key := setupTestData()
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(wsa1),
					wsa2Key: NewWSAResource(wsa2),
				},
				{Project: "proj2", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(wsa1),
				},
			}

			scopeToSA, saToWSAs, wsaToSANames, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			if len(scopeToSA) != 2 {
				t.Fatalf("Number of scopes should be 2, got %d", len(scopeToSA))
			}
			if len(serviceAccounts) != 2 {
				t.Fatalf("Number of service accounts should be 2, got %d", len(serviceAccounts))
			}
			verifyServiceAccountMappings(t, scopeToSA, saToWSAs, wsaToSANames, serviceAccounts, scopeMap)
		})

		t.Run("creates one service account for multiple WSAs with identical scopes", func(t *testing.T) {
			_, _, wsa1Key, _ := setupTestData()
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1", Namespace: "default"},
					}),
				},
			}

			scopeToSA, saToWSAs, wsaToSANames, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			if len(scopeToSA) != 1 {
				t.Fatalf("Number of scopes should be 1, got %d", len(scopeToSA))
			}
			if len(serviceAccounts) != 1 {
				t.Fatalf("Number of service accounts should be 1, got %d", len(serviceAccounts))
			}
			verifyServiceAccountMappings(t, scopeToSA, saToWSAs, wsaToSANames, serviceAccounts, scopeMap)
		})

		t.Run("includes all scope dimensions in service account labels and annotations", func(t *testing.T) {
			wsa1, _, wsa1Key, _ := setupTestData()
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{
				{Project: "proj1", ProjectGroup: "group1", Environment: "env1", Tenant: "tenant1", Step: "step1", Space: "space1"}: {
					wsa1Key: NewWSAResource(wsa1),
				},
			}

			_, _, _, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			if len(serviceAccounts) != 1 {
				t.Fatalf("Number of service accounts should be 1, got %d", len(serviceAccounts))
			}

			sa := serviceAccounts[0]

			expectedLabels := []string{ProjectKey, ProjectGroupKey, EnvironmentKey, TenantKey, StepKey, SpaceKey}
			for _, key := range expectedLabels {
				if _, ok := sa.Labels[key]; !ok {
					t.Fatalf("Service account should have label %s", key)
				}
			}

			expectedAnnotations := map[string]string{
				ProjectKey:      "proj1",
				ProjectGroupKey: "group1",
				EnvironmentKey:  "env1",
				TenantKey:       "tenant1",
				StepKey:         "step1",
				SpaceKey:        "space1",
			}
			for key, expectedValue := range expectedAnnotations {
				if sa.Annotations[key] != expectedValue {
					t.Fatalf("Service account annotation %s should be '%s', got '%s'", key, expectedValue, sa.Annotations[key])
				}
			}
		})

		t.Run("creates no service accounts for an empty scope map", func(t *testing.T) {
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{}

			scopeToSA, _, _, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			if len(scopeToSA) != 0 {
				t.Fatalf("Number of scopes should be zero, got %d", len(scopeToSA))
			}
			if len(serviceAccounts) != 0 {
				t.Fatalf("Number of service accounts should be zero, got %d", len(serviceAccounts))
			}
		})
	})
}

func setupTestData() (*v1beta1.WorkloadServiceAccount, *v1beta1.WorkloadServiceAccount, types.NamespacedName, types.NamespacedName) {
	wsa1 := &v1beta1.WorkloadServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "wsa1", Namespace: "default"}}
	wsa2 := &v1beta1.WorkloadServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "wsa2", Namespace: "default"}}
	wsa1Key := types.NamespacedName{Namespace: "default", Name: "wsa1"}
	wsa2Key := types.NamespacedName{Namespace: "default", Name: "wsa2"}
	return wsa1, wsa2, wsa1Key, wsa2Key
}

// verifyServiceAccountMappings is a helper to verify the consistency of service account mappings
func verifyServiceAccountMappings(
	t *testing.T,
	scopeToSA map[Scope]ServiceAccountName,
	saToWSAs map[ServiceAccountName]map[types.NamespacedName]WSAResource,
	wsaToSANames map[types.NamespacedName][]string,
	serviceAccounts []*corev1.ServiceAccount,
	originalScopeMap map[Scope]map[types.NamespacedName]WSAResource,
) {
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
		if !found {
			t.Fatalf("Service account from scope map should exist in service accounts slice")
		}

		wsaMap, exists := saToWSAs[sa]
		if !exists {
			t.Fatalf("Service account should have WSA mapping")
		}
		if len(wsaMap) == 0 {
			t.Fatalf("Service account should have at least one WSA")
		}

		// Verify WSAs in the mapping exist in the original scope
		originalWSAs := originalScopeMap[scope]
		for wsaKey, wsa := range wsaMap {
			originalWSA, wsaExists := originalWSAs[wsaKey]
			if !wsaExists {
				t.Fatalf("WSA %s should exist in original scope %v", wsaKey, scope)
			}
			if !reflect.DeepEqual(wsa, originalWSA) {
				t.Fatalf("WSA pointer should match original")
			}
		}

		// Verify reverse mapping (WSA to service account names)
		for wsaKey := range wsaMap {
			saNames, exists := wsaToSANames[wsaKey]
			if !exists {
				t.Fatalf("WSA %s should have service account mapping", wsaKey)
			}
			found := slices.Contains(saNames, string(sa))
			if !found {
				t.Fatalf("WSA should map back to service account")
			}
		}
	}

	// Verify service account names follow the expected pattern (either human-readable or hash-based)
	for _, sa := range serviceAccounts {
		if !strings.HasPrefix(sa.Name, "octopus-sa-") {
			t.Fatalf("Service account name should follow the expected pattern")
		}
		if len(sa.Name) <= 5 {
			t.Fatalf("Service account name should have content after prefix")
		}

		// Verify service account has proper metadata
		if sa.Labels == nil {
			t.Fatalf("Service account should have labels")
		}
		if sa.Annotations == nil {
			t.Fatalf("Service account should have annotations")
		}
	}
}
