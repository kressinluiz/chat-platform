package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

var messagesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "chat_messages_total",
		Help: "Total number of messages broadcast",
	},
	[]string{"room_id"},
)

var wsUpgradeFailures = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "websocket_upgrade_failures_total",
		Help: "Total number of failed WebSocket upgrade attempts",
	},
)

var httpRequests = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	},
	[]string{"method", "endpoint"},
)

var requestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"method", "endpoint"},
)

func PrometheusMiddleware(next http.Handler, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequests.WithLabelValues(r.Method, path).Inc()
		timer := prometheus.NewTimer(requestDuration.WithLabelValues(r.Method, path))
		defer timer.ObserveDuration()
		next.ServeHTTP(w, r)
	})
}

func NewActiveConnectionsMetric(hub *Hub) prometheus.GaugeFunc {
	return prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "active_websocket_connections",
			Help: "Current number of active WebSocket connections",
		},
		func() float64 {
			return float64(hub.ConnectedClients())
		},
	)
}
