// Package middleware provides composable http.RoundTripper middlewares for the
// Aegis egress proxy. Middlewares wrap outbound HTTP requests to add behavior
// such as domain filtering, credential injection, logging, and caching.
package middleware

import "net/http"

// RoundTripperFunc is an adapter to allow the use of ordinary functions as
// http.RoundTripper. If f is a function with the appropriate signature,
// RoundTripperFunc(f) is a RoundTripper that calls f.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls f(req).
func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Middleware is a function that wraps an http.RoundTripper to add behavior
// to outbound HTTP requests. Middlewares are composed using [Chain].
type Middleware func(http.RoundTripper) http.RoundTripper

// Chain composes multiple middlewares around a base RoundTripper.
// Middlewares are applied in the order given: the first middleware in the
// list is the outermost wrapper (executes first on request, last on response).
//
// Example:
//
//	transport := middleware.Chain(
//	    http.DefaultTransport,
//	    middleware.NewLoggingMiddleware(logger),
//	    middleware.NewAllowlistMiddleware("allowlist", allowed, nil),
//	)
func Chain(base http.RoundTripper, middlewares ...Middleware) http.RoundTripper {
	for i := len(middlewares) - 1; i >= 0; i-- {
		base = middlewares[i](base)
	}
	return base
}
