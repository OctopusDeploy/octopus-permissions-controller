package metrics

import (
	"context"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type MetricsCollector struct {
	client client.Client
	engine rules.Engine
}

func NewMetricsCollector(cli client.Client) *MetricsCollector {
	return &MetricsCollector{
		client: cli,
	}
}

func (mc *MetricsCollector) CollectResourceMetrics(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Collect scope metrics from user-defined WSA/CWSA resources
	if err := mc.collectScopeMetricsFromResources(ctx); err != nil {
		logger.Error(err, "Failed to collect scope metrics from resources")
		return err
	}

	if err := mc.collectWSAMetrics(ctx); err != nil {
		logger.Error(err, "Failed to collect WSA metrics")
		return err
	}

	if err := mc.collectCWSAMetrics(ctx); err != nil {
		logger.Error(err, "Failed to collect CWSA metrics")
		return err
	}

	if err := mc.collectServiceAccountMetrics(ctx); err != nil {
		logger.Error(err, "Failed to collect ServiceAccount metrics")
		return err
	}

	if err := mc.collectRoleMetrics(ctx); err != nil {
		logger.Error(err, "Failed to collect Role metrics")
		return err
	}

	if err := mc.collectRoleBindingMetrics(ctx); err != nil {
		logger.Error(err, "Failed to collect RoleBinding metrics")
		return err
	}

	return nil
}

func (mc *MetricsCollector) collectWSAMetrics(ctx context.Context) error {
	wsaEnumerable, err := mc.engine.GetWorkloadServiceAccounts(ctx)
	if err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for wsa := range wsaEnumerable {
		namespaceCounts[wsa.GetNamespace()]++
	}

	for namespace, count := range namespaceCounts {
		SetWSATotal(namespace, float64(count))
	}

	return nil
}

func (mc *MetricsCollector) collectCWSAMetrics(ctx context.Context) error {
	cwsaEnumerable, err := mc.engine.GetClusterWorkloadServiceAccounts(ctx)
	if err != nil {
		return err
	}

	count := 0
	for range cwsaEnumerable {
		count++
	}

	SetCWSATotal(float64(count))
	return nil
}

func (mc *MetricsCollector) collectServiceAccountMetrics(ctx context.Context) error {
	saEnumerable, err := mc.engine.GetServiceAccounts(ctx)
	if err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for sa := range saEnumerable {
		namespaceCounts[sa.Namespace]++
	}

	for namespace, count := range namespaceCounts {
		SetServiceAccountsTotal(namespace, float64(count))
	}

	return nil
}

func (mc *MetricsCollector) collectRoleMetrics(ctx context.Context) error {
	roleEnumerable, err := mc.engine.GetRoles(ctx)
	if err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for role := range roleEnumerable {
		namespaceCounts[role.Namespace]++
	}

	for namespace, count := range namespaceCounts {
		SetRolesTotal(namespace, float64(count))
	}

	clusterRoleEnumerable, err := mc.engine.GetClusterRoles(ctx)
	if err != nil {
		return err
	}

	clusterRoleCount := 0
	for range clusterRoleEnumerable {
		clusterRoleCount++
	}

	SetClusterRolesTotal(float64(clusterRoleCount))
	return nil
}

func (mc *MetricsCollector) collectRoleBindingMetrics(ctx context.Context) error {

	roleBindingEnumerable, err := mc.engine.GetRoleBindings(ctx)
	if err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for rb := range roleBindingEnumerable {
		namespaceCounts[rb.Namespace]++
	}

	for namespace, count := range namespaceCounts {
		SetRoleBindingsTotal(namespace, float64(count))
	}

	clusterRoleBindingEnumerable, err := mc.engine.GetClusterRoleBindings(ctx)
	if err != nil {
		return err
	}

	clusterRoleBindingCount := 0
	for range clusterRoleBindingEnumerable {
		clusterRoleBindingCount++
	}

	SetClusterRoleBindingsTotal(float64(clusterRoleBindingCount))
	return nil
}

type Scope struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Tenant      string `json:"tenant"`
	Step        string `json:"step"`
	Space       string `json:"space"`
}

// collectScopeMetricsFromResources collects scope metrics from user-defined WSA/CWSA resources
func (mc *MetricsCollector) collectScopeMetricsFromResources(ctx context.Context) error {
	// Collect WSA resources
	wsaList := &v1beta1.WorkloadServiceAccountList{}
	if err := mc.client.List(ctx, wsaList); err != nil {
		return err
	}

	// Collect CWSA resources
	cwsaList := &v1beta1.ClusterWorkloadServiceAccountList{}
	if err := mc.client.List(ctx, cwsaList); err != nil {
		return err
	}

	// Count total distinct user-defined resources
	totalResources := len(wsaList.Items) + len(cwsaList.Items)
	SetDistinctScopesTotal(float64(totalResources))

	var withProjects, withEnvironments, withTenants, withSteps, withSpaces int

	// Process WSA resources
	for _, wsa := range wsaList.Items {
		if len(wsa.Spec.Scope.Projects) > 0 {
			withProjects++
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

	SetScopesWithProjectsTotal(float64(withProjects))
	SetScopesWithEnvironmentsTotal(float64(withEnvironments))
	SetScopesWithTenantsTotal(float64(withTenants))
	SetScopesWithStepsTotal(float64(withSteps))
	SetScopesWithSpacesTotal(float64(withSpaces))

	return nil
}

func (mc *MetricsCollector) TrackScopeMatching(controllerType string, matchedScopesCount int) {

	if matchedScopesCount > 0 {
		IncRequestsScopeMatched(controllerType)
	} else {
		IncRequestsScopeNotMatched(controllerType)
	}
}
