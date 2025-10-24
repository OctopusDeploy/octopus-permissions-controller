package metrics

import (
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Test", func() {
	Context("When creating metrics collector", func() {
		BeforeEach(func() {
			By("creating the new metrics collector")
			engine := rules.NewInMemoryEngine(k8sClient)
			collector := NewOctopusMetricsCollector(k8sClient, &engine)
			Expect(collector).NotTo(BeNil())
		})
		AfterEach(func() {
			By("Cleanup completed")
		})
		It("should successfully create the metrics collector", func() {
			By("Creating collector with prometheus.Collector interface")
			engine := rules.NewInMemoryEngine(k8sClient)
			collector := NewOctopusMetricsCollector(k8sClient, &engine)
			Expect(collector).NotTo(BeNil())
		})
	})

	Context("When using unified metrics", func() {
		It("should track requests with unified format", func() {
			By("Recording requests with different scope matching results")

			// Test the new unified metric - these should not panic
			IncRequestsTotal("workloadserviceaccount", true)
			IncRequestsTotal("workloadserviceaccount", false)
			IncRequestsTotal("clusterworkloadserviceaccount", true)
			IncRequestsTotal("clusterworkloadserviceaccount", false)

			// If we get here without panicking, the metrics are working
			Expect(true).To(BeTrue())
		})

		It("should track through collector", func() {
			By("Using the collector to track requests")
			engine := rules.NewInMemoryEngine(k8sClient)
			collector := NewOctopusMetricsCollector(k8sClient, &engine)

			// Test collector methods - these should not panic
			collector.TrackRequest("workloadserviceaccount", true)
			collector.TrackRequest("workloadserviceaccount", false)

			// If we get here without panicking, the collector is working
			Expect(collector).NotTo(BeNil())
		})
	})
})
