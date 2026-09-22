// Package admin provides an HTTP handler for the Aegis administrative API.
// It serves health checks, Prometheus metrics, budget summaries, and cache
// statistics on a separate port from the main proxy (default :9090).
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rishavkumarj/aegis/internal/budget"
	"github.com/rishavkumarj/aegis/internal/cache"
	"github.com/rishavkumarj/aegis/internal/proxy"
)

// Handler serves the admin API and metrics endpoint.
// It provides operational endpoints for health checking, observability, and
// cache management. The handler is intended to run on a separate port from
// the main proxy to isolate admin traffic.
type Handler struct {
	mux       *http.ServeMux
	tracker   *budget.Tracker
	cache     cache.Store // may be nil if caching is disabled
	registry  *proxy.ProviderRegistry
	logger    *slog.Logger
	startTime time.Time
}

// NewHandler creates a new admin API handler with the given dependencies.
// If cacheStore is nil, cache-related endpoints will return appropriate errors.
func NewHandler(
	tracker *budget.Tracker,
	cacheStore cache.Store,
	registry *proxy.ProviderRegistry,
	logger *slog.Logger,
) *Handler {
	h := &Handler{
		mux:       http.NewServeMux(),
		tracker:   tracker,
		cache:     cacheStore,
		registry:  registry,
		logger:    logger,
		startTime: time.Now(),
	}

	h.mux.HandleFunc("GET /health", h.handleHealth)
	h.mux.Handle("GET /metrics", promhttp.Handler())
	h.mux.HandleFunc("GET /admin/stats", h.handleStats)
	h.mux.HandleFunc("GET /admin/budget", h.handleBudget)
	h.mux.HandleFunc("GET /admin/cache", h.handleCache)
	h.mux.HandleFunc("POST /admin/cache/purge", h.handleCachePurge)

	return h
}

// ServeHTTP implements [http.Handler] by delegating to the internal ServeMux.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("admin request",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("remote", r.RemoteAddr),
	)
	h.mux.ServeHTTP(w, r)
}

// healthResponse is the JSON structure returned by the /health endpoint.
type healthResponse struct {
	Status        string  `json:"status"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// handleHealth serves GET /health and returns the service status and uptime.
func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:        "ok",
		UptimeSeconds: time.Since(h.startTime).Seconds(),
	})
}

// statsProviderInfo is a summary view of a registered provider.
type statsProviderInfo struct {
	Name     string `json:"name"`
	Prefix   string `json:"prefix"`
	Upstream string `json:"upstream"`
}

// statsResponse is the JSON structure returned by the /admin/stats endpoint.
type statsResponse struct {
	UptimeSeconds float64             `json:"uptime_seconds"`
	Providers     []statsProviderInfo  `json:"providers"`
	Cache         *cache.CacheStats   `json:"cache,omitempty"`
	Budget        statsBudgetSummary   `json:"budget"`
}

// statsBudgetSummary provides a high-level view of spending.
type statsBudgetSummary struct {
	TotalSpend float64 `json:"total_spend"`
	Records    int     `json:"records"`
}

// handleStats serves GET /admin/stats and returns a JSON summary of the
// proxy's current operational state including providers, cache, and budget.
func (h *Handler) handleStats(w http.ResponseWriter, _ *http.Request) {
	// Build provider list.
	providers := h.registry.Providers()
	providerInfos := make([]statsProviderInfo, len(providers))
	for i, p := range providers {
		providerInfos[i] = statsProviderInfo{
			Name:     p.Name,
			Prefix:   p.Prefix,
			Upstream: p.Upstream.String(),
		}
	}

	// Aggregate budget data.
	records := h.tracker.AllRecords()
	var totalSpend float64
	for _, r := range records {
		totalSpend += r.TotalSpend
	}

	resp := statsResponse{
		UptimeSeconds: time.Since(h.startTime).Seconds(),
		Providers:     providerInfos,
		Budget: statsBudgetSummary{
			TotalSpend: totalSpend,
			Records:    len(records),
		},
	}

	// Include cache stats if caching is enabled.
	if h.cache != nil {
		stats := h.cache.Stats()
		resp.Cache = &stats
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleBudget serves GET /admin/budget and returns all spend records as a
// JSON array.
func (h *Handler) handleBudget(w http.ResponseWriter, _ *http.Request) {
	records := h.tracker.AllRecords()
	if records == nil {
		records = make([]*budget.SpendRecord, 0)
	}
	writeJSON(w, http.StatusOK, records)
}

// handleCache serves GET /admin/cache and returns the current cache statistics.
// Returns an error if caching is not configured.
func (h *Handler) handleCache(w http.ResponseWriter, _ *http.Request) {
	if h.cache == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "caching is not enabled",
		})
		return
	}
	writeJSON(w, http.StatusOK, h.cache.Stats())
}

// handleCachePurge serves POST /admin/cache/purge and returns a success
// response. Full purge functionality is not yet implemented.
func (h *Handler) handleCachePurge(w http.ResponseWriter, _ *http.Request) {
	if h.cache == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "caching is not enabled",
		})
		return
	}

	h.logger.Info("cache purge requested")
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// writeJSON marshals v as JSON and writes it to the response with the
// appropriate Content-Type header and status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
