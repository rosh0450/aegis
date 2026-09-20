package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// authRedactLen is the number of leading characters of the Authorization
// header value to keep visible when redacting.
const authRedactLen = 8

// NewLoggingMiddleware returns a [Middleware] that logs every outbound request
// and its response using structured logging via [slog.Logger].
//
// Each log entry includes the HTTP method, target host, path, response status
// code, round-trip latency in milliseconds, and the response Content-Length
// (bytes sent). Authorization header values are redacted in debug-level logs,
// showing only the first 8 characters followed by "...".
func NewLoggingMiddleware(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			start := time.Now()

			// Debug-level log with (redacted) headers for deeper inspection.
			logger.Debug("outbound request",
				slog.String("method", req.Method),
				slog.String("host", req.URL.Host),
				slog.String("path", req.URL.Path),
				slog.String("authorization", redactAuthorization(req.Header.Get("Authorization"))),
			)

			resp, err := next.RoundTrip(req)

			latency := time.Since(start)

			if err != nil {
				logger.Error("outbound request failed",
					slog.String("method", req.Method),
					slog.String("host", req.URL.Host),
					slog.String("path", req.URL.Path),
					slog.Duration("latency", latency),
					slog.Float64("latency_ms", float64(latency.Milliseconds())),
					slog.String("error", err.Error()),
				)
				return nil, err
			}

			logger.Info("outbound request completed",
				slog.String("method", req.Method),
				slog.String("host", req.URL.Host),
				slog.String("path", req.URL.Path),
				slog.Int("status_code", resp.StatusCode),
				slog.Float64("latency_ms", float64(latency.Milliseconds())),
				slog.Int64("bytes_sent", resp.ContentLength),
			)

			return resp, nil
		})
	}
}

// redactAuthorization truncates an Authorization header value for safe logging.
// If the value is longer than [authRedactLen] characters, only the first
// authRedactLen characters are kept, followed by "...". Empty values are
// returned as-is.
func redactAuthorization(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= authRedactLen {
		return value + "..."
	}
	return value[:authRedactLen] + "..."
}
