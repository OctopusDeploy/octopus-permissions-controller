package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// WSA and CWSA metrics
	wsaTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "octopus_wsa_total",
			Help: "Total number of WorkloadServiceAccounts",
		},
		[]string{"namespace"},
	)

	cwsaTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_cwsa_total",
			Help: "Total number of ClusterWorkloadServiceAccounts",
		},
	)

	// ServiceAccount metrics
	serviceAccountsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "octopus_service_accounts_total",
			Help: "Total number of Service Accounts managed by Octopus",
		},
		[]string{"namespace"},
	)

	// Role metrics
	rolesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "octopus_roles_total",
			Help: "Total number of Roles managed by Octopus",
		},
		[]string{"namespace"},
	)

	clusterRolesTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_cluster_roles_total",
			Help: "Total number of ClusterRoles managed by Octopus",
		},
	)

	// RoleBinding metrics
	roleBindingsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "octopus_role_bindings_total",
			Help: "Total number of RoleBindings managed by Octopus",
		},
		[]string{"namespace"},
	)

	clusterRoleBindingsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_cluster_role_bindings_total",
			Help: "Total number of ClusterRoleBindings managed by Octopus",
		},
	)

	// Scope metrics
	distinctScopesTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_distinct_scopes_total",
			Help: "Total number of distinct scopes",
		},
	)

	scopesWithProjectsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_scopes_with_projects_total",
			Help: "Number of scopes with Project defined",
		},
	)

	scopesWithEnvironmentsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_scopes_with_environments_total",
			Help: "Number of scopes with Environment defined",
		},
	)

	scopesWithTenantsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_scopes_with_tenants_total",
			Help: "Number of scopes with Tenant defined",
		},
	)

	scopesWithStepsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_scopes_with_steps_total",
			Help: "Number of scopes with Step defined",
		},
	)

	scopesWithSpacesTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octopus_scopes_with_spaces_total",
			Help: "Number of scopes with Space defined",
		},
	)

	// Agent metrics
	discoveredAgentsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "octopus_discovered_agents_total",
			Help: "Number of discovered agents",
		},
		[]string{"agent_type"},
	)

	// Request metrics
	requestsServedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "octopus_requests_served_total",
			Help: "Total number of reconciliation requests served",
		},
		[]string{"controller_type", "result"},
	)

	requestsScopeMatchedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "octopus_requests_scope_matched_total",
			Help: "Number of requests where the scope matched",
		},
		[]string{"controller_type"},
	)

	requestsScopeNotMatchedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "octopus_requests_scope_not_matched_total",
			Help: "Number of requests where the scope didn't match",
		},
		[]string{"controller_type"},
	)

	reconciliationDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "octopus_reconciliation_duration_seconds",
			Help:    "Time taken to complete a reconciliation request",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller_type", "result"},
	)

	// Version information metric
	versionInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "octopus_version_info",
			Help: "Version information about the Octopus Permissions Controller",
		},
		[]string{"version"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		wsaTotal,
		cwsaTotal,
		serviceAccountsTotal,
		rolesTotal,
		clusterRolesTotal,
		roleBindingsTotal,
		clusterRoleBindingsTotal,
		distinctScopesTotal,
		scopesWithProjectsTotal,
		scopesWithEnvironmentsTotal,
		scopesWithTenantsTotal,
		scopesWithStepsTotal,
		scopesWithSpacesTotal,
		discoveredAgentsTotal,
		requestsServedTotal,
		requestsScopeMatchedTotal,
		requestsScopeNotMatchedTotal,
		reconciliationDurationSeconds,
		versionInfo,
	)
}

// WSA and CWSA metrics functions
func SetWSATotal(namespace string, count float64) {
	wsaTotal.WithLabelValues(namespace).Set(count)
}

func SetCWSATotal(count float64) {
	cwsaTotal.Set(count)
}

// ServiceAccount metrics functions
func SetServiceAccountsTotal(namespace string, count float64) {
	serviceAccountsTotal.WithLabelValues(namespace).Set(count)
}

// Role metrics functions
func SetRolesTotal(namespace string, count float64) {
	rolesTotal.WithLabelValues(namespace).Set(count)
}

func SetClusterRolesTotal(count float64) {
	clusterRolesTotal.Set(count)
}

// RoleBinding metrics functions
func SetRoleBindingsTotal(namespace string, count float64) {
	roleBindingsTotal.WithLabelValues(namespace).Set(count)
}

func SetClusterRoleBindingsTotal(count float64) {
	clusterRoleBindingsTotal.Set(count)
}

// Scope metrics functions
func SetDistinctScopesTotal(count float64) {
	distinctScopesTotal.Set(count)
}

func SetScopesWithProjectsTotal(count float64) {
	scopesWithProjectsTotal.Set(count)
}

func SetScopesWithEnvironmentsTotal(count float64) {
	scopesWithEnvironmentsTotal.Set(count)
}

func SetScopesWithTenantsTotal(count float64) {
	scopesWithTenantsTotal.Set(count)
}

func SetScopesWithStepsTotal(count float64) {
	scopesWithStepsTotal.Set(count)
}

func SetScopesWithSpacesTotal(count float64) {
	scopesWithSpacesTotal.Set(count)
}

// Agent metrics functions
func SetDiscoveredAgentsTotal(agentType string, count float64) {
	discoveredAgentsTotal.WithLabelValues(agentType).Set(count)
}

// Request metrics functions
func IncRequestsServed(controllerType, result string) {
	requestsServedTotal.WithLabelValues(controllerType, result).Inc()
}

func IncRequestsScopeMatched(controllerType string) {
	requestsScopeMatchedTotal.WithLabelValues(controllerType).Inc()
}

func IncRequestsScopeNotMatched(controllerType string) {
	requestsScopeNotMatchedTotal.WithLabelValues(controllerType).Inc()
}

func ObserveReconciliationDuration(controllerType, result string, duration float64) {
	reconciliationDurationSeconds.WithLabelValues(controllerType, result).Observe(duration)
}

func RecordReconciliationDurationFunc(controllerType string, startTime time.Time) {
	duration := time.Since(startTime).Seconds()
	if err := recover(); err != nil {
		ObserveReconciliationDuration(controllerType, "error", duration)
		IncRequestsServed(controllerType, "error")
		panic(err) // Re-panic to maintain original behavior
	}
	IncRequestsServed(controllerType, "success")
	ObserveReconciliationDuration(controllerType, "success", duration)
}