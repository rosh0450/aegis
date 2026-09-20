// Package app provides the top-level application wiring and server lifecycle
// management for the Aegis proxy.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rishavkumarj/aegis/internal/config"
	"github.com/rishavkumarj/aegis/internal/middleware"
	"github.com/rishavkumarj/aegis/internal/proxy"
	"github.com/rishavkumarj/aegis/internal/transport"
)

// shutdownTimeout is the maximum duration to wait for in-flight requests
// to complete during graceful shutdown.
const shutdownTimeout = 15 * time.Second

// Server manages the lifecycle of the Aegis proxy.
// It owns the wired dependency graph from config through to the HTTP server
// and handles graceful startup and shutdown.
type Server struct {
	cfg        *config.Config
	httpServer *http.Server
	logger     *slog.Logger
}

// NewServer creates a new Server from the given config.
//
// It wires together the full dependency chain:
//
//	config → transport → middleware chain → provider registry → forward proxy → handler → http.Server
//
// The middleware chain is composed of a logging middleware and an allowlist
// (domain-filtering) middleware layered on top of the base HTTP transport.
func NewServer(cfg *config.Config) (*Server, error) {
	// --- Logger -----------------------------------------------------------
	logger, err := buildLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("app: build logger: %w", err)
	}

	// --- Transport --------------------------------------------------------
	base := transport.NewTransport()

	// --- Middleware chain --------------------------------------------------
	chain := middleware.Chain(
		base,
		middleware.NewLoggingMiddleware(logger),
		middleware.NewAllowlistMiddleware(
			cfg.Security.Mode,
			cfg.Security.AllowlistDomains,
			cfg.Security.DenylistDomains,
		),
	)

	// --- Provider registry ------------------------------------------------
	providers := cfg.Providers
	if len(providers) == 0 {
		providers = proxy.DefaultProviders()
		logger.Info("no providers configured, using defaults", "count", len(providers))
	}

	registry, err := proxy.NewProviderRegistry(providers)
	if err != nil {
		return nil, fmt.Errorf("app: create provider registry: %w", err)
	}

	// --- Forward proxy & handler -----------------------------------------
	fwd := proxy.NewForwardProxy(registry, chain, logger)
	handler := proxy.NewHandler(fwd, logger)

	// --- HTTP server ------------------------------------------------------
	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutSec) * time.Second,
	}

	return &Server{
		cfg:        cfg,
		httpServer: srv,
		logger:     logger,
	}, nil
}

// Run starts the HTTP server and blocks until it is shut down.
//
// It installs OS signal handlers for SIGINT and SIGTERM. When a signal is
// received the server performs a graceful shutdown, waiting up to 15 seconds
// for in-flight requests to complete before returning.
func (s *Server) Run(ctx context.Context) error {
	// Derive a context that is cancelled on SIGINT / SIGTERM.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start the HTTP server in a separate goroutine.
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("starting aegis proxy", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Block until signal or server error.
	select {
	case err := <-errCh:
		return fmt.Errorf("app: server exited unexpectedly: %w", err)
	case <-ctx.Done():
		s.logger.Info("shutdown signal received, draining connections…")
	}

	// Graceful shutdown with a bounded timeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("app: graceful shutdown failed: %w", err)
	}

	s.logger.Info("server stopped gracefully")
	return nil
}

// buildLogger constructs an *slog.Logger from the logging configuration.
func buildLogger(lc config.LoggingConfig) (*slog.Logger, error) {
	var level slog.Level
	switch lc.Level {
	case "debug":
		level = slog.LevelDebug
	case "info", "":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown log level %q", lc.Level)
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch lc.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text", "":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("unknown log format %q", lc.Format)
	}

	return slog.New(handler), nil
}
