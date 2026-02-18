package rules

import (
	"reflect"
	"testing"
)

const (
	// known slugs
	knownProject      = "project-1"
	knownProjectGroup = "known-project-group"
	knownEnvironment  = "env-1"
	knownTenant       = "tenant-1"
	knownStep         = "step-1"
	knownSpace        = "space-1"

	// unknown slugs
	unknownProject      = "project-2"
	unknownProjectGroup = "unknown-project-group"
	unknownEnvironment  = "env-2"
	unknownTenant       = "tenant-2"
	unknownStep         = "step-2"
	unknownSpace        = "space-2"
)

func TestGlobalVocabulary_GetKnownScopeCombination(t *testing.T) {
	setupVocabulary := func() GlobalVocabulary {
		vocabulary := NewGlobalVocabulary()
		vocabulary[ProjectIndex].Insert(knownProject)
		vocabulary[ProjectGroupIndex].Insert(knownProjectGroup)
		vocabulary[EnvironmentIndex].Insert(knownEnvironment)
		vocabulary[TenantIndex].Insert(knownTenant)
		vocabulary[StepIndex].Insert(knownStep)
		vocabulary[SpaceIndex].Insert(knownSpace)
		return vocabulary
	}

	t.Run("returns known values and wildcards for single known value", func(t *testing.T) {
		vocabulary := setupVocabulary()
		scope := Scope{
			Project:      knownProject,
			ProjectGroup: unknownProjectGroup,
			Environment:  unknownEnvironment,
			Tenant:       unknownTenant,
			Step:         unknownStep,
			Space:        unknownSpace,
		}
		expected := Scope{
			Project:      knownProject,
			ProjectGroup: WildcardValue,
			Environment:  WildcardValue,
			Tenant:       WildcardValue,
			Step:         WildcardValue,
			Space:        WildcardValue,
		}

		knownScope := vocabulary.GetKnownScopeCombination(scope)
		if !reflect.DeepEqual(knownScope, expected) {
			t.Errorf("Expected %+v, got %+v", expected, knownScope)
		}
	})

	t.Run("returns known values and wildcards for multiple known values", func(t *testing.T) {
		vocabulary := setupVocabulary()
		scope := Scope{
			Project:      knownProject,
			ProjectGroup: knownProjectGroup,
			Environment:  knownEnvironment,
			Tenant:       knownTenant,
			Step:         unknownStep,
			Space:        unknownSpace,
		}
		expected := Scope{
			Project:      knownProject,
			ProjectGroup: knownProjectGroup,
			Environment:  knownEnvironment,
			Tenant:       knownTenant,
			Step:         WildcardValue,
			Space:        WildcardValue,
		}

		knownScope := vocabulary.GetKnownScopeCombination(scope)
		if !reflect.DeepEqual(knownScope, expected) {
			t.Errorf("Expected %+v, got %+v", expected, knownScope)
		}
	})

	t.Run("returns all known values when all values are in vocabulary", func(t *testing.T) {
		vocabulary := setupVocabulary()
		scope := Scope{
			Project:      knownProject,
			ProjectGroup: knownProjectGroup,
			Environment:  knownEnvironment,
			Tenant:       knownTenant,
			Step:         knownStep,
			Space:        knownSpace,
		}
		expected := Scope{
			Project:      knownProject,
			ProjectGroup: knownProjectGroup,
			Environment:  knownEnvironment,
			Tenant:       knownTenant,
			Step:         knownStep,
			Space:        knownSpace,
		}

		knownScope := vocabulary.GetKnownScopeCombination(scope)
		if !reflect.DeepEqual(knownScope, expected) {
			t.Errorf("Expected %+v, got %+v", expected, knownScope)
		}
	})

	t.Run("returns all wildcards when no values are known", func(t *testing.T) {
		vocabulary := setupVocabulary()
		scope := Scope{
			Project:      unknownProject,
			ProjectGroup: unknownProjectGroup,
			Environment:  unknownEnvironment,
			Tenant:       unknownTenant,
			Step:         unknownStep,
			Space:        unknownSpace,
		}
		expected := Scope{
			Project:      WildcardValue,
			ProjectGroup: WildcardValue,
			Environment:  WildcardValue,
			Tenant:       WildcardValue,
			Step:         WildcardValue,
			Space:        WildcardValue,
		}

		knownScope := vocabulary.GetKnownScopeCombination(scope)
		if !reflect.DeepEqual(knownScope, expected) {
			t.Errorf("Expected %+v, got %+v", expected, knownScope)
		}
	})
}
