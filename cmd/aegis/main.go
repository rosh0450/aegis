// Package main is the entry point for the Aegis CLI.
//
// Build with version info:
//
//	go build -ldflags "-X main.Version=v1.0.0 -X main.Commit=$(git rev-parse HEAD) -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" ./cmd/aegis
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rishavkumarj/aegis/internal/app"
	"github.com/rishavkumarj/aegis/internal/config"
)

// Build-time variables injected via ldflags.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the full cobra command tree.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "aegis",
		Short: "Aegis - API Key & Security Governance Proxy",
		Long: `Aegis is a lightweight egress proxy that sits between your application
and third-party APIs. It provides domain filtering, credential injection,
caching, and budget enforcement.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newServeCmd(),
		newVersionCmd(),
		newConfigCmd(),
	)

	return root
}

// ---------------------------------------------------------------------------
// serve
// ---------------------------------------------------------------------------

func newServeCmd() *cobra.Command {
	var (
		cfgPath string
		addr    string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Aegis proxy server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadConfig(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// CLI flag overrides the config file value.
			if addr != "" {
				cfg.Server.Addr = addr
			}

			srv, err := app.NewServer(cfg)
			if err != nil {
				return err
			}

			return srv.Run(context.Background())
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "aegis.yaml", "path to config file")
	cmd.Flags().StringVarP(&addr, "addr", "a", "", "listen address (overrides config)")

	return cmd
}

// ---------------------------------------------------------------------------
// version
// ---------------------------------------------------------------------------

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("aegis %s\n", Version)
			fmt.Printf("  commit:  %s\n", Commit)
			fmt.Printf("  built:   %s\n", BuildDate)
		},
	}
}

// ---------------------------------------------------------------------------
// config (parent) + validate / init
// ---------------------------------------------------------------------------

func newConfigCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "config",
		Short: "Configuration utilities",
	}

	parent.AddCommand(newConfigValidateCmd(), newConfigInitCmd())
	return parent
}

func newConfigValidateCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a configuration file",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadConfig(cfgPath)
			if err != nil {
				return fmt.Errorf("config invalid: %w", err)
			}
			fmt.Printf("✔ config %q is valid\n", cfgPath)
			fmt.Printf("  server addr : %s\n", cfg.Server.Addr)
			fmt.Printf("  providers   : %d\n", len(cfg.Providers))
			fmt.Printf("  security    : %s\n", cfg.Security.Mode)
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "aegis.yaml", "path to config file")
	return cmd
}

// exampleConfig is the content written by `aegis config init`.
const exampleConfig = `# Aegis - API Key & Security Governance Proxy
# Configuration File

server:
  addr: ":8080"
  admin_addr: ":9090"
  read_timeout_sec: 30
  write_timeout_sec: 60
  idle_timeout_sec: 120

logging:
  level: "info"    # debug, info, warn, error
  format: "text"   # text, json

# Provider routing: maps URL path prefixes to upstream APIs
providers:
  - name: "openai"
    prefix: "/v1/openai"
    upstream: "https://api.openai.com"
  - name: "anthropic"
    prefix: "/v1/anthropic"
    upstream: "https://api.anthropic.com"
  - name: "gemini"
    prefix: "/v1/gemini"
    upstream: "https://generativelanguage.googleapis.com"

# Security & egress filtering
security:
  mode: "open"  # allowlist, denylist, open
  allowlist_domains: []
  denylist_domains: []
  strip_headers:
    - "Authorization"
    - "x-api-key"
    - "api-key"
  credentials:
    - domain: "api.openai.com"
      header: "Authorization"
      value_env: "OPENAI_API_KEY"
      prefix: "Bearer "
    - domain: "api.anthropic.com"
      header: "x-api-key"
      value_env: "ANTHROPIC_API_KEY"
      prefix: ""
  leak_detection:
    enabled: true
    mode: "warn"

# Caching configuration
cache:
  enabled: false
  backend: "memory"  # memory, sqlite, redis
  sqlite_path: "~/.aegis/cache.db"
  redis_addr: "localhost:6379"
  max_memory_mb: 256
  rules:
    - match: "api.openai.com/v1/embeddings"
      ttl: "24h"
    - match: "api.openai.com/v1/models"
      ttl: "1h"

# Budget & rate limiting
budget:
  enabled: false
  default_monthly_limit: 100.00
  db_path: "~/.aegis/budget.db"
  limits: []
`

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write an example config file to the current directory",
		RunE: func(_ *cobra.Command, _ []string) error {
			const target = "config.example.yaml"
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("%s already exists; remove it first", target)
			}
			if err := os.WriteFile(target, []byte(exampleConfig), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", target, err)
			}
			fmt.Printf("✔ wrote %s\n", target)
			return nil
		},
	}
}
