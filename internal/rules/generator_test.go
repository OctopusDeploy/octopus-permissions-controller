package rules

import (
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("Scope Generation", func() {
	Describe("getScopesForWSAs", func() {
		Context("When generating scopes for WorkloadServiceAccounts", func() {
			It("Should return non-empty results with scopes mapped to WSAs", func() {
				By("Creating test vocabulary and WSA list")
				testData := NewFindAllPossibleResourceScopesTest("Simple Scopes and WSAs")

				By("Converting WSA pointers to WSAResource interface")
				resources := make([]WSAResource, len(testData.WsaList))
				for i, wsa := range testData.WsaList {
					resources[i] = NewWSAResource(wsa)
				}

				By("Generating scopes for WSAs")
				result, _ := getScopesForWSAs(resources)

				By("Verifying results")
				Expect(result).NotTo(BeEmpty(), "getScopesForWSAs should return non-empty result")

				for scope, wsaMap := range result {
					Expect(wsaMap).NotTo(BeEmpty(), "Each scope should have at least one WSA: %v", scope)
				}
			})
		})
	})
})

var _ = Describe("Service Account Deduplication", func() {
	Context("When multiple scopes map to the same set of WSAs", func() {
		It("Should create only one service account for that set of WSAs", func() {
			By("Creating a WSA that appears in multiple scopes")
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

			By("Generating service account mappings")
			_, _, wsaToSANames, _ := GenerateServiceAccountMappings(scopeMap)

			By("Verifying only one service account is created")
			Expect(wsaToSANames).To(HaveKey(wsa1Key))
			Expect(wsaToSANames[wsa1Key]).To(HaveLen(1))
		})
	})
})

var _ = Describe("GenerateServiceAccountMappings", func() {
	var wsa1, wsa2 *v1beta1.WorkloadServiceAccount
	var wsa1Key, wsa2Key types.NamespacedName

	BeforeEach(func() {
		wsa1 = &v1beta1.WorkloadServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "wsa1", Namespace: "default"}}
		wsa2 = &v1beta1.WorkloadServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "wsa2", Namespace: "default"}}
		wsa1Key = types.NamespacedName{Namespace: "default", Name: "wsa1"}
		wsa2Key = types.NamespacedName{Namespace: "default", Name: "wsa2"}
	})

	Context("When generating service account mappings", func() {
		It("Should handle single scope with one WSA", func() {
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(wsa1),
				},
			}

			scopeToSA, saToWSAs, wsaToSANames, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			Expect(scopeToSA).To(HaveLen(1), "Number of scopes should match")
			Expect(serviceAccounts).To(HaveLen(1), "Number of service accounts should match")
			verifyServiceAccountMappings(scopeToSA, saToWSAs, wsaToSANames, serviceAccounts, scopeMap)
		})

		It("Should handle multiple scopes with overlapping WSAs", func() {
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

			Expect(scopeToSA).To(HaveLen(2), "Number of scopes should match")
			Expect(serviceAccounts).To(HaveLen(2), "Number of service accounts should match")
			verifyServiceAccountMappings(scopeToSA, saToWSAs, wsaToSANames, serviceAccounts, scopeMap)
		})

		It("Should create one service account for identical scopes", func() {
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{
				{Project: "proj1", Environment: "env1", Tenant: "*", Step: "*", Space: "*"}: {
					wsa1Key: NewWSAResource(&v1beta1.WorkloadServiceAccount{
						ObjectMeta: metav1.ObjectMeta{Name: "wsa1", Namespace: "default"},
					}),
				},
			}

			scopeToSA, saToWSAs, wsaToSANames, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			Expect(scopeToSA).To(HaveLen(1), "Number of scopes should match")
			Expect(serviceAccounts).To(HaveLen(1), "Number of service accounts should match")
			verifyServiceAccountMappings(scopeToSA, saToWSAs, wsaToSANames, serviceAccounts, scopeMap)
		})

		It("Should handle empty scope map", func() {
			scopeMap := map[Scope]map[types.NamespacedName]WSAResource{}

			scopeToSA, _, _, serviceAccounts := GenerateServiceAccountMappings(scopeMap)

			Expect(scopeToSA).To(BeEmpty(), "Number of scopes should be zero")
			Expect(serviceAccounts).To(BeEmpty(), "Number of service accounts should be zero")
		})
	})
})

// verifyServiceAccountMappings is a helper to verify the consistency of service account mappings
func verifyServiceAccountMappings(
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
		Expect(found).To(BeTrue(), "Service account from scope map should exist in service accounts slice")

		wsaMap, exists := saToWSAs[sa]
		Expect(exists).To(BeTrue(), "Service account should have WSA mapping")
		Expect(wsaMap).NotTo(BeEmpty(), "Service account should have at least one WSA")

		// Verify WSAs in the mapping exist in the original scope
		originalWSAs := originalScopeMap[scope]
		for wsaKey, wsa := range wsaMap {
			originalWSA, wsaExists := originalWSAs[wsaKey]
			Expect(wsaExists).To(BeTrue(), "WSA %s should exist in original scope %v", wsaKey, scope)
			Expect(wsa).To(Equal(originalWSA), "WSA pointer should match original")
		}

		// Verify reverse mapping (WSA to service account names)
		for wsaKey := range wsaMap {
			saNames, exists := wsaToSANames[wsaKey]
			Expect(exists).To(BeTrue(), "WSA %s should have service account mapping", wsaKey)
			Expect(saNames).To(ContainElement(string(sa)), "WSA should map back to service account")
		}
	}

	// Verify service account names follow the expected pattern (either human-readable or hash-based)
	for _, sa := range serviceAccounts {
		Expect(sa.Name).To(HavePrefix("octopus-sa-"), "Service account name should follow the expected pattern")
		Expect(len(sa.Name)).To(BeNumerically(">", 5), "Service account name should have content after prefix")

		// Verify service account has proper metadata
		Expect(sa.Labels).NotTo(BeNil(), "Service account should have labels")
		Expect(sa.Annotations).NotTo(BeNil(), "Service account should have annotations")
	}
}

// generateServiceAccountName tests removed - function is deprecated in favor of generateGroupedServiceAccountName
