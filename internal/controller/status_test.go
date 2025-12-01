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
	"testing"
	"time"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConditionsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []metav1.Condition
		b        []metav1.Condition
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "both empty",
			a:        []metav1.Condition{},
			b:        []metav1.Condition{},
			expected: true,
		},
		{
			name: "different lengths",
			a: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
			},
			b:        []metav1.Condition{},
			expected: false,
		},
		{
			name: "same conditions same order",
			a: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				{Type: "Planning", Status: metav1.ConditionTrue, Reason: "Complete"},
			},
			b: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				{Type: "Planning", Status: metav1.ConditionTrue, Reason: "Complete"},
			},
			expected: true,
		},
		{
			name: "same conditions different order",
			a: []metav1.Condition{
				{Type: "Planning", Status: metav1.ConditionTrue, Reason: "Complete"},
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
			},
			b: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				{Type: "Planning", Status: metav1.ConditionTrue, Reason: "Complete"},
			},
			expected: true,
		},
		{
			name: "different status",
			a: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
			},
			b: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Success"},
			},
			expected: false,
		},
		{
			name: "different reason",
			a: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
			},
			b: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Failed"},
			},
			expected: false,
		},
		{
			name: "different message is ignored",
			a: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success", Message: "msg1"},
			},
			b: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success", Message: "msg2"},
			},
			expected: true,
		},
		{
			name: "different lastTransitionTime is ignored",
			a: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success", LastTransitionTime: metav1.NewTime(time.Now())},
			},
			b: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success", LastTransitionTime: metav1.NewTime(time.Now().Add(time.Hour))},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConditionsEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("ConditionsEqual() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestWSAStatusNeedsUpdate(t *testing.T) {
	tests := []struct {
		name     string
		current  *agentoctopuscomv1beta1.WorkloadServiceAccountStatus
		desired  *agentoctopuscomv1beta1.WorkloadServiceAccountStatus
		expected bool
	}{
		{
			name:     "both nil",
			current:  nil,
			desired:  nil,
			expected: false,
		},
		{
			name:    "current nil desired not nil",
			current: nil,
			desired: &agentoctopuscomv1beta1.WorkloadServiceAccountStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				},
			},
			expected: true,
		},
		{
			name: "same conditions",
			current: &agentoctopuscomv1beta1.WorkloadServiceAccountStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				},
			},
			desired: &agentoctopuscomv1beta1.WorkloadServiceAccountStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				},
			},
			expected: false,
		},
		{
			name: "different conditions",
			current: &agentoctopuscomv1beta1.WorkloadServiceAccountStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				},
			},
			desired: &agentoctopuscomv1beta1.WorkloadServiceAccountStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Failed"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WSAStatusNeedsUpdate(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("WSAStatusNeedsUpdate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestCWSAStatusNeedsUpdate(t *testing.T) {
	tests := []struct {
		name     string
		current  *agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus
		desired  *agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus
		expected bool
	}{
		{
			name:     "both nil",
			current:  nil,
			desired:  nil,
			expected: false,
		},
		{
			name: "same conditions",
			current: &agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				},
			},
			desired: &agentoctopuscomv1beta1.ClusterWorkloadServiceAccountStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Success"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CWSAStatusNeedsUpdate(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("CWSAStatusNeedsUpdate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
