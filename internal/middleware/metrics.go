package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the Aegis proxy.
// Metrics are registered automatically via promauto and use the "aegis_"
// namespace prefix for consistent naming.
type Metrics struct {
	// RequestsTotal counts total requests, labeled by target host, HTTP
	// method, and response status code.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration observes the duration of each proxied request in
	// seconds, labeled by target host and HTTP method.
	RequestDuration *prometheus.HistogramVec

	// CacheHitsTotal counts the number of responses served from cache
	// (X-Aegis-Cache: HIT).
	CacheHitsTotal prometheus.Counter

	// CacheMissesTotal counts the number of responses that were not
	// served from cache (X-Aegis-Cache: MISS).
	CacheMissesTotal prometheus.Counter

	// InFlightRequests tracks the number of requests currently being
	// processed by the proxy.
	InFlightRequests prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics for the Aegis proxy.
// All metrics are registered with the default prometheus registry via promauto.
func NewMetrics() *Metrics {
	return &Metrics{
		RequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "aegis",
			Name:      "requests_total",
			Help:      "Total number of proxied HTTP requests.",
		}, []string{"host", "method", "status_code"}),

		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "aegis",
			Name:      "request_duration_seconds",
			Help:      "Duration of proxied HTTP requests in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"host", "method"}),

		CacheHitsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "aegis",
			Name:      "cache_hits_total",
			Help:      "Total number of responses served from cache.",
		}),

		CacheMissesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "aegis",
			Name:      "cache_misses_total",
			Help:      "Total number of cache misses.",
		}),

		InFlightRequests: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "aegis",
			Name:      "in_flight_requests",
			Help:      "Number of HTTP requests currently being processed.",
		}),
	}
}

// NewMetricsMiddleware creates a [Middleware] that records Prometheus metrics
// for every proxied request. It tracks request counts, durations, in-flight
// concurrency, and cache hit/miss rates.
//
// The middleware inspects the X-Aegis-Cache response header set by the cache
// middleware to determine whether a response was served from cache.
func NewMetricsMiddleware(m *Metrics) Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			host := req.URL.Host
			method := req.Method

			m.InFlightRequests.Inc()
			start := time.Now()

			resp, err := next.RoundTrip(req)

			duration := time.Since(start).Seconds()
			m.InFlightRequests.Dec()

			if err != nil {
				// Record a failed request with status "error".
				m.RequestsTotal.WithLabelValues(host, method, "error").Inc()
				m.RequestDuration.WithLabelValues(host, method).Observe(duration)
				return nil, err
			}

			statusCode := strconv.Itoa(resp.StatusCode)
			m.RequestsTotal.WithLabelValues(host, method, statusCode).Inc()
			m.RequestDuration.WithLabelValues(host, method).Observe(duration)

			// Check the X-Aegis-Cache header to track cache performance.
			switch resp.Header.Get("X-Aegis-Cache") {
			case "HIT":
				m.CacheHitsTotal.Inc()
			case "MISS":
				m.CacheMissesTotal.Inc()
			}

			return resp, nil
		})
	}
}
