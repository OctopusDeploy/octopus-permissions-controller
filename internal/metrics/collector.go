package metrics

import (
	"context"
	"fmt"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// OctopusMetricsCollector implements the prometheus.Collector interface
// to provide up-to-date metrics when Prometheus scrapes the endpoint.
type OctopusMetricsCollector struct {
	client client.Client
	engine rules.Engine

	// Metric descriptors
	wsaTotalDesc                     *prometheus.Desc
	cwsaTotalDesc                    *prometheus.Desc
	serviceAccountsTotalDesc         *prometheus.Desc
	rolesTotalDesc                   *prometheus.Desc
	clusterRolesTotalDesc            *prometheus.Desc
	roleBindingsTotalDesc            *prometheus.Desc
	clusterRoleBindingsTotalDesc     *prometheus.Desc
	distinctScopesTotalDesc          *prometheus.Desc
	scopesWithProjectsTotalDesc      *prometheus.Desc
	scopesWithProjectGroupsTotalDesc *prometheus.Desc
	scopesWithEnvironmentsTotalDesc  *prometheus.Desc
	scopesWithTenantsTotalDesc       *prometheus.Desc
	scopesWithStepsTotalDesc         *prometheus.Desc
	scopesWithSpacesTotalDesc        *prometheus.Desc
	targetNamespacesTotalDesc        *prometheus.Desc
}

// NewOctopusMetricsCollector creates a new instance of OctopusMetricsCollector
func NewOctopusMetricsCollector(cli client.Client, eng rules.Engine) *OctopusMetricsCollector {
	return &OctopusMetricsCollector{
		client: cli,
		engine: eng,

		wsaTotalDesc: prometheus.NewDesc(
			"octopus_wsa_total",
			"Total number of WorkloadServiceAccounts",
			[]string{"namespace"},
			nil,
		),
		cwsaTotalDesc: prometheus.NewDesc(
			"octopus_cwsa_total",
			"Total number of ClusterWorkloadServiceAccounts",
			nil,
			nil,
		),
		serviceAccountsTotalDesc: prometheus.NewDesc(
			"octopus_service_accounts_total",
			"Total number of Service Accounts managed by Octopus",
			[]string{"namespace"},
			nil,
		),
		rolesTotalDesc: prometheus.NewDesc(
			"octopus_roles_total",
			"Total number of Roles managed by Octopus",
			[]string{"namespace"},
			nil,
		),
		clusterRolesTotalDesc: prometheus.NewDesc(
			"octopus_cluster_roles_total",
			"Total number of ClusterRoles managed by Octopus",
			nil,
			nil,
		),
		roleBindingsTotalDesc: prometheus.NewDesc(
			"octopus_role_bindings_total",
			"Total number of RoleBindings managed by Octopus",
			[]string{"namespace"},
			nil,
		),
		clusterRoleBindingsTotalDesc: prometheus.NewDesc(
			"octopus_cluster_role_bindings_total",
			"Total number of ClusterRoleBindings managed by Octopus",
			nil,
			nil,
		),
		distinctScopesTotalDesc: prometheus.NewDesc(
			"octopus_distinct_scopes_total",
			"Total number of distinct scopes",
			nil,
			nil,
		),
		scopesWithProjectsTotalDesc: prometheus.NewDesc(
			"octopus_scopes_with_projects_total",
			"Number of scopes with Project defined",
			nil,
			nil,
		),
		scopesWithProjectGroupsTotalDesc: prometheus.NewDesc(
			"octopus_scopes_with_project_groups_total",
			"Number of scopes with Project Group defined",
			nil,
			nil,
		),
		scopesWithEnvironmentsTotalDesc: prometheus.NewDesc(
			"octopus_scopes_with_environments_total",
			"Number of scopes with Environment defined",
			nil,
			nil,
		),
		scopesWithTenantsTotalDesc: prometheus.NewDesc(
			"octopus_scopes_with_tenants_total",
			"Number of scopes with Tenant defined",
			nil,
			nil,
		),
		scopesWithStepsTotalDesc: prometheus.NewDesc(
			"octopus_scopes_with_steps_total",
			"Number of scopes with Step defined",
			nil,
			nil,
		),
		scopesWithSpacesTotalDesc: prometheus.NewDesc(
			"octopus_scopes_with_spaces_total",
			"Number of scopes with Space defined",
			nil,
			nil,
		),
		targetNamespacesTotalDesc: prometheus.NewDesc(
			"octopus_target_namespaces_total",
			"Number of target namespaces",
			nil,
			nil,
		),
	}
}

// Describe implements the prometheus.Collector interface
// It sends the descriptors of all the metrics the collector will provide
func (c *OctopusMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.wsaTotalDesc
	ch <- c.cwsaTotalDesc
	ch <- c.serviceAccountsTotalDesc
	ch <- c.rolesTotalDesc
	ch <- c.clusterRolesTotalDesc
	ch <- c.roleBindingsTotalDesc
	ch <- c.clusterRoleBindingsTotalDesc
	ch <- c.distinctScopesTotalDesc
	ch <- c.scopesWithProjectsTotalDesc
	ch <- c.scopesWithProjectGroupsTotalDesc
	ch <- c.scopesWithEnvironmentsTotalDesc
	ch <- c.scopesWithTenantsTotalDesc
	ch <- c.scopesWithStepsTotalDesc
	ch <- c.scopesWithSpacesTotalDesc
	ch <- c.targetNamespacesTotalDesc
}

// Collect implements the prometheus.Collector interface
// It is called by Prometheus when it scrapes the metrics endpoint
// This method collects current metrics and sends them to the provided channel
func (c *OctopusMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	if err := c.collectWSAMetrics(ctx, ch); err != nil {
		logger.Error(err, "Failed to collect WSA metrics")
	}

	if err := c.collectCWSAMetrics(ctx, ch); err != nil {
		logger.Error(err, "Failed to collect CWSA metrics")
	}

	if err := c.collectServiceAccountMetrics(ctx, ch); err != nil {
		logger.Error(err, "Failed to collect ServiceAccount metrics")
	}

	if err := c.collectRoleMetrics(ctx, ch); err != nil {
		logger.Error(err, "Failed to collect Role metrics")
	}

	if err := c.collectRoleBindingMetrics(ctx, ch); err != nil {
		logger.Error(err, "Failed to collect RoleBinding metrics")
	}

	if err := c.collectScopeMetricsFromResources(ctx, ch); err != nil {
		logger.Error(err, "Failed to collect scope metrics from resources")
	}

	c.collectTargetNamespaceMetrics(ch)
}

// collectWSAMetrics collects WorkloadServiceAccount metrics
func (c *OctopusMetricsCollector) collectWSAMetrics(ctx context.Context, ch chan<- prometheus.Metric) error {
	wsaEnumerable, err := c.engine.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get workload service accounts: %w", err)
	}

	namespaceCounts := make(map[string]int)
	for wsa := range wsaEnumerable {
		namespaceCounts[wsa.GetNamespace()]++
	}

	for namespace, count := range namespaceCounts {
		metric := prometheus.MustNewConstMetric(
			c.wsaTotalDesc,
			prometheus.GaugeValue,
			float64(count),
			namespace,
		)
		ch <- metric
	}

	return nil
}

