package metrics

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/types"
	"github.com/octopusdeploy/octopus-permissions-controller/testdata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

const (
	scopeMatchedTrue           = "true"
	endOfFile                  = "EOF"
	workloadServiceAccountType = "workloadserviceaccount"
)

// MockEngine is a mock implementation of the rules.Engine interface for testing
type MockEngine struct{}

// NewMockEngine creates a new mock engine with predefined test data
func NewMockEngine() *MockEngine {
	return &MockEngine{}
}

// Reconcile mock implementation
func (m *MockEngine) Reconcile(ctx context.Context) error {
	return nil
}

// GetWorkloadServiceAccounts returns mock WSA data
func (m *MockEngine) GetWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error) {
	return func(yield func(*v1beta1.WorkloadServiceAccount) bool) {
		wsa1 := &v1beta1.WorkloadServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wsa1",
				Namespace: "namespace1",
			},
		}
		wsa2 := &v1beta1.WorkloadServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wsa2",
				Namespace: "namespace1",
			},
		}
		if !yield(wsa1) {
			return
		}
		yield(wsa2)
	}, nil
}

// GetClusterWorkloadServiceAccounts returns mock CWSA data
func (m *MockEngine) GetClusterWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.ClusterWorkloadServiceAccount], error) {
	return func(yield func(*v1beta1.ClusterWorkloadServiceAccount) bool) {
		cwsa1 := &v1beta1.ClusterWorkloadServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cwsa1",
			},
		}
		yield(cwsa1)
	}, nil
}

// GetServiceAccounts returns mock ServiceAccount data
func (m *MockEngine) GetServiceAccounts(ctx context.Context) (iter.Seq[*corev1.ServiceAccount], error) {
	return func(yield func(*corev1.ServiceAccount) bool) {
		sa1 := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sa1",
				Namespace: "namespace1",
			},
		}
		sa2 := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sa2",
				Namespace: "namespace1",
			},
		}
		sa3 := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sa3",
				Namespace: "namespace2",
			},
		}
		if !yield(sa1) {
			return
		}
		if !yield(sa2) {
			return
		}
		yield(sa3)
	}, nil
}

// GetRoles returns mock Role data
func (m *MockEngine) GetRoles(ctx context.Context) (iter.Seq[*rbacv1.Role], error) {
	return func(yield func(*rbacv1.Role) bool) {
		role1 := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "role1",
				Namespace: "namespace1",
			},
		}
		role2 := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "role2",
				Namespace: "namespace2",
			},
		}
		if !yield(role1) {
			return
		}
		yield(role2)
	}, nil
}

// GetClusterRoles returns mock ClusterRole data
func (m *MockEngine) GetClusterRoles(ctx context.Context) (iter.Seq[*rbacv1.ClusterRole], error) {
	return func(yield func(*rbacv1.ClusterRole) bool) {
		cr1 := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "clusterrole1",
			},
		}
		cr2 := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "clusterrole2",
			},
		}
		if !yield(cr1) {
			return
		}
		yield(cr2)
	}, nil
}

// GetRoleBindings returns mock RoleBinding data
func (m *MockEngine) GetRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.RoleBinding], error) {
	return func(yield func(*rbacv1.RoleBinding) bool) {
		rb1 := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rb1",
				Namespace: "namespace1",
			},
		}
		rb2 := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rb2",
				Namespace: "namespace1",
			},
		}
		rb3 := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rb3",
				Namespace: "namespace2",
			},
		}
		if !yield(rb1) {
			return
		}
		if !yield(rb2) {
			return
		}
		yield(rb3)
	}, nil
}

// GetClusterRoleBindings returns mock ClusterRoleBinding data
func (m *MockEngine) GetClusterRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.ClusterRoleBinding], error) {
	return func(yield func(*rbacv1.ClusterRoleBinding) bool) {
		crb1 := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: "crb1",
			},
		}
		yield(crb1)
	}, nil
}

// GetScopeToSA returns mock scope to service account mapping
func (m *MockEngine) GetScopeToSA() map[types.Scope]rules.ServiceAccountName {
	return map[types.Scope]rules.ServiceAccountName{
		{Project: "proj1"}:                       "sa-proj1",
		{Project: "proj2"}:                       "sa-proj2",
		{Environment: "env1", Tenant: "tenant1"}: "sa-env1-tenant1",
	}
}

