// Package config defines the configuration types for the Aegis proxy.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config is the root configuration for the Aegis proxy.
type Config struct {
	Server    ServerConfig     `koanf:"server"`
	Security  SecurityConfig   `koanf:"security"`
	Cache     CacheConfig      `koanf:"cache"`
	Budget    BudgetConfig     `koanf:"budget"`
	Logging   LoggingConfig    `koanf:"logging"`
	Providers []ProviderConfig `koanf:"providers"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr            string `koanf:"addr"`              // Listen address, default ":8080"
	AdminAddr       string `koanf:"admin_addr"`        // Admin API address, default ":9090"
	ReadTimeoutSec  int    `koanf:"read_timeout_sec"`  // Default 30
	WriteTimeoutSec int    `koanf:"write_timeout_sec"` // Default 60
	IdleTimeoutSec  int    `koanf:"idle_timeout_sec"`  // Default 120
}

// ReadTimeout returns the read timeout as a time.Duration.
func (s ServerConfig) ReadTimeout() time.Duration {
	return time.Duration(s.ReadTimeoutSec) * time.Second
}

// WriteTimeout returns the write timeout as a time.Duration.
func (s ServerConfig) WriteTimeout() time.Duration {
	return time.Duration(s.WriteTimeoutSec) * time.Second
}

// IdleTimeout returns the idle timeout as a time.Duration.
func (s ServerConfig) IdleTimeout() time.Duration {
	return time.Duration(s.IdleTimeoutSec) * time.Second
}

// SecurityConfig controls domain filtering, credential injection, and leak detection.
type SecurityConfig struct {
	Mode             string              `koanf:"mode"`              // "allowlist" | "denylist" | "open"
	AllowlistDomains []string            `koanf:"allowlist_domains"` // Glob patterns
	DenylistDomains  []string            `koanf:"denylist_domains"`
	StripHeaders     []string            `koanf:"strip_headers"` // Headers to strip from client requests
	Credentials      []CredentialEntry   `koanf:"credentials"`
	LeakDetection    LeakDetectionConfig `koanf:"leak_detection"`
}

// CredentialEntry maps a domain to the credential that should be injected
// into outbound requests targeting that domain.
type CredentialEntry struct {
	Domain   string `koanf:"domain"`    // e.g. "api.openai.com"
	Header   string `koanf:"header"`    // e.g. "Authorization"
	ValueEnv string `koanf:"value_env"` // Env var name, e.g. "OPENAI_API_KEY"
	Prefix   string `koanf:"prefix"`    // e.g. "Bearer " (prepended to env value)
}

// LeakDetectionConfig controls whether the proxy scans request bodies for
// accidentally included API keys or secrets.
type LeakDetectionConfig struct {
	Enabled bool   `koanf:"enabled"` // Enable API key leak detection in request bodies
	Mode    string `koanf:"mode"`    // "block" or "warn"
}

// CacheConfig controls the response caching layer.
type CacheConfig struct {
	Enabled     bool        `koanf:"enabled"`
	Backend     string      `koanf:"backend"`       // "memory" | "sqlite" | "redis"
	SQLitePath  string      `koanf:"sqlite_path"`   // Path to SQLite DB file
	RedisAddr   string      `koanf:"redis_addr"`    // Redis address
	MaxMemoryMB int         `koanf:"max_memory_mb"` // Max memory for in-memory cache
	Rules       []CacheRule `koanf:"rules"`
}

// CacheRule defines a TTL for responses matching a URL pattern.
type CacheRule struct {
	Match string `koanf:"match"` // URL pattern to match (glob)
	TTL   string `koanf:"ttl"`   // Duration string e.g. "24h", "5m"
}

// BudgetConfig controls spend tracking and rate limiting.
type BudgetConfig struct {
	Enabled             bool          `koanf:"enabled"`
	DefaultMonthlyLimit float64       `koanf:"default_monthly_limit"` // Default $ limit per month
	DBPath              string        `koanf:"db_path"`               // SQLite path for spend tracking
	Limits              []BudgetLimit `koanf:"limits"`
}

// BudgetLimit defines spend and rate limits for a specific developer/endpoint pair.
type BudgetLimit struct {
	Developer    string  `koanf:"developer"`     // Developer/team identifier
	Endpoint     string  `koanf:"endpoint"`      // Target endpoint pattern
	MonthlyLimit float64 `koanf:"monthly_limit"` // $ limit per month
	RPM          int     `koanf:"rpm"`           // Requests per minute
	TPM          int     `koanf:"tpm"`           // Tokens per minute
}

// LoggingConfig controls structured logging output.
type LoggingConfig struct {
	Level  string `koanf:"level"`  // "debug", "info", "warn", "error"
	Format string `koanf:"format"` // "text" or "json"
}

// ProviderConfig maps an incoming URL path prefix to an upstream API provider.
type ProviderConfig struct {
	Name     string `koanf:"name"`     // Human-readable provider name, e.g. "openai"
	Prefix   string `koanf:"prefix"`   // URL path prefix, e.g. "/v1/openai"
	Upstream string `koanf:"upstream"` // Upstream base URL, e.g. "https://api.openai.com"
}

// DefaultConfig returns a Config populated with sensible default values.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Addr:            ":8080",
			AdminAddr:       ":9090",
			ReadTimeoutSec:  30,
			WriteTimeoutSec: 60,
			IdleTimeoutSec:  120,
		},
		Security: SecurityConfig{
			Mode: "allowlist",
			LeakDetection: LeakDetectionConfig{
				Enabled: true,
				Mode:    "block",
			},
		},
		Cache: CacheConfig{
			Enabled:     false,
			Backend:     "memory",
			MaxMemoryMB: 256,
		},
		Budget: BudgetConfig{
			Enabled:             false,
			DefaultMonthlyLimit: 100.0,
			DBPath:              "aegis_budget.db",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Validate checks the Config for logical errors and returns a combined error
// describing all problems found. It returns nil if the configuration is valid.
func (c *Config) Validate() error {
	var errs []error

	// Server validation.
	if c.Server.Addr == "" {
		errs = append(errs, errors.New("server.addr must not be empty"))
	}
	if c.Server.ReadTimeoutSec <= 0 {
		errs = append(errs, errors.New("server.read_timeout_sec must be positive"))
	}
	if c.Server.WriteTimeoutSec <= 0 {
		errs = append(errs, errors.New("server.write_timeout_sec must be positive"))
	}
	if c.Server.IdleTimeoutSec <= 0 {
		errs = append(errs, errors.New("server.idle_timeout_sec must be positive"))
	}

	// Security validation.
	validModes := map[string]bool{"allowlist": true, "denylist": true, "open": true}
	if !validModes[c.Security.Mode] {
		errs = append(errs, fmt.Errorf("security.mode must be one of allowlist, denylist, open; got %q", c.Security.Mode))
	}
	if c.Security.Mode == "allowlist" && len(c.Security.AllowlistDomains) == 0 && len(c.Providers) == 0 {
		errs = append(errs, errors.New("security.mode is allowlist but no allowlist_domains or providers are configured"))
	}
	if c.Security.LeakDetection.Enabled {
		validLeakModes := map[string]bool{"block": true, "warn": true}
		if !validLeakModes[c.Security.LeakDetection.Mode] {
			errs = append(errs, fmt.Errorf("security.leak_detection.mode must be block or warn; got %q", c.Security.LeakDetection.Mode))
		}
	}
	for i, cred := range c.Security.Credentials {
		if cred.Domain == "" {
			errs = append(errs, fmt.Errorf("security.credentials[%d].domain must not be empty", i))
		}
		if cred.Header == "" {
			errs = append(errs, fmt.Errorf("security.credentials[%d].header must not be empty", i))
		}
		if cred.ValueEnv == "" {
			errs = append(errs, fmt.Errorf("security.credentials[%d].value_env must not be empty", i))
		}
	}

	// Cache validation.
	if c.Cache.Enabled {
		validBackends := map[string]bool{"memory": true, "sqlite": true, "redis": true}
		if !validBackends[c.Cache.Backend] {
			errs = append(errs, fmt.Errorf("cache.backend must be one of memory, sqlite, redis; got %q", c.Cache.Backend))
		}
		if c.Cache.Backend == "sqlite" && c.Cache.SQLitePath == "" {
			errs = append(errs, errors.New("cache.sqlite_path is required when cache.backend is sqlite"))
		}
		if c.Cache.Backend == "redis" && c.Cache.RedisAddr == "" {
			errs = append(errs, errors.New("cache.redis_addr is required when cache.backend is redis"))
		}
		for i, rule := range c.Cache.Rules {
			if rule.Match == "" {
				errs = append(errs, fmt.Errorf("cache.rules[%d].match must not be empty", i))
			}
			if rule.TTL == "" {
				errs = append(errs, fmt.Errorf("cache.rules[%d].ttl must not be empty", i))
			} else if _, err := time.ParseDuration(rule.TTL); err != nil {
				errs = append(errs, fmt.Errorf("cache.rules[%d].ttl is invalid: %w", i, err))
			}
		}
	}

	// Budget validation.
	if c.Budget.Enabled {
		if c.Budget.DBPath == "" {
			errs = append(errs, errors.New("budget.db_path is required when budget is enabled"))
		}
		if c.Budget.DefaultMonthlyLimit < 0 {
			errs = append(errs, errors.New("budget.default_monthly_limit must not be negative"))
		}
	}

	// Logging validation.
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		errs = append(errs, fmt.Errorf("logging.level must be one of debug, info, warn, error; got %q", c.Logging.Level))
	}
	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[c.Logging.Format] {
		errs = append(errs, fmt.Errorf("logging.format must be text or json; got %q", c.Logging.Format))
	}

	// Provider validation.
	prefixes := make(map[string]bool)
	for i, p := range c.Providers {
		if p.Name == "" {
			errs = append(errs, fmt.Errorf("providers[%d].name must not be empty", i))
		}
		if p.Prefix == "" {
			errs = append(errs, fmt.Errorf("providers[%d].prefix must not be empty", i))
		} else if !strings.HasPrefix(p.Prefix, "/") {
			errs = append(errs, fmt.Errorf("providers[%d].prefix must start with /; got %q", i, p.Prefix))
		} else if prefixes[p.Prefix] {
			errs = append(errs, fmt.Errorf("providers[%d].prefix %q is duplicated", i, p.Prefix))
		} else {
			prefixes[p.Prefix] = true
		}
		if p.Upstream == "" {
			errs = append(errs, fmt.Errorf("providers[%d].upstream must not be empty", i))
		} else if u, err := url.Parse(p.Upstream); err != nil || u.Scheme == "" {
			errs = append(errs, fmt.Errorf("providers[%d].upstream must be a valid URL with scheme; got %q", i, p.Upstream))
		}
	}

	return errors.Join(errs...)
}
