package middleware

import (
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/rishavkumarj/aegis/internal/cache"
	"golang.org/x/sync/singleflight"
)

// defaultCacheTTL is used when caching is enabled but no rule matches
// the request URL.
const defaultCacheTTL = 5 * time.Minute

// CacheRule defines when and how long to cache a response.
type CacheRule struct {
	// Pattern is a host/path glob pattern (e.g. "api.openai.com/v1/*").
	// It is matched against the request's Host + Path using [path.Match].
	Pattern string

	// TTL is how long a matching response should be cached.
	TTL time.Duration
}

// cacheResult bundles the return values from a singleflight call so they
// can be passed through the generic interface{} return.
type cacheResult struct {
	resp *http.Response
	err  error
}

// NewCacheMiddleware creates a caching [Middleware] that checks the cache
// before forwarding requests and stores successful (2xx) responses.
//
// Concurrent identical requests are deduplicated via [singleflight.Group]
// so that at most one upstream call is made per unique cache key at any
// given time.
//
// Only GET and POST requests are eligible for caching. POST caching is
// useful for idempotent AI API calls such as embeddings. Responses with
// Cache-Control: no-store or no-cache are never cached.
func NewCacheMiddleware(
	store cache.Store,
	rules []CacheRule,
	logger *slog.Logger,
) Middleware {
	var group singleflight.Group

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			// Only cache GET and POST requests.
			if req.Method != http.MethodGet && req.Method != http.MethodPost {
				return next.RoundTrip(req)
			}

			key, err := cache.GenerateKey(req)
			if err != nil {
				logger.Warn("cache: key generation failed, skipping cache",
					slog.String("error", err.Error()),
				)
				return next.RoundTrip(req)
			}

			// Check for a cached response.
			if cached, ok, err := store.Get(req.Context(), key); err == nil && ok {
				logger.Debug("cache: hit",
					slog.String("key", key),
					slog.String("url", req.URL.String()),
				)
				return cached.ToHTTPResponse(req), nil
			}

			// Deduplicate concurrent identical requests.
			v, err, _ := group.Do(key, func() (interface{}, error) {
				resp, err := next.RoundTrip(req)
				if err != nil {
					return &cacheResult{resp: nil, err: err}, nil
				}

				// Only cache successful (2xx) responses.
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					if !hasNoCacheDirective(resp) {
						ttl := matchTTL(rules, req)
						cr, cacheErr := cache.FromHTTPResponse(resp, ttl)
						if cacheErr != nil {
							logger.Warn("cache: failed to serialize response",
								slog.String("key", key),
								slog.String("error", cacheErr.Error()),
							)
						} else {
							if setErr := store.Set(req.Context(), key, cr, ttl); setErr != nil {
								logger.Warn("cache: failed to store response",
									slog.String("key", key),
									slog.String("error", setErr.Error()),
								)
							}
						}
					}
				}

				// Tag the response as a cache miss.
				resp.Header.Set("X-Aegis-Cache", "MISS")
				return &cacheResult{resp: resp, err: nil}, nil
			})

			if err != nil {
				return nil, err
			}

			result := v.(*cacheResult)
			return result.resp, result.err
		})
	}
}

// matchTTL returns the TTL for the first [CacheRule] whose Pattern matches
// the request's Host+Path. If no rule matches, [defaultCacheTTL] is returned.
func matchTTL(rules []CacheRule, req *http.Request) time.Duration {
	target := req.URL.Host + req.URL.Path
	for _, r := range rules {
		if matched, _ := path.Match(r.Pattern, target); matched {
			return r.TTL
		}
	}
	return defaultCacheTTL
}

// hasNoCacheDirective checks whether the response carries a Cache-Control
// header containing "no-store" or "no-cache".
func hasNoCacheDirective(resp *http.Response) bool {
	cc := resp.Header.Get("Cache-Control")
	if cc == "" {
		return false
	}
	lower := strings.ToLower(cc)
	return strings.Contains(lower, "no-store") || strings.Contains(lower, "no-cache")
}