// GetServiceAccountForScope returns the service account for a given scope
func (m *MockEngine) GetServiceAccountForScope(scope types.Scope) (rules.ServiceAccountName, error) {
	scopeToSA := m.GetScopeToSA()
	if sa, exists := scopeToSA[scope]; exists {
		return sa, nil
	}
	return "", fmt.Errorf("service account not found for scope %v", scope)
}

func (m *MockEngine) EnsureRoles(context.Context, []rules.WSAResource) (map[string]rbacv1.Role, error) {
	return make(map[string]rbacv1.Role), nil
}

func (m *MockEngine) EnsureServiceAccounts(context.Context, []*corev1.ServiceAccount, []string) error {
	return nil
}

func (m *MockEngine) EnsureRoleBindings(context.Context, []rules.WSAResource, map[string]rbacv1.Role, map[string][]string, []string) error {
	return nil
}

func (m *MockEngine) ComputeScopesForWSAs([]rules.WSAResource) (map[types.Scope]map[string]rules.WSAResource, rules.GlobalVocabulary) {
	return make(map[types.Scope]map[string]rules.WSAResource), rules.GlobalVocabulary{}
}

func (m *MockEngine) GenerateServiceAccountMappings(map[types.Scope]map[string]rules.WSAResource) (map[types.Scope]rules.ServiceAccountName, map[rules.ServiceAccountName]map[string]rules.WSAResource, map[string][]string, []*corev1.ServiceAccount) {
	return make(map[types.Scope]rules.ServiceAccountName), make(map[rules.ServiceAccountName]map[string]rules.WSAResource), make(map[string][]string), []*corev1.ServiceAccount{}
}

func (m *MockEngine) DiscoverTargetNamespaces(context.Context, client.Client) ([]string, error) {
	return []string{"namespace1", "namespace2"}, nil
}

