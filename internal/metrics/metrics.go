package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "octopus_requests_total",
			Help: "Total number of requests processed with scope matching status",
		},
		[]string{"controller_type", "scope_matched"},
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
		requestsTotal,
		reconciliationDurationSeconds,
		versionInfo,
	)
}

func IncRequestsTotal(requestIdentifier string, scopeMatched bool) {
	scopeMatchedStr := "false"
	if scopeMatched {
		scopeMatchedStr = "true"
	}
	requestsTotal.WithLabelValues(requestIdentifier, scopeMatchedStr).Inc()
}

func ObserveReconciliationDuration(controllerType, result string, duration float64) {
	reconciliationDurationSeconds.WithLabelValues(controllerType, result).Observe(duration)
}

func RecordReconciliationDurationFunc(controllerType string, startTime time.Time) {
	duration := time.Since(startTime).Seconds()
	if err := recover(); err != nil {
		ObserveReconciliationDuration(controllerType, "error", duration)
		panic(err) // Re-panic to maintain original behavior
	}
	ObserveReconciliationDuration(controllerType, "success", duration)
}
