package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
)

// ForwardProxy wraps [httputil.ReverseProxy] to forward requests to upstream
// API providers.  It uses the Go 1.20+ Rewrite hook for URL rewriting and
// delegates provider resolution to a [ProviderRegistry].
type ForwardProxy struct {
	registry  *ProviderRegistry
	transport http.RoundTripper
	logger    *slog.Logger
}

// NewForwardProxy creates a new ForwardProxy with the given registry,
// transport (which should already have any middleware chain applied), and
// structured logger.
//
// If transport is nil, [http.DefaultTransport] is used.
// If logger is nil, [slog.Default] is used.
func NewForwardProxy(registry *ProviderRegistry, transport http.RoundTripper, logger *slog.Logger) *ForwardProxy {
	if transport == nil {
		transport = http.DefaultTransport
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ForwardProxy{
		registry:  registry,
		transport: transport,
		logger:    logger,
	}
}

// ServeHTTP handles incoming proxy requests by matching them to a provider
// and forwarding via [httputil.ReverseProxy].
//
// The flow is:
//  1. Match the request path against the provider registry.
//  2. If no match, respond with a 404 JSON error.
//  3. Create an httputil.ReverseProxy with a Rewrite hook that rewrites
//     the URL to the upstream provider, preserving the remaining path and
//     query parameters.
//  4. Forward the request through the configured transport.
func (fp *ForwardProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	provider, remaining, ok := fp.registry.Match(r.URL.Path)
	if !ok {
		fp.logger.Warn("no matching provider",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
		)
		writeJSONError(w, http.StatusNotFound, "no provider matched the request path")
		return
	}

	fp.logger.Info("forwarding request",
		slog.String("provider", provider.Name),
		slog.String("upstream", provider.Upstream.String()),
		slog.String("remaining_path", remaining),
		slog.String("method", r.Method),
	)

	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Set the target URL scheme and host from the upstream provider.
			pr.SetURL(provider.Upstream)

			// The upstream base path (usually empty) plus the remaining
			// path after stripping the Aegis prefix form the final path.
			pr.Out.URL.Path = singleJoiningSlash(provider.Upstream.Path, remaining)

			// Preserve query parameters from the original request.
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery

			// Set the Host header to the upstream host so TLS and
			// virtual-host routing work correctly.
			pr.Out.Host = provider.Upstream.Host

			// Set standard forwarding headers.
			pr.SetXForwarded()
		},
		Transport: fp.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			fp.logger.Error("upstream request failed",
				slog.String("provider", provider.Name),
				slog.String("error", err.Error()),
			)
			writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("upstream error: %v", err))
		},
	}

	rp.ServeHTTP(w, r)
}

// singleJoiningSlash joins two URL path segments with exactly one "/".
func singleJoiningSlash(a, b string) string {
	aSlash := len(a) > 0 && a[len(a)-1] == '/'
	bSlash := len(b) > 0 && b[0] == '/'
	switch {
	case aSlash && bSlash:
		return a + b[1:]
	case !aSlash && !bSlash:
		return a + "/" + b
	}
	return a + b
}

// writeJSONError writes a JSON-encoded error response with the given HTTP
// status code and message.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
