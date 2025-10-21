package metrics

import (
	"context"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type MetricsCollector struct {
	client client.Client
}

func NewMetricsCollector(cli client.Client) *MetricsCollector {
	return &MetricsCollector{
		client: cli,
	}
}

func (mc *MetricsCollector) CollectResourceMetrics(ctx context.Context) error {
	logger := log.FromContext(ctx)

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
	wsaList := &v1beta1.WorkloadServiceAccountList{}
	if err := mc.client.List(ctx, wsaList); err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for _, wsa := range wsaList.Items {
		namespaceCounts[wsa.Namespace]++
	}

	for namespace, count := range namespaceCounts {
		SetWSATotal(namespace, float64(count))
	}

	return nil
}

func (mc *MetricsCollector) collectCWSAMetrics(ctx context.Context) error {
	cwsaList := &v1beta1.ClusterWorkloadServiceAccountList{}
	if err := mc.client.List(ctx, cwsaList); err != nil {
		return err
	}

	SetCWSATotal(float64(len(cwsaList.Items)))
	return nil
}

func (mc *MetricsCollector) collectServiceAccountMetrics(ctx context.Context) error {
	saList := &corev1.ServiceAccountList{}
	if err := mc.client.List(ctx, saList); err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for _, sa := range saList.Items {
		if isOctopusManaged(sa.Labels) {
			namespaceCounts[sa.Namespace]++
		}
	}

	for namespace, count := range namespaceCounts {
		SetServiceAccountsTotal(namespace, float64(count))
	}

	return nil
}

func (mc *MetricsCollector) collectRoleMetrics(ctx context.Context) error {

	roleList := &rbacv1.RoleList{}
	if err := mc.client.List(ctx, roleList); err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for _, role := range roleList.Items {
		if isOctopusManaged(role.Labels) {
			namespaceCounts[role.Namespace]++
		}
	}

	for namespace, count := range namespaceCounts {
		SetRolesTotal(namespace, float64(count))
	}

	clusterRoleList := &rbacv1.ClusterRoleList{}
	if err := mc.client.List(ctx, clusterRoleList); err != nil {
		return err
	}

	clusterRoleCount := 0
	for _, clusterRole := range clusterRoleList.Items {
		if isOctopusManaged(clusterRole.Labels) {
			clusterRoleCount++
		}
	}

	SetClusterRolesTotal(float64(clusterRoleCount))
	return nil
}

func (mc *MetricsCollector) collectRoleBindingMetrics(ctx context.Context) error {

	roleBindingList := &rbacv1.RoleBindingList{}
	if err := mc.client.List(ctx, roleBindingList); err != nil {
		return err
	}

	namespaceCounts := make(map[string]int)
	for _, rb := range roleBindingList.Items {
		if isOctopusManaged(rb.Labels) {
			namespaceCounts[rb.Namespace]++
		}
	}

	for namespace, count := range namespaceCounts {
		SetRoleBindingsTotal(namespace, float64(count))
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	if err := mc.client.List(ctx, clusterRoleBindingList); err != nil {
		return err
	}

	clusterRoleBindingCount := 0
	for _, crb := range clusterRoleBindingList.Items {
		if isOctopusManaged(crb.Labels) {
			clusterRoleBindingCount++
		}
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

func (mc *MetricsCollector) CollectScopeMetrics(scopes []Scope) {
	distinctScopes := len(scopes)
	SetDistinctScopesTotal(float64(distinctScopes))

	var withProjects, withEnvironments, withTenants, withSteps, withSpaces int
	for _, scope := range scopes {
		if scope.Project != "" {
			withProjects++
		}
		if scope.Environment != "" {
			withEnvironments++
		}
		if scope.Tenant != "" {
			withTenants++
		}
		if scope.Step != "" {
			withSteps++
		}
		if scope.Space != "" {
			withSpaces++
		}
	}

	SetScopesWithProjectsTotal(float64(withProjects))
	SetScopesWithEnvironmentsTotal(float64(withEnvironments))
	SetScopesWithTenantsTotal(float64(withTenants))
	SetScopesWithStepsTotal(float64(withSteps))
	SetScopesWithSpacesTotal(float64(withSpaces))
}

func (mc *MetricsCollector) TrackScopeMatching(controllerType string, matchedScopesCount int, totalWSAResources int) {

	matchedScopes := matchedScopesCount

	if matchedScopes > 0 {
		// Increment matched scopes count
		for i := 0; i < matchedScopes; i++ {
			IncRequestsScopeMatched(controllerType)
		}
	}

	unmatchedCount := totalWSAResources - matchedScopes
	if unmatchedCount > 0 {
		for i := 0; i < unmatchedCount; i++ {
			IncRequestsScopeNotMatched(controllerType)
		}
	}
}

// isOctopusManaged checks if a resource is managed by the Octopus controller
func isOctopusManaged(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	managedBy, exists := labels["app.kubernetes.io/managed-by"]
	if exists && managedBy == "octopus-permissions-controller" {
		return true
	}
	// Check for octopus-specific labels
	_, octopusLabel := labels["octopus.com/managed"]
	return octopusLabel
}
