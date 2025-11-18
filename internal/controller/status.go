/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"sort"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionsEqual compares two condition slices for equality in an order-insensitive manner.
// It returns true if both slices contain the same conditions with matching Type, Status, and Reason.
// Message and LastTransitionTime are intentionally not compared to avoid spurious updates.
func ConditionsEqual(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}

	if len(a) == 0 {
		return true
	}

	sortedA := make([]metav1.Condition, len(a))
	sortedB := make([]metav1.Condition, len(b))
	copy(sortedA, a)
	copy(sortedB, b)

	sort.Slice(sortedA, func(i, j int) bool { return sortedA[i].Type < sortedA[j].Type })
	sort.Slice(sortedB, func(i, j int) bool { return sortedB[i].Type < sortedB[j].Type })

	for i := range sortedA {
		if sortedA[i].Type != sortedB[i].Type ||
			sortedA[i].Status != sortedB[i].Status ||
			sortedA[i].Reason != sortedB[i].Reason {
			return false
		}
	}
	return true
}

// WSAStatusNeedsUpdate determines if a WorkloadServiceAccount status update is needed.
// It returns true if the conditions have meaningfully changed.
func WSAStatusNeedsUpdate(current, desired *agentoctopuscomv1beta1.WorkloadServiceAccountStatus) bool {
	if current == nil && desired == nil {
		return false
	}
	if current == nil || desired == nil {
		return true
	}
	return !ConditionsEqual(current.Conditions, desired.Conditions)
}

// CWSAStatusNeedsUpdate determines if a ClusterWorkloadServiceAccount status update is needed.
// It returns true if the conditions have meaningfully changed.
func CWSAStatusNeedsUpdate(current, desired *agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus) bool {
	if current == nil && desired == nil {
		return false
	}
	if current == nil || desired == nil {
		return true
	}
	return !ConditionsEqual(current.Conditions, desired.Conditions)
}
