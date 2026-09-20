package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/rishavkumarj/aegis/internal/config"
)

// defaultStripHeaders are the headers stripped from client requests when no
// custom list is configured. These cover the most common patterns used by
// cloud API providers.
var defaultStripHeaders = []string{
	"Authorization",
	"X-Api-Key",
	"Api-Key",
	"X-Goog-Api-Key",
}

// sensitiveQueryParams lists query parameter names that commonly carry API
// keys or tokens. They are removed from every outgoing request URL.
var sensitiveQueryParams = []string{
	"key",
	"api_key",
	"apikey",
	"access_token",
	"token",
}

// NewCredentialMiddleware creates a middleware that:
//  1. STRIPS sensitive headers from client requests (Authorization, x-api-key,
//     api-key, x-goog-api-key) or a custom set supplied via stripHeaders.
//  2. STRIPS sensitive query parameters (?key=, ?api_key=, ?apikey=,
//     ?access_token=, ?token=).
//  3. INJECTS the correct credential for the target domain based on config.
//
// The credential is read from the environment variable specified in the
// [config.CredentialEntry]. If the env var is empty or not set, no credential
// is injected and a warning is logged.
//
// The original request is never mutated; a shallow clone is created before any
// modifications are applied.
func NewCredentialMiddleware(
	credentials []config.CredentialEntry,
	stripHeaders []string,
	logger *slog.Logger,
) Middleware {
	// Build O(1) lookup map: domain -> CredentialEntry.
	credMap := make(map[string]config.CredentialEntry, len(credentials))
	for _, c := range credentials {
		credMap[strings.ToLower(c.Domain)] = c
	}

	// Fall back to defaults when no custom strip list is provided.
	if len(stripHeaders) == 0 {
		stripHeaders = defaultStripHeaders
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			// Clone so we never mutate the caller's request.
			req = req.Clone(req.Context())

			// --- Strip phase ---------------------------------------------------

			// Remove sensitive headers.
			for _, h := range stripHeaders {
				if req.Header.Get(h) != "" {
					logger.Debug("stripping header from client request",
						slog.String("header", h),
						slog.String("host", req.URL.Host),
					)
					req.Header.Del(h)
				}
			}

			// Remove sensitive query parameters.
			q := req.URL.Query()
			changed := false
			for _, param := range sensitiveQueryParams {
				if q.Has(param) {
					logger.Debug("stripping query parameter from client request",
						slog.String("param", param),
						slog.String("host", req.URL.Host),
					)
					q.Del(param)
					changed = true
				}
			}
			if changed {
				req.URL.RawQuery = q.Encode()
			}

			// --- Inject phase --------------------------------------------------

			host := strings.ToLower(req.URL.Host)
			entry, ok := credMap[host]
			if !ok {
				// No credential configured for this domain; forward as-is.
				return next.RoundTrip(req)
			}

			envVal := os.Getenv(entry.ValueEnv)
			if envVal == "" {
				logger.Warn("credential env var is not set for matched domain",
					slog.String("domain", host),
					slog.String("env_var", entry.ValueEnv),
				)
				return next.RoundTrip(req)
			}

			headerValue := entry.Prefix + envVal

			req.Header.Set(entry.Header, headerValue)

			// Log with the value redacted: show only the first 4 and last 4 chars.
			logger.Debug("injected credential for domain",
				slog.String("domain", host),
				slog.String("header", entry.Header),
				slog.String("value", redactValue(envVal)),
			)

			return next.RoundTrip(req)
		})
	}
}

// redactValue returns a redacted version of v, showing at most the first 4
// and last 4 characters separated by "…". Short values are fully masked.
func redactValue(v string) string {
	if len(v) <= 8 {
		return strings.Repeat("*", len(v))
	}
	return v[:4] + "…" + v[len(v)-4:]
}
