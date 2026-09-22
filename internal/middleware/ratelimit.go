package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimitConfig defines rate limit parameters for outbound requests.
type RateLimitConfig struct {
	RequestsPerMinute int // RPM limit (0 = unlimited)
	TokensPerMinute   int // TPM limit (0 = unlimited) — informational, not enforced pre-request
	BurstSize         int // Burst allowance (defaults to RPM/10 if 0)
}

// NewRateLimitMiddleware creates a per-host rate limiting middleware. Each
// upstream host receives its own token-bucket limiter derived from the default
// config. Requests that exceed the rate limit receive a synthetic 429 Too Many
// Requests response.
func NewRateLimitMiddleware(defaultConfig RateLimitConfig, logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	burst := defaultConfig.BurstSize
	if burst <= 0 && defaultConfig.RequestsPerMinute > 0 {
		burst = defaultConfig.RequestsPerMinute / 10
		if burst < 1 {
			burst = 1
		}
	}

	var limiters sync.Map // map[string]*rate.Limiter

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			// Unlimited when RPM is 0.
			if defaultConfig.RequestsPerMinute <= 0 {
				return next.RoundTrip(req)
			}

			host := req.URL.Host
			limiter := getOrCreateLimiter(&limiters, host, defaultConfig.RequestsPerMinute, burst)

			if !limiter.Allow() {
				logger.Warn("rate limit exceeded",
					slog.String("host", host),
					slog.Int("rpm_limit", defaultConfig.RequestsPerMinute),
				)
				return rateLimitResponse(req), nil
			}

			return next.RoundTrip(req)
		})
	}
}

// getOrCreateLimiter returns the rate.Limiter for the given host, creating one
// if it does not already exist.
func getOrCreateLimiter(limiters *sync.Map, host string, rpm, burst int) *rate.Limiter {
	if v, ok := limiters.Load(host); ok {
		return v.(*rate.Limiter)
	}

	// rate.Limit is events per second; RPM / 60 converts to per-second.
	limiter := rate.NewLimiter(rate.Limit(float64(rpm)/60.0), burst)
	actual, _ := limiters.LoadOrStore(host, limiter)
	return actual.(*rate.Limiter)
}

// rateLimitResponse builds a synthetic 429 Too Many Requests HTTP response.
func rateLimitResponse(req *http.Request) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"error":               "rate_limit_exceeded",
		"message":             "Request rate limit exceeded",
		"retry_after_seconds": 1,
	})

	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type": {"application/json"},
			"Retry-After":  {"1"},
		},
		Body:          io.NopCloser(strings.NewReader(string(body))),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}
