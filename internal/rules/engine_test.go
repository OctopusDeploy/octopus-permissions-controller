package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetKnownScopeCombination_GetsMostSpecificSet(t *testing.T) {
	type test struct {
		name     string
		scope    Scope
		expected Scope
	}

	// Generate a vocabulary with some known values
	vocabulary := NewGlobalVocabulary()
	vocabulary[ProjectIndex].Insert("project-1")
	vocabulary[EnvironmentIndex].Insert("env-1")
	vocabulary[TenantIndex].Insert("tenant-1")
	vocabulary[StepIndex].Insert("step-1")
	vocabulary[SpaceIndex].Insert("space-1")

	// Generate test scopes
	tests := []test{
		{
			name: "Single known value",
			scope: Scope{
				Project:     "project-1", // Known
				Environment: "env-2",     // Unknown
				Tenant:      "tenant-2",  // Unknown
				Step:        "step-2",    // Unknown
				Space:       "space-2",   // Unknown
			},
			expected: Scope{
				Project:     "project-1", // Known
				Environment: WildcardValue,
				Tenant:      WildcardValue,
				Step:        WildcardValue,
				Space:       WildcardValue,
			},
		},
		{
			name: "multiple known values",
			scope: Scope{
				Project:     "project-1", // Known
				Environment: "env-1",     // known
				Tenant:      "tenant-1",  // Known
				Step:        "step-2",    // Unknown
				Space:       "space-2",   // Unknown
			},
			expected: Scope{
				Project:     "project-1", // Known
				Environment: "env-1",     // known
				Tenant:      "tenant-1",  // Known
				Step:        WildcardValue,
				Space:       WildcardValue,
			},
		},
		{
			name: "all known values",
			scope: Scope{
				Project:     "project-1", // Known
				Environment: "env-1",     // known
				Tenant:      "tenant-1",  // Known
				Step:        "step-1",    // Known
				Space:       "space-1",   // Known
			},
			expected: Scope{
				Project:     "project-1", // Known
				Environment: "env-1",     // known
				Tenant:      "tenant-1",  // Known
				Step:        "step-1",    // Known
				Space:       "space-1",   // Known
			},
		},
		{
			name: "no known values",
			scope: Scope{
				Project:     "project-2", // Unknown
				Environment: "env-2",     // Unknown
				Tenant:      "tenant-2",  // Unknown
				Step:        "step-2",    // Unknown
				Space:       "space-2",   // Unknown
			},
			expected: Scope{
				Project:     WildcardValue,
				Environment: WildcardValue,
				Tenant:      WildcardValue,
				Step:        WildcardValue,
				Space:       WildcardValue,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			knownScope := vocabulary.GetKnownScopeCombination(tc.scope)
			assert.Equal(t, tc.expected, knownScope)
		})
	}
}
