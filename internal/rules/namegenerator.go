package rules

import (
	"crypto/sha256"
	"fmt"
)

// GenerateServiceAccountName generates a ServiceAccountName based on the given scope
func GenerateServiceAccountName(scope Scope) ServiceAccountName {
	hash := sha256.Sum256([]byte(scope.String()))
	fullHash := fmt.Sprintf("%x", hash)

	return ServiceAccountName(fmt.Sprintf("octopus-sa-%s", fullHash))
}
