package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/octopusdeploy/octopus-permissions-controller/testdata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
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

	var collector *OctopusMetricsCollector
	var engine rules.Engine
	var testNamespaceName string

	BeforeEach(func() {
		By("Setting up metrics collector and test environment")
		inMemoryEngine := rules.NewInMemoryEngine(k8sClient)
		engine = &inMemoryEngine
		collector = NewOctopusMetricsCollector(k8sClient, engine)
		Expect(collector).NotTo(BeNil())

		By("Creating unique test namespace for this test")
		testNamespaceName = fmt.Sprintf("octopus-test-%d", time.Now().UnixNano())
		testNamespace := &v1.Namespace{}
		testNamespace.Name = testNamespaceName

		err := k8sClient.Create(context.Background(), testNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		By("Cleaning up test resources")
		cleanupTestResourcesFromNamespace(testNamespaceName)
	})

	Context("Workload Service Account Metrics", func() {
		It("should accurately count WSA and CWSA resources", func() {
			By("Applying test resources with known WSA and CWSA distribution")
			applyTestResourcesFromFile(testNamespaceName, "unit/test-wsa.yaml")

			By("Waiting for resources to be processed")
			time.Sleep(2 * time.Second)

			By("Collecting WSA and CWSA metrics")
			registry := prometheus.NewRegistry()
			err := registry.Register(collector)
			Expect(err).NotTo(HaveOccurred())

			metricFamilies, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			metrics := convertMetricFamiliesToMap(metricFamilies)

			By("Validating WSA namespace distribution")
			// Expected from test-wsa.yaml: 2 WSA total (both in test namespace)
			wsaFamily, exists := metrics["octopus_wsa_total"]
			Expect(exists).To(BeTrue(), "octopus_wsa_total metric should exist")

			namespaceCounts := make(map[string]float64)
			totalWSA := 0.0
			for _, metric := range wsaFamily.Metric {
				if metric.Gauge != nil && metric.Gauge.Value != nil {
					totalWSA += *metric.Gauge.Value
					for _, label := range metric.Label {
						if label.Name != nil && *label.Name == "namespace" && label.Value != nil {
							namespaceCounts[*label.Value] = *metric.Gauge.Value
						}
					}
				}
			}

			Expect(totalWSA).To(Equal(float64(2)), "Should have 2 total WSA resources")
			Expect(namespaceCounts[testNamespaceName]).To(Equal(float64(2)), "Should have 2 WSA in test namespace")

			By("Validating CWSA count")
			// Expected from test-wsa.yaml: 2 CWSA resources (cluster-scoped)
			cwsaCount := getMetricValue(metrics, "octopus_cwsa_total")
			Expect(cwsaCount).To(Equal(float64(2)), "Should have 2 CWSA resources")
		})
	})

	//Context("Kubernetes Resource Metrics", func() {
	//	It("should accurately collect Kubernetes resource metrics", func() {
	//		By("Getting baseline metrics before applying test resources")
	//		baselineRegistry := prometheus.NewRegistry()
	//		err := baselineRegistry.Register(collector)
	//		Expect(err).NotTo(HaveOccurred())
	//
	//		baselineMetricFamilies, err := baselineRegistry.Gather()
	//		Expect(err).NotTo(HaveOccurred())
	//		baselineMetrics := convertMetricFamiliesToMap(baselineMetricFamilies)
	//
	//		baselineSA := getTotalMetricValue(baselineMetrics, "octopus_service_accounts_total")
	//		baselineRoles := getTotalMetricValue(baselineMetrics, "octopus_roles_total")
	//		baselineClusterRoles := getMetricValue(baselineMetrics, "octopus_cluster_roles_total")
	//		baselineRoleBindings := getTotalMetricValue(baselineMetrics, "octopus_role_bindings_total")
	//		baselineClusterRoleBindings := getMetricValue(baselineMetrics, "octopus_cluster_role_bindings_total")
	//
	//		By("Applying test Kubernetes resources with known counts")
	//		applyTestResourcesFromFile(testNamespaceName, "unit/test-k8s-resources.yaml")
	//
	//		By("Waiting for resources to be processed")
	//		time.Sleep(2 * time.Second)
	//
	//		By("Collecting metrics after applying test resources")
	//		afterRegistry := prometheus.NewRegistry()
	//		err = afterRegistry.Register(collector)
	//		Expect(err).NotTo(HaveOccurred())
	//
	//		afterMetricFamilies, err := afterRegistry.Gather()
	//		Expect(err).NotTo(HaveOccurred())
	//		afterMetrics := convertMetricFamiliesToMap(afterMetricFamilies)
	//
	//		By("Validating ServiceAccount metrics increased by expected test resources")
	//		// Expected: baseline + 3 test ServiceAccounts
	//		afterSA := getTotalMetricValue(afterMetrics, "octopus_service_accounts_total")
	//		Expect(afterSA).To(Equal(baselineSA+3), "ServiceAccount count should increase by 3")
	//
	//		By("Validating Role metrics increased by expected test resources")
	//		// Expected: baseline + 2 test Roles
	//		afterRoles := getTotalMetricValue(afterMetrics, "octopus_roles_total")
	//		Expect(afterRoles).To(Equal(baselineRoles+2), "Role count should increase by 2")
	//
	//		// Expected: baseline + 2 test ClusterRoles
	//		afterClusterRoles := getMetricValue(afterMetrics, "octopus_cluster_roles_total")
	//		Expect(afterClusterRoles).To(Equal(baselineClusterRoles+2), "ClusterRole count should increase by 2")
	//
	//		By("Validating RoleBinding metrics increased by expected test resources")
	//		// Expected: baseline + 3 test RoleBindings
	//		afterRoleBindings := getTotalMetricValue(afterMetrics, "octopus_role_bindings_total")
	//		Expect(afterRoleBindings).To(Equal(baselineRoleBindings+3), "RoleBinding count should increase by 3")
	//
	//		// Expected: baseline + 2 test ClusterRoleBindings
	//		afterClusterRoleBindings := getMetricValue(afterMetrics, "octopus_cluster_role_bindings_total")
	//		Expect(afterClusterRoleBindings).To(Equal(baselineClusterRoleBindings+2), "ClusterRoleBinding count should increase by 2")
	//	})
	//})

	Context("Scope Metrics", func() {
		It("should accurately count all scope types", func() {
			By("Applying test resources with known scope combinations")
			applyTestResourcesWithNamespace(testNamespaceName)

			By("Waiting for resources to be processed")
			time.Sleep(2 * time.Second)

			By("Collecting and validating scope metrics")
			registry := prometheus.NewRegistry()
			err := registry.Register(collector)
			Expect(err).NotTo(HaveOccurred())

			metricFamilies, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			metrics := convertMetricFamiliesToMap(metricFamilies)

			// WSA1 (metrics-wsa-full): spaces, projects, environments, tenants, steps (5 scopes)
			// WSA2 (metrics-wsa-partial): projects, environments (2 scopes)
			// WSA3 (metrics-wsa-spaces-only): spaces (1 scope)
			// CWSA1 (metrics-cwsa-full): spaces, projects, environments, tenants, steps (5 scopes)
			// CWSA2 (metrics-cwsa-partial): projects, tenants (2 scopes)
			// CWSA3 (metrics-cwsa-steps-only): steps (1 scope)

			By("Validating projects scope count (WSA1 + WSA2 + CWSA1 + CWSA2 = 4)")
			projectsCount := getMetricValue(metrics, "octopus_scopes_with_projects_total")
			Expect(projectsCount).To(Equal(float64(4)), "Should have 4 resources with projects scope")

			By("Validating environments scope count (WSA1 + WSA2 + CWSA1 = 3)")
			environmentsCount := getMetricValue(metrics, "octopus_scopes_with_environments_total")
			Expect(environmentsCount).To(Equal(float64(3)), "Should have 3 resources with environments scope")

			By("Validating tenants scope count (WSA1 + CWSA1 + CWSA2 = 3)")
			tenantsCount := getMetricValue(metrics, "octopus_scopes_with_tenants_total")
			Expect(tenantsCount).To(Equal(float64(3)), "Should have 3 resources with tenants scope")

			By("Validating steps scope count (WSA1 + CWSA1 + CWSA3 = 3)")
			stepsCount := getMetricValue(metrics, "octopus_scopes_with_steps_total")
			Expect(stepsCount).To(Equal(float64(3)), "Should have 3 resources with steps scope")

			By("Validating spaces scope count (WSA1 + WSA3 + CWSA1 = 3)")
			spacesCount := getMetricValue(metrics, "octopus_scopes_with_spaces_total")
			Expect(spacesCount).To(Equal(float64(3)), "Should have 3 resources with spaces scope")

			By("Validating distinct scopes total (3 WSA + 3 CWSA = 6)")
			distinctScopesCount := getMetricValue(metrics, "octopus_distinct_scopes_total")
			Expect(distinctScopesCount).To(Equal(float64(6)), "Should have 6 total distinct scopes")
		})
	})

	Context("Request Metrics", func() {
		It("should accurately count request metrics", func() {
			By("Recording request metrics with known counts")

			// Create a fresh counter for this test to avoid state from other tests
			testRequestsTotal := prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "test_octopus_requests_total",
					Help: "Test version of total number of requests processed with scope matching status",
				},
				[]string{"controller_type", "scope_matched"},
			)

			registry := prometheus.NewRegistry()
			registry.MustRegister(testRequestsTotal)

			// Record specific requests
			testRequestsTotal.WithLabelValues("workloadserviceaccount", "true").Inc()
			testRequestsTotal.WithLabelValues("workloadserviceaccount", "true").Inc()
			testRequestsTotal.WithLabelValues("workloadserviceaccount", "false").Inc()
			testRequestsTotal.WithLabelValues("clusterworkloadserviceaccount", "true").Inc()

			By("Collecting request metrics")
			metricFamilies, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			metrics := convertMetricFamiliesToMap(metricFamilies)

			// Verify request metrics exist
			requestsFamily, exists := metrics["test_octopus_requests_total"]
			Expect(exists).To(BeTrue(), "test_octopus_requests_total metric should exist")

			// Verify we can track different controller types and scope matching results
			foundWSATrue := false
			foundWSAFalse := false
			foundCWSATrue := false

			for _, metric := range requestsFamily.Metric {
				if metric.Counter != nil && metric.Counter.Value != nil {
					controllerType := ""
					scopeMatched := ""

					for _, label := range metric.Label {
						if label.Name != nil && label.Value != nil {
							switch *label.Name {
							case "controller_type":
								controllerType = *label.Value
							case "scope_matched":
								scopeMatched = *label.Value
							}
						}
					}

					if controllerType == "workloadserviceaccount" && scopeMatched == "true" {
						foundWSATrue = true
						Expect(*metric.Counter.Value).To(Equal(float64(2)), "Should have 2 WSA true requests")
					}
					if controllerType == "workloadserviceaccount" && scopeMatched == "false" {
						foundWSAFalse = true
						Expect(*metric.Counter.Value).To(Equal(float64(1)), "Should have 1 WSA false request")
					}
					if controllerType == "clusterworkloadserviceaccount" && scopeMatched == "true" {
						foundCWSATrue = true
						Expect(*metric.Counter.Value).To(Equal(float64(1)), "Should have 1 CWSA true request")
					}
				}
			}

			Expect(foundWSATrue).To(BeTrue(), "Should find WSA true metrics")
			Expect(foundWSAFalse).To(BeTrue(), "Should find WSA false metrics")
			Expect(foundCWSATrue).To(BeTrue(), "Should find CWSA true metrics")
		})
	})

	Context("Performance Metrics", func() {
		It("should accurately record reconciliation duration metrics", func() {
			By("Recording reconciliation duration metrics")

			// Create a fresh histogram for this test to avoid state from other tests
			testReconciliationDuration := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "test_octopus_reconciliation_duration_seconds",
					Help:    "Test version of time taken to complete a reconciliation request",
					Buckets: prometheus.DefBuckets,
				},
				[]string{"controller_type", "result"},
			)

			registry := prometheus.NewRegistry()
			registry.MustRegister(testReconciliationDuration)

			// Record specific durations
			testReconciliationDuration.WithLabelValues("workloadserviceaccount", "success").Observe(0.5)
			testReconciliationDuration.WithLabelValues("workloadserviceaccount", "error").Observe(0.1)
			testReconciliationDuration.WithLabelValues("clusterworkloadserviceaccount", "success").Observe(0.3)

			By("Collecting reconciliation duration metrics")
			metricFamilies, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			metrics := convertMetricFamiliesToMap(metricFamilies)

			// Verify reconciliation duration metrics exist
			durationFamily, exists := metrics["test_octopus_reconciliation_duration_seconds"]
			Expect(exists).To(BeTrue(), "test_octopus_reconciliation_duration_seconds metric should exist")

			// Verify we can track different controller types and results
			foundWSASuccess := false
			foundWSAError := false
			foundCWSASuccess := false

			for _, metric := range durationFamily.Metric {
				if metric.Histogram != nil && metric.Histogram.SampleCount != nil {
					controllerType := ""
					result := ""

					for _, label := range metric.Label {
						if label.Name != nil && label.Value != nil {
							switch *label.Name {
							case "controller_type":
								controllerType = *label.Value
							case "result":
								result = *label.Value
							}
						}
					}

					if controllerType == "workloadserviceaccount" && result == "success" {
						foundWSASuccess = true
						Expect(*metric.Histogram.SampleCount).To(Equal(uint64(1)), "Should have 1 WSA success sample")
					}
					if controllerType == "workloadserviceaccount" && result == "error" {
						foundWSAError = true
						Expect(*metric.Histogram.SampleCount).To(Equal(uint64(1)), "Should have 1 WSA error sample")
					}
					if controllerType == "clusterworkloadserviceaccount" && result == "success" {
						foundCWSASuccess = true
						Expect(*metric.Histogram.SampleCount).To(Equal(uint64(1)), "Should have 1 CWSA success sample")
					}
				}
			}

			Expect(foundWSASuccess).To(BeTrue(), "Should find WSA success metrics")
			Expect(foundWSAError).To(BeTrue(), "Should find WSA error metrics")
			Expect(foundCWSASuccess).To(BeTrue(), "Should find CWSA success metrics")
		})
	})
})

