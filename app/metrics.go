package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics holds this server's own Prometheus registry and the two custom
// metrics. A registry per Server (not the global default) means tests can
// build many Servers without "duplicate registration" panics.
type metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newMetrics() *metrics {
	m := &metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests handled, by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds, by method and route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.duration,
	)
	return m
}

func (m *metrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// metricsMethod keeps the method label to a fixed set. Clients can send any
// method string, and every distinct label value becomes a new time series.
func metricsMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

// withMetrics counts each request and records its duration. The route label
// is the matched mux pattern (for example "/db"), never the raw URL, so
// arbitrary paths from scanners collapse into the "/" pattern instead of
// creating one series each. Probes, the preStop hook and /metrics itself are
// not counted: probes fire every few seconds per pod, preStop sleeps on
// purpose, and counting scrapes would only measure Prometheus.
func (s *Server) withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/readyz", "/internal/prestop", "/metrics":
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// The mux sets r.Pattern while it routes, so read it after ServeHTTP.
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		method := metricsMethod(r.Method)
		s.metrics.requests.WithLabelValues(method, route, strconv.Itoa(rec.status)).Inc()
		s.metrics.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	})
}
