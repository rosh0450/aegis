package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
)

// Version is the current Aegis proxy version.  It is set at build time
// via -ldflags or defaults to "dev".
var Version = "dev"

// contextKey is an unexported type for context keys defined in this package.
type contextKey int

const (
	// requestIDKey is the context key for the request ID.
	requestIDKey contextKey = iota
)

// RequestIDFromContext extracts the request ID from the context.
// Returns an empty string if no request ID is present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Handler is the main HTTP handler for the Aegis proxy.
// It routes requests to the appropriate upstream provider after adding
// request-level metadata such as a unique request ID.
type Handler struct {
	forward *ForwardProxy
	logger  *slog.Logger
}

// NewHandler creates a new proxy Handler.
//
// If logger is nil, [slog.Default] is used.
func NewHandler(forward *ForwardProxy, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		forward: forward,
		logger:  logger,
	}
}

// ServeHTTP implements [http.Handler].
//
// It performs the following steps for every request:
//  1. Generates or extracts a unique request ID (X-Request-ID header).
//  2. Attaches the request ID to the request context and response headers.
//  3. Routes health checks (GET /health) and welcome requests (GET /).
//  4. Delegates all other requests to the [ForwardProxy].
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// --- Request ID ---
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = generateRequestID()
	}

	ctx := context.WithValue(r.Context(), requestIDKey, reqID)
	r = r.WithContext(ctx)
	w.Header().Set("X-Request-ID", reqID)

	logger := h.logger.With(slog.String("request_id", reqID))

	// --- Health check ---
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		logger.Debug("health check")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
		})
		return
	}

	// --- Welcome / root ---
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		logger.Debug("welcome request")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "aegis",
			"version": Version,
			"message": "Aegis API egress proxy is running",
		})
		return
	}

	// --- Forward to upstream ---
	logger.Info("incoming request",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	)
	h.forward.ServeHTTP(w, r)
}

// generateRequestID returns a short random hex string suitable for use as
// a request identifier.  It produces 8 random bytes encoded as 16 hex
// characters (e.g. "a1b2c3d4e5f6a7b8").
func generateRequestID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Extremely unlikely; fall back to a fixed sentinel so callers
		// always get a non-empty value.
		return "00000000deadbeef"
	}
	return hex.EncodeToString(b)
}
