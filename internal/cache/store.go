// Package cache provides a pluggable caching layer for the Aegis egress proxy.
// It defines the [Store] interface that all cache backends must implement and
// provides helpers to convert between [CachedResponse] and [*http.Response].
package cache

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

// CachedResponse holds a serialized HTTP response for cache storage.
// It captures the essential parts of an HTTP response so they can be
// persisted and later reconstituted into a full [*http.Response].
type CachedResponse struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       []byte              `json:"body"`
	CachedAt   time.Time           `json:"cached_at"`
	TTL        time.Duration       `json:"ttl"`
}

// CacheStats provides cache performance metrics.
type CacheStats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Entries   int64 `json:"entries"`
	BytesUsed int64 `json:"bytes_used"`
}

// Store is the interface that all cache backends must implement.
// Implementations must be safe for concurrent use by multiple goroutines.
type Store interface {
	// Get retrieves a cached response by key. The boolean return value
	// indicates whether the key was found. An error is returned only for
	// backend failures, not cache misses.
	Get(ctx context.Context, key string) (*CachedResponse, bool, error)

	// Set stores a response in the cache with the given TTL.
	Set(ctx context.Context, key string, resp *CachedResponse, ttl time.Duration) error

	// Delete removes a single entry from the cache.
	Delete(ctx context.Context, key string) error

	// Stats returns a snapshot of the cache performance metrics.
	Stats() CacheStats

	// Close releases any resources held by the store.
	Close() error
}

// ToHTTPResponse converts a CachedResponse back into an [*http.Response].
// The returned response is associated with the given request and includes
// an X-Aegis-Cache: HIT header to indicate it was served from cache.
func (cr *CachedResponse) ToHTTPResponse(req *http.Request) *http.Response {
	header := make(http.Header, len(cr.Headers)+1)
	for k, vals := range cr.Headers {
		header[k] = vals
	}
	header.Set("X-Aegis-Cache", "HIT")

	return &http.Response{
		StatusCode:    cr.StatusCode,
		Status:        http.StatusText(cr.StatusCode),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(cr.Body)),
		ContentLength: int64(len(cr.Body)),
		Request:       req,
	}
}

// FromHTTPResponse creates a CachedResponse from an [*http.Response].
// It reads the full response body and then replaces resp.Body with a new
// reader so the original response remains usable by the caller.
func FromHTTPResponse(resp *http.Response, ttl time.Duration) (*CachedResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	// Restore the body so downstream consumers can still read it.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	headers := make(map[string][]string, len(resp.Header))
	for k, v := range resp.Header {
		dst := make([]string, len(v))
		copy(dst, v)
		headers[k] = dst
	}

	return &CachedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
		CachedAt:   time.Now(),
		TTL:        ttl,
	}, nil
}
