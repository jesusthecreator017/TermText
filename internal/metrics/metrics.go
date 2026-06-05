// Package metrics defines the Prometheus collectors exposed at /metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "termtext_http_requests_total",
		Help: "Total HTTP requests by method, path pattern, and status.",
	}, []string{"method", "path", "status"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "termtext_http_request_duration_seconds",
		Help:    "HTTP request latency by method and path pattern.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	WSConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "termtext_ws_connections",
		Help: "Current number of open WebSocket connections.",
	})

	MessagesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "termtext_messages_total",
		Help: "Total chat messages accepted and persisted.",
	})
)
