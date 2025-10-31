package rules

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GlobalVocabulary", func() {
	var vocabulary GlobalVocabulary

	BeforeEach(func() {
		// Generate a vocabulary with some known values
		vocabulary = NewGlobalVocabulary()
		vocabulary[ProjectIndex].Insert("project-1")
		vocabulary[EnvironmentIndex].Insert("env-1")
		vocabulary[TenantIndex].Insert("tenant-1")
		vocabulary[StepIndex].Insert("step-1")
		vocabulary[SpaceIndex].Insert("space-1")
	})

	Describe("GetKnownScopeCombination", func() {
		Context("When getting most specific scope combination", func() {
			It("Should return known values and wildcards for single known value", func() {
				scope := Scope{
					Project:     "project-1", // Known
					Environment: "env-2",     // Unknown
					Tenant:      "tenant-2",  // Unknown
					Step:        "step-2",    // Unknown
					Space:       "space-2",   // Unknown
				}
				expected := Scope{
					Project:     "project-1", // Known
					Environment: WildcardValue,
					Tenant:      WildcardValue,
					Step:        WildcardValue,
					Space:       WildcardValue,
				}

				knownScope := vocabulary.GetKnownScopeCombination(scope)
				Expect(knownScope).To(Equal(expected))
			})

			It("Should return known values and wildcards for multiple known values", func() {
				scope := Scope{
					Project:     "project-1", // Known
					Environment: "env-1",     // Known
					Tenant:      "tenant-1",  // Known
					Step:        "step-2",    // Unknown
					Space:       "space-2",   // Unknown
				}
				expected := Scope{
					Project:     "project-1", // Known
					Environment: "env-1",     // Known
					Tenant:      "tenant-1",  // Known
					Step:        WildcardValue,
					Space:       WildcardValue,
				}

				knownScope := vocabulary.GetKnownScopeCombination(scope)
				Expect(knownScope).To(Equal(expected))
			})

			It("Should return all known values when all values are in vocabulary", func() {
				scope := Scope{
					Project:     "project-1", // Known
					Environment: "env-1",     // Known
					Tenant:      "tenant-1",  // Known
					Step:        "step-1",    // Known
					Space:       "space-1",   // Known
				}
				expected := Scope{
					Project:     "project-1", // Known
					Environment: "env-1",     // Known
					Tenant:      "tenant-1",  // Known
					Step:        "step-1",    // Known
					Space:       "space-1",   // Known
				}

				knownScope := vocabulary.GetKnownScopeCombination(scope)
				Expect(knownScope).To(Equal(expected))
			})

			It("Should return all wildcards when no values are known", func() {
				scope := Scope{
					Project:     "project-2", // Unknown
					Environment: "env-2",     // Unknown
					Tenant:      "tenant-2",  // Unknown
					Step:        "step-2",    // Unknown
					Space:       "space-2",   // Unknown
				}
				expected := Scope{
					Project:     WildcardValue,
					Environment: WildcardValue,
					Tenant:      WildcardValue,
					Step:        WildcardValue,
					Space:       WildcardValue,
				}

				knownScope := vocabulary.GetKnownScopeCombination(scope)
				Expect(knownScope).To(Equal(expected))
			})
		})
	})
})
