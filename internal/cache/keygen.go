package cache

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// GenerateKey creates a deterministic cache key from an HTTP request.
//
// For GET and HEAD requests the key is:
//
//	METHOD:HOST:PATH:SORTED_QUERY_PARAMS
//
// For POST, PUT, and PATCH requests the key includes a SHA-256 hash of
// the request body instead of query parameters:
//
//	METHOD:HOST:PATH:sha256(BODY)
//
// The request body is read and then restored so subsequent handlers can
// still consume it.
func GenerateKey(req *http.Request) (string, error) {
	method := req.Method
	host := req.URL.Host
	path := req.URL.Path

	switch method {
	case http.MethodGet, http.MethodHead:
		return fmt.Sprintf("%s:%s:%s:%s", method, host, path, sortedQuery(req)), nil

	case http.MethodPost, http.MethodPut, http.MethodPatch:
		hash, err := bodyHash(req)
		if err != nil {
			return "", fmt.Errorf("cache keygen: reading body: %w", err)
		}
		return fmt.Sprintf("%s:%s:%s:%s", method, host, path, hash), nil

	default:
		// Fallback for other methods – use method + host + path only.
		return fmt.Sprintf("%s:%s:%s", method, host, path), nil
	}
}

// sortedQuery returns the request's query parameters sorted
// alphabetically by key (and by value within the same key) and
// joined as key=value pairs separated by "&".
func sortedQuery(req *http.Request) string {
	params := req.URL.Query()
	if len(params) == 0 {
		return ""
	}

	// Sort keys.
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	first := true
	for _, k := range keys {
		vals := params[k]
		sort.Strings(vals)
		for _, v := range vals {
			if !first {
				b.WriteByte('&')
			}
			first = false
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(v)
		}
	}
	return b.String()
}

// bodyHash reads the request body, computes its SHA-256 hash, and
// restores the body so downstream handlers can still read it.
func bodyHash(req *http.Request) (string, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return "empty", nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}
	req.Body.Close()

	// Restore the body for downstream consumers.
	req.Body = io.NopCloser(bytes.NewReader(body))

	h := sha256.Sum256(body)
	return fmt.Sprintf("%x", h), nil
}