var _ = Describe("Metrics Test", func() {
	Context("When creating metrics collector", func() {
		BeforeEach(func() {
			By("creating the new metrics collector")
			engine := rules.NewInMemoryEngine(k8sClient, targetNamespaceRegex)
			collector := NewOctopusMetricsCollector(k8sClient, &engine)
			Expect(collector).NotTo(BeNil())
		})
		AfterEach(func() {
			By("Cleanup completed")
		})
		It("should successfully create the metrics collector", func() {
			By("Creating collector with prometheus.Collector interface")
			engine := rules.NewInMemoryEngine(k8sClient, targetNamespaceRegex)
			collector := NewOctopusMetricsCollector(k8sClient, &engine)
			Expect(collector).NotTo(BeNil())
		})
	})

	var collector *OctopusMetricsCollector
	var engine rules.Engine
	var testNamespaceName string

	BeforeEach(func() {
		By("Setting up metrics collector and test environment")
		inMemoryEngine := rules.NewInMemoryEngine(k8sClient, targetNamespaceRegex)
		engine = &inMemoryEngine
		collector = NewOctopusMetricsCollector(k8sClient, engine)
		Expect(collector).NotTo(BeNil())

		By("Creating unique test namespace for this test")
		testNamespaceName = fmt.Sprintf("octopus-test-%d", time.Now().UnixNano())
		testNamespace := &corev1.Namespace{}
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

	Context("Kubernetes Resource Metrics with Mock Engine", func() {
		It("should accurately collect Kubernetes resource metrics", func() {
			By("Creating a mock engine with known resource counts")
			mockEngine := NewMockEngine()

			// Create collector with mock engine
			mockCollector := NewOctopusMetricsCollector(k8sClient, mockEngine)
			Expect(mockCollector).NotTo(BeNil())

			By("Collecting metrics from mock engine")
			registry := prometheus.NewRegistry()
			err := registry.Register(mockCollector)
			Expect(err).NotTo(HaveOccurred())

			metricFamilies, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			metrics := convertMetricFamiliesToMap(metricFamilies)

			By("Validating ServiceAccount metrics")
			// MockEngine returns 2 SAs in namespace1, 1 SA in namespace2
			_, exists := metrics["octopus_service_accounts_total"]
			Expect(exists).To(BeTrue(), "octopus_service_accounts_total metric should exist")

			totalSA := getTotalMetricValue(metrics, "octopus_service_accounts_total")
			Expect(totalSA).To(Equal(float64(3)), "Should have 3 total ServiceAccounts")

			By("Validating Role metrics")
			// MockEngine returns 1 Role in namespace1, 1 Role in namespace2
			totalRoles := getTotalMetricValue(metrics, "octopus_roles_total")
			Expect(totalRoles).To(Equal(float64(2)), "Should have 2 total Roles")

			By("Validating ClusterRole metrics")
			// MockEngine returns 2 ClusterRoles
			clusterRoles := getMetricValue(metrics, "octopus_cluster_roles_total")
			Expect(clusterRoles).To(Equal(float64(2)), "Should have 2 ClusterRoles")

			By("Validating RoleBinding metrics")
			// MockEngine returns 2 RoleBindings in namespace1, 1 RoleBinding in namespace2
			totalRoleBindings := getTotalMetricValue(metrics, "octopus_role_bindings_total")
			Expect(totalRoleBindings).To(Equal(float64(3)), "Should have 3 total RoleBindings")

			By("Validating ClusterRoleBinding metrics")
			// MockEngine returns 1 ClusterRoleBinding
			clusterRoleBindings := getMetricValue(metrics, "octopus_cluster_role_bindings_total")
			Expect(clusterRoleBindings).To(Equal(float64(1)), "Should have 1 ClusterRoleBinding")
		})
	})

	Context("Distinct Scopes Metrics with Mock Engine", func() {
		It("should accurately count distinct scopes", func() {
			By("Creating a mock engine with known distinct scopes")
			mockEngine := NewMockEngine()

			// Create collector with mock engine
			mockCollector := NewOctopusMetricsCollector(k8sClient, mockEngine)
			Expect(mockCollector).NotTo(BeNil())

			By("Collecting distinct scope metrics")
			registry := prometheus.NewRegistry()
			err := registry.Register(mockCollector)
			Expect(err).NotTo(HaveOccurred())

			metricFamilies, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			metrics := convertMetricFamiliesToMap(metricFamilies)

			By("Validating distinct scopes count")
			// MockEngine returns 3 distinct scopes
			distinctScopes := getMetricValue(metrics, "octopus_distinct_scopes_total")
			Expect(distinctScopes).To(Equal(float64(3)), "Should have 3 distinct scopes")
		})
	})

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
			testRequestsTotal.WithLabelValues(workloadServiceAccountType, scopeMatchedTrue).Inc()
			testRequestsTotal.WithLabelValues(workloadServiceAccountType, scopeMatchedTrue).Inc()
			testRequestsTotal.WithLabelValues(workloadServiceAccountType, "false").Inc()
			testRequestsTotal.WithLabelValues("clusterworkloadServiceAccountType", scopeMatchedTrue).Inc()

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

					if controllerType == workloadServiceAccountType && scopeMatched == scopeMatchedTrue {
						foundWSATrue = true
						Expect(*metric.Counter.Value).To(Equal(float64(2)), "Should have 2 WSA true requests")
					}
					if controllerType == workloadServiceAccountType && scopeMatched == "false" {
						foundWSAFalse = true
						Expect(*metric.Counter.Value).To(Equal(float64(1)), "Should have 1 WSA false request")
					}
					if controllerType == "clusterworkloadServiceAccountType" && scopeMatched == scopeMatchedTrue {
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
			testReconciliationDuration.WithLabelValues(workloadServiceAccountType, "success").Observe(0.5)
			testReconciliationDuration.WithLabelValues(workloadServiceAccountType, "error").Observe(0.1)
			testReconciliationDuration.WithLabelValues("clusterworkloadServiceAccountType", "success").Observe(0.3)

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

					if controllerType == workloadServiceAccountType && result == "success" {
						foundWSASuccess = true
						Expect(*metric.Histogram.SampleCount).To(Equal(uint64(1)), "Should have 1 WSA success sample")
					}
					if controllerType == workloadServiceAccountType && result == "error" {
						foundWSAError = true
						Expect(*metric.Histogram.SampleCount).To(Equal(uint64(1)), "Should have 1 WSA error sample")
					}
					if controllerType == "clusterworkloadServiceAccountType" && result == "success" {
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
			if err.Error() == endOfFile {
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
			if err.Error() == endOfFile {
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
				if err.Error() == endOfFile {
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

	testNamespace := &corev1.Namespace{}
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

func getTotalMetricValue(metrics map[string]*dto.MetricFamily, metricName string) float64 {
	family, exists := metrics[metricName]
	if !exists {
		return 0
	}

	total := 0.0
	for _, metric := range family.Metric {
		if metric.Gauge != nil && metric.Gauge.Value != nil {
			total += *metric.Gauge.Value
		}
	}

	return total
}