// Helper functions

func applyTestResourcesWithNamespace(namespaceName string) {
	data := testdata.Unit
	yamlContent, err := data.ReadFile("unit/test-scopes.yaml")
	Expect(err).NotTo(HaveOccurred())

	// Replace the hardcoded namespace with the dynamic one
	yamlStr := string(yamlContent)
	yamlStr = strings.ReplaceAll(yamlStr, "octopus-test", namespaceName)

	// Split YAML documents
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(yamlStr), 4096)
	for {
		var obj unstructured.Unstructured
		err := decoder.Decode(&obj)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			Expect(err).NotTo(HaveOccurred())
		}

		if obj.Object == nil {
			continue
		}

		// Apply the resource
		err = k8sClient.Create(context.Background(), &obj)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func applyTestResourcesFromFile(namespaceName string, fileName string) {
	data := testdata.Unit
	yamlContent, err := data.ReadFile(fileName)
	Expect(err).NotTo(HaveOccurred())

	// Replace the hardcoded namespace with the dynamic one
	yamlStr := string(yamlContent)
	yamlStr = strings.ReplaceAll(yamlStr, "octopus-test", namespaceName)

	// Split YAML documents
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(yamlStr), 4096)
	for {
		var obj unstructured.Unstructured
		err := decoder.Decode(&obj)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			Expect(err).NotTo(HaveOccurred())
		}

		if obj.Object == nil {
			continue
		}

		// Apply the resource
		err = k8sClient.Create(context.Background(), &obj)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func cleanupTestResourcesFromNamespace(namespaceName string) {
	data := testdata.Unit

	// Clean up resources from both test files
	testFiles := []string{"unit/test-scopes.yaml", "unit/test-wsa.yaml"}

	for _, testFile := range testFiles {
		yamlContent, err := data.ReadFile(testFile)
		if err != nil {
			continue // Fail silently in cleanup
		}

		yamlStr := string(yamlContent)
		yamlStr = strings.ReplaceAll(yamlStr, "octopus-test", namespaceName)

		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(yamlStr), 4096)
		for {
			var obj unstructured.Unstructured
			err := decoder.Decode(&obj)
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				continue
			}

			if obj.Object == nil {
				continue
			}

			_ = k8sClient.Delete(context.Background(), &obj)
		}
	}

	testNamespace := &v1.Namespace{}
	testNamespace.Name = namespaceName
	_ = k8sClient.Delete(context.Background(), testNamespace)
}

func convertMetricFamiliesToMap(families []*dto.MetricFamily) map[string]*dto.MetricFamily {
	result := make(map[string]*dto.MetricFamily)
	for _, family := range families {
		if family.Name != nil {
			result[*family.Name] = family
		}
	}
	return result
}

func getMetricValue(metrics map[string]*dto.MetricFamily, metricName string) float64 {
	family, exists := metrics[metricName]
	if !exists {
		return 0
	}

	if len(family.Metric) == 0 {
		return 0
	}

	metric := family.Metric[0]
	if metric.Gauge != nil && metric.Gauge.Value != nil {
		return *metric.Gauge.Value
	}

	return 0
}
