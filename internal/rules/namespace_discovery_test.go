package rules

import (
	"context"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("NamespaceDiscoveryService", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("DiscoverTargetNamespaces", func() {
		Context("When discovering target namespaces from deployments", func() {
			It("Should return default namespace when no deployments exist", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				regex := regexp.MustCompile("^octopus-(agent|worker)-.*")
				service := NamespaceDiscoveryService{TargetNamespaceRegex: regex}
				result, err := service.DiscoverTargetNamespaces(context.Background(), fakeClient)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ConsistOf("default"))
			})

			It("Should return unique namespaces when multiple deployments share namespaces", func() {
				deployments := []appsv1.Deployment{
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
				}

				var objects []client.Object
				for i := range deployments {
					objects = append(objects, &deployments[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(objects...).
					Build()

				regex := regexp.MustCompile("^octopus-(agent|worker)-.*")
				service := NamespaceDiscoveryService{TargetNamespaceRegex: regex}
				result, err := service.DiscoverTargetNamespaces(context.Background(), fakeClient)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ConsistOf("octopus-agent-1", "octopus-agent-3"))
			})

			It("Should filter out namespaces that don't match the regex", func() {
				deployments := []appsv1.Deployment{
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
				}

				var objects []client.Object
				for i := range deployments {
					objects = append(objects, &deployments[i])
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(objects...).
					Build()

				regex := regexp.MustCompile("^octopus-(agent|worker)-.*")
				service := NamespaceDiscoveryService{TargetNamespaceRegex: regex}
				result, err := service.DiscoverTargetNamespaces(context.Background(), fakeClient)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ConsistOf("octopus-agent-namespace"))
			})
		})
	})
})
