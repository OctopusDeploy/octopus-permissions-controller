package rules

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDiscoverTargetNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name        string
		deployments []appsv1.Deployment
		expected    []string
		regex       *regexp.Regexp
	}{
		{
			name:        "no deployments should return default namespace",
			deployments: []appsv1.Deployment{},
			expected:    []string{"default"},
			regex:       regexp.MustCompile("^octopus-(agent|worker)-.*"),
		},
		{
			name: "multiple deployments with duplicate namespace should return unique namespaces",
			deployments: []appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "octopus-agent-1",
						Namespace: "octopus-agent-1",
						Labels: map[string]string{
							"app.kubernetes.io/name": "octopus-agent",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "octopus-agent-2",
						Namespace: "octopus-agent-1", // duplicate namespace
						Labels: map[string]string{
							"app.kubernetes.io/name": "octopus-agent",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "octopus-agent-3",
						Namespace: "octopus-agent-3",
						Labels: map[string]string{
							"app.kubernetes.io/name": "octopus-agent",
						},
					},
				},
			},
			expected: []string{"octopus-agent-1", "octopus-agent-3"}, // should be sorted and unique
			regex:    regexp.MustCompile("^octopus-(agent|worker)-.*"),
		},
		{
			name: "namespaces that don't match the regex should not be returned",
			deployments: []appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "octopus-agent-3",
						Namespace: "my-scary-namespace",
						Labels: map[string]string{
							"app.kubernetes.io/name": "octopus-agent",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "octopus-agent-3",
						Namespace: "octopus-agent-namespace",
						Labels: map[string]string{
							"app.kubernetes.io/name": "octopus-agent",
						},
					},
				},
			},
			expected: []string{"octopus-agent-namespace"},
			regex:    regexp.MustCompile("^octopus-(agent|worker)-.*"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test deployments
			var objects []client.Object
			for i := range tt.deployments {
				objects = append(objects, &tt.deployments[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Test the function
			service := NamespaceDiscoveryService{TargetNamespaceRegex: tt.regex}
			result, err := service.DiscoverTargetNamespaces(context.Background(), fakeClient)

			// Assertions
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}