func (c *OctopusMetricsCollector) collectCWSAMetrics(ctx context.Context, ch chan<- prometheus.Metric) error {
	cwsaEnumerable, err := c.engine.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster workload service accounts: %w", err)
	}

	count := 0
	for range cwsaEnumerable {
		count++
	}

	metric := prometheus.MustNewConstMetric(
		c.cwsaTotalDesc,
		prometheus.GaugeValue,
		float64(count),
	)
	ch <- metric

	return nil
}

func (c *OctopusMetricsCollector) collectServiceAccountMetrics(ctx context.Context, ch chan<- prometheus.Metric) error {
	saEnumerable, err := c.engine.GetServiceAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service accounts: %w", err)
	}

	namespaceCounts := make(map[string]int)
	for sa := range saEnumerable {
		namespaceCounts[sa.Namespace]++
	}

	for namespace, count := range namespaceCounts {
		metric := prometheus.MustNewConstMetric(
			c.serviceAccountsTotalDesc,
			prometheus.GaugeValue,
			float64(count),
			namespace,
		)
		ch <- metric
	}

	return nil
}

func (c *OctopusMetricsCollector) collectRoleMetrics(ctx context.Context, ch chan<- prometheus.Metric) error {
	roleEnumerable, err := c.engine.GetRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to get roles: %w", err)
	}

	namespaceCounts := make(map[string]int)
	for role := range roleEnumerable {
		namespaceCounts[role.Namespace]++
	}

	for namespace, count := range namespaceCounts {
		metric := prometheus.MustNewConstMetric(
			c.rolesTotalDesc,
			prometheus.GaugeValue,
			float64(count),
			namespace,
		)
		ch <- metric
	}

	clusterRoleEnumerable, err := c.engine.GetClusterRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster roles: %w", err)
	}

	clusterRoleCount := 0
	for range clusterRoleEnumerable {
		clusterRoleCount++
	}

	metric := prometheus.MustNewConstMetric(
		c.clusterRolesTotalDesc,
		prometheus.GaugeValue,
		float64(clusterRoleCount),
	)
	ch <- metric

	return nil
}

