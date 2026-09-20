// Package transport provides optimized HTTP transports for the Aegis proxy's
// outbound connections to upstream API providers.
package transport

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// NewTransport returns an *http.Transport tuned for high-throughput API egress.
//
// The transport is configured with:
//   - Aggressive connection pooling (500 idle conns per host, 10 000 total)
//   - Per-host connection cap of 2 000 to prevent resource exhaustion
//   - HTTP/2 enabled via ForceAttemptHTTP2
//   - TLS 1.2 minimum for security compliance
//   - Conservative timeouts: 10s dial, 10s TLS handshake, 30s response header, 90s idle
//
// Callers should share a single Transport across the application; it is safe
// for concurrent use and maintains its own connection pool.
func NewTransport() *http.Transport {
	return &http.Transport{
		// Connection pooling.
		MaxIdleConns:        10_000,
		MaxIdleConnsPerHost: 500,
		MaxConnsPerHost:     2_000,
		IdleConnTimeout:     90 * time.Second,

		// Dialer with timeout and keep-alive.
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		// TLS configuration.
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},

		// Response header timeout guards against slow upstream responses.
		ResponseHeaderTimeout: 30 * time.Second,

		// Expect-Continue timeout for requests with bodies.
		ExpectContinueTimeout: 1 * time.Second,

		// Enable HTTP/2 negotiation.
		ForceAttemptHTTP2: true,

		// Disable automatic compression so the proxy passes through the
		// upstream's Content-Encoding faithfully.
		DisableCompression: false,
	}
}
