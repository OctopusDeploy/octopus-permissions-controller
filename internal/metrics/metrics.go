package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (

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
		requestsServedTotal,
		requestsScopeMatchedTotal,
		requestsScopeNotMatchedTotal,
		reconciliationDurationSeconds,
		versionInfo,
	)
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
