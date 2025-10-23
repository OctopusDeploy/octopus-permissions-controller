package metrics

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Test", func() {
	Context("When getting metrics", func() {
		BeforeEach(func() {
			By("creating the metrics collector")
			a := NewMetricsCollector(k8sClient)
			err := a.CollectResourceMetrics(context.TODO())
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			By("Cleanup the specific resource instance WorkloadServiceAccount")
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
		})
	})
})
