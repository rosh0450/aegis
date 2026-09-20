package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// FilterMode controls how the allowlist middleware evaluates domains.
const (
	// FilterModeAllowlist permits only domains that match the allowlist patterns.
	FilterModeAllowlist = "allowlist"
	// FilterModeDenylist blocks domains that match the denylist patterns.
	FilterModeDenylist = "denylist"
	// FilterModeOpen permits all domains without restriction.
	FilterModeOpen = "open"
)

// blockResponse is the JSON body returned when a domain is blocked.
type blockResponse struct {
	Error   string `json:"error"`
	Domain  string `json:"domain"`
	Message string `json:"message"`
}

// NewAllowlistMiddleware returns a [Middleware] that filters outbound requests
// by domain. It supports three modes:
//
//   - "allowlist": only requests to domains matching an allowlist pattern are
//     forwarded; all others receive a synthetic 403 response.
//   - "denylist": requests to domains matching a denylist pattern are blocked;
//     all others are forwarded.
//   - "open": all requests are forwarded without restriction.
//
// Patterns support glob syntax as defined by [path.Match] (e.g. "*.openai.com").
// The host is extracted from the request URL and compared without the port.
func NewAllowlistMiddleware(mode string, allowlist []string, denylist []string) Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			host := extractHost(req)

			var blocked bool
			var reason string

			switch mode {
			case FilterModeAllowlist:
				if !matchesDomain(host, allowlist) {
					blocked = true
					reason = "Domain is not in the allowlist"
				}
			case FilterModeDenylist:
				if matchesDomain(host, denylist) {
					blocked = true
					reason = "Domain is in the denylist"
				}
			case FilterModeOpen:
				// All domains are permitted.
			default:
				slog.Warn("unknown allowlist filter mode, defaulting to open",
					slog.String("mode", mode),
				)
			}

			if blocked {
				slog.Info("blocked outbound request",
					slog.String("domain", host),
					slog.String("mode", mode),
					slog.String("method", req.Method),
					slog.String("url", req.URL.String()),
				)
				return syntheticForbidden(req, host, reason), nil
			}

			return next.RoundTrip(req)
		})
	}
}

// matchesDomain reports whether host matches any of the provided glob patterns.
// Matching is case-insensitive and uses [path.Match] semantics.
func matchesDomain(host string, patterns []string) bool {
	host = strings.ToLower(host)
	for _, p := range patterns {
		p = strings.ToLower(p)
		if matched, _ := path.Match(p, host); matched {
			return true
		}
	}
	return false
}

// extractHost returns the hostname portion of the request URL, stripping any
// port suffix.
func extractHost(req *http.Request) string {
	host := req.URL.Hostname()
	if host == "" {
		// Fallback to the Host header, stripping any port.
		host = req.Host
		if h, _, found := strings.Cut(host, ":"); found {
			host = h
		}
	}
	return host
}

// syntheticForbidden builds a 403 Forbidden *http.Response with a JSON body
// describing the blocked domain.
func syntheticForbidden(req *http.Request, domain, message string) *http.Response {
	body := blockResponse{
		Error:   "domain_blocked",
		Domain:  domain,
		Message: message,
	}
	data, err := json.Marshal(body)
	if err != nil {
		// Fallback to a plain message if marshalling somehow fails.
		data = []byte(fmt.Sprintf(`{"error":"domain_blocked","domain":%q,"message":%q}`, domain, message))
	}

	return &http.Response{
		Status:     "403 Forbidden",
		StatusCode: http.StatusForbidden,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type":   {"application/json"},
			"Content-Length": {fmt.Sprintf("%d", len(data))},
		},
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
		Request:       req,
	}
}