func (c *OctopusMetricsCollector) collectRoleBindingMetrics(ctx context.Context, ch chan<- prometheus.Metric) error {

	roleBindingEnumerable, err := c.engine.GetRoleBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get role bindings: %w", err)
	}

	namespaceCounts := make(map[string]int)
	for rb := range roleBindingEnumerable {
		namespaceCounts[rb.Namespace]++
	}

	for namespace, count := range namespaceCounts {
		metric := prometheus.MustNewConstMetric(
			c.roleBindingsTotalDesc,
			prometheus.GaugeValue,
			float64(count),
			namespace,
		)
		ch <- metric
	}

	clusterRoleBindingEnumerable, err := c.engine.GetClusterRoleBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster role bindings: %w", err)
	}

	clusterRoleBindingCount := 0
	for range clusterRoleBindingEnumerable {
		clusterRoleBindingCount++
	}

	metric := prometheus.MustNewConstMetric(
		c.clusterRoleBindingsTotalDesc,
		prometheus.GaugeValue,
		float64(clusterRoleBindingCount),
	)
	ch <- metric

	return nil
}

// collectScopeMetricsFromResources collects scope metrics from user-defined WSA/CWSA resources
func (c *OctopusMetricsCollector) collectScopeMetricsFromResources(ctx context.Context, ch chan<- prometheus.Metric) error {
	// Collect WSA resources
	wsaList := &v1beta1.WorkloadServiceAccountList{}
	if err := c.client.List(ctx, wsaList); err != nil {
		return fmt.Errorf("failed to list workload service accounts: %w", err)
	}

	// Collect CWSA resources
	cwsaList := &v1beta1.ClusterWorkloadServiceAccountList{}
	if err := c.client.List(ctx, cwsaList); err != nil {
		return fmt.Errorf("failed to list cluster workload service accounts: %w", err)
	}

	scopeToSAMap := c.engine.GetScopeToSA()
	distinctScopesCount := len(scopeToSAMap)
	distinctScopesMetric := prometheus.MustNewConstMetric(
		c.distinctScopesTotalDesc,
		prometheus.GaugeValue,
		float64(distinctScopesCount),
	)
	ch <- distinctScopesMetric

	var withProjects, withProjectGroups, withEnvironments, withTenants, withSteps, withSpaces int

	// Process WSA resources
	for _, wsa := range wsaList.Items {
		if len(wsa.Spec.Scope.Projects) > 0 {
			withProjects++
		}
		if len(wsa.Spec.Scope.ProjectGroups) > 0 {
			withProjectGroups++
		}
		if len(wsa.Spec.Scope.Environments) > 0 {
			withEnvironments++
		}
		if len(wsa.Spec.Scope.Tenants) > 0 {
			withTenants++
		}
		if len(wsa.Spec.Scope.Steps) > 0 {
			withSteps++
		}
		if len(wsa.Spec.Scope.Spaces) > 0 {
			withSpaces++
		}
	}

	// Process CWSA resources
	for _, cwsa := range cwsaList.Items {
		if len(cwsa.Spec.Scope.Projects) > 0 {
			withProjects++
		}
		if len(cwsa.Spec.Scope.ProjectGroups) > 0 {
			withProjectGroups++
		}
		if len(cwsa.Spec.Scope.Environments) > 0 {
			withEnvironments++
		}
		if len(cwsa.Spec.Scope.Tenants) > 0 {
			withTenants++
		}
		if len(cwsa.Spec.Scope.Steps) > 0 {
			withSteps++
		}
		if len(cwsa.Spec.Scope.Spaces) > 0 {
			withSpaces++
		}
	}

	// Create and send scope metrics
	scopeMetrics := []struct {
		desc  *prometheus.Desc
		value int
	}{
		{c.scopesWithProjectsTotalDesc, withProjects},
		{c.scopesWithProjectGroupsTotalDesc, withProjectGroups},
		{c.scopesWithEnvironmentsTotalDesc, withEnvironments},
		{c.scopesWithTenantsTotalDesc, withTenants},
		{c.scopesWithStepsTotalDesc, withSteps},
		{c.scopesWithSpacesTotalDesc, withSpaces},
	}

	for _, scopeMetric := range scopeMetrics {
		metric := prometheus.MustNewConstMetric(
			scopeMetric.desc,
			prometheus.GaugeValue,
			float64(scopeMetric.value),
		)
		ch <- metric
	}

	return nil
}

func (c *OctopusMetricsCollector) collectTargetNamespaceMetrics(ch chan<- prometheus.Metric) {
	targetNamespaces := c.engine.GetTargetNamespaces()
	count := len(targetNamespaces)

	metric := prometheus.MustNewConstMetric(
		c.targetNamespacesTotalDesc,
		prometheus.GaugeValue,
		float64(count),
	)
	ch <- metric
}

func (c *OctopusMetricsCollector) TrackRequest(controllerType string, scopeMatched bool) {
	IncRequestsTotal(controllerType, scopeMatched)
}
