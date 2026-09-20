package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/rishavkumarj/aegis/internal/security"
)

// maxScanSize is the maximum request body size (1 MB) that the sanitize
// middleware will read and scan. Larger bodies are passed through without
// inspection to avoid performance degradation on bulk uploads.
const maxScanSize = 1 << 20 // 1 MB

// scannableContentTypes lists the Content-Type prefixes that are eligible for
// body scanning. Binary or multipart uploads are skipped.
var scannableContentTypes = []string{
	"application/json",
	"text/",
	"application/x-www-form-urlencoded",
}

// sanitizeErrorResponse is the JSON body returned when mode is "block" and
// leaked keys are detected.
type sanitizeErrorResponse struct {
	Error    string              `json:"error"`
	Message  string              `json:"message"`
	Detected []detectedKeyEntry  `json:"detected"`
}

// detectedKeyEntry is a single leaked key entry in the error response.
type detectedKeyEntry struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
}

// NewSanitizeMiddleware creates a middleware that scans outbound request bodies
// for accidentally included API keys before forwarding to upstream.
//
// mode controls behaviour:
//   - "block": reject the request with 400 Bad Request if keys are found.
//   - "warn":  log a warning but allow the request through.
//   - "" or any other value: disabled (pass through).
//
// Only request bodies that are smaller than 1 MB and carry a text-like
// Content-Type are scanned.
func NewSanitizeMiddleware(
	mode string,
	detector *security.Detector,
	logger *slog.Logger,
) Middleware {
	mode = strings.ToLower(strings.TrimSpace(mode))

	return func(next http.RoundTripper) http.RoundTripper {
		// Fast path: disabled mode — zero overhead.
		if mode != "block" && mode != "warn" {
			return next
		}

		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			// Nothing to scan when there is no body.
			if req.Body == nil || req.ContentLength == 0 {
				return next.RoundTrip(req)
			}

			// Skip bodies that are too large.
			if req.ContentLength > maxScanSize {
				logger.Debug("skipping body scan: body exceeds max scan size",
					slog.Int64("content_length", req.ContentLength),
					slog.String("host", req.URL.Host),
				)
				return next.RoundTrip(req)
			}

			// Skip non-text content types.
			if !isScannable(req.Header.Get("Content-Type")) {
				return next.RoundTrip(req)
			}

			// Read the full body. We use LimitReader as a safety net even
			// when ContentLength is known, because ContentLength can be -1
			// (unknown) and we still want a cap.
			body, err := io.ReadAll(io.LimitReader(req.Body, maxScanSize+1))
			if err != nil {
				return nil, err
			}
			// Always close the original body.
			_ = req.Body.Close()

			// Restore the body so the upstream transport can read it.
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))

			// Skip scan if we read more than maxScanSize (unknown length case).
			if len(body) > maxScanSize {
				return next.RoundTrip(req)
			}

			detected := detector.Scan(string(body))
			if len(detected) == 0 {
				return next.RoundTrip(req)
			}

			// --- Keys detected --------------------------------------------------

			if mode == "warn" {
				for _, dk := range detected {
					logger.Warn("potential API key detected in request body",
						slog.String("provider", dk.Provider),
						slog.String("name", dk.Name),
						slog.String("redacted", dk.Redacted),
						slog.String("host", req.URL.Host),
					)
				}
				return next.RoundTrip(req)
			}

			// mode == "block"
			logger.Warn("blocking request: API key(s) detected in body",
				slog.Int("count", len(detected)),
				slog.String("host", req.URL.Host),
			)

			return syntheticBlockResponse(req, detected), nil
		})
	}
}

// isScannable reports whether the given Content-Type value is eligible for
// body scanning.
func isScannable(ct string) bool {
	ct = strings.ToLower(ct)
	for _, prefix := range scannableContentTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

// syntheticBlockResponse builds a 400 Bad Request *http.Response with a JSON
// body describing the detected keys.
func syntheticBlockResponse(req *http.Request, detected []security.DetectedKey) *http.Response {
	entries := make([]detectedKeyEntry, len(detected))
	for i, dk := range detected {
		entries[i] = detectedKeyEntry{
			Provider: dk.Provider,
			Key:      dk.Redacted,
		}
	}

	payload := sanitizeErrorResponse{
		Error:    "api_key_leak_detected",
		Message:  "Request body contains potential API keys",
		Detected: entries,
	}

	body, _ := json.Marshal(payload) // types are safe; Marshal won't fail.

	return &http.Response{
		Status:     "400 Bad Request",
		StatusCode: http.StatusBadRequest,
		Proto:      req.Proto,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Header: http.Header{
			"Content-Type": {"application/json"},
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}
