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
})
