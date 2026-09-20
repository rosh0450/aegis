// Package proxy implements the Aegis reverse-proxy engine.
// It routes incoming requests to upstream API providers based on URL
// path-prefix matching and forwards them through a configurable
// middleware transport chain.
package proxy

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/rishavkumarj/aegis/internal/config"
)

// Provider represents a registered API provider with routing information.
// Each provider maps a local URL path prefix to an upstream base URL so
// that incoming requests can be rewritten and forwarded.
type Provider struct {
	Name     string   // Human-readable name ("openai", "anthropic")
	Prefix   string   // URL path prefix ("/v1/openai")
	Upstream *url.URL // Parsed upstream base URL
}

// ProviderRegistry maps incoming request paths to upstream providers.
// Providers are matched in registration order; the first provider whose
// prefix matches the incoming path wins.
type ProviderRegistry struct {
	providers []Provider
}

// NewProviderRegistry creates a ProviderRegistry from a slice of
// [config.ProviderConfig] values.  It parses and validates each upstream
// URL and returns an error if any configuration is invalid.
func NewProviderRegistry(configs []config.ProviderConfig) (*ProviderRegistry, error) {
	providers := make([]Provider, 0, len(configs))

	for _, cfg := range configs {
		if cfg.Name == "" {
			return nil, fmt.Errorf("provider config: name must not be empty")
		}
		if cfg.Prefix == "" {
			return nil, fmt.Errorf("provider %q: prefix must not be empty", cfg.Name)
		}
		if cfg.Upstream == "" {
			return nil, fmt.Errorf("provider %q: upstream must not be empty", cfg.Name)
		}

		u, err := url.Parse(cfg.Upstream)
		if err != nil {
			return nil, fmt.Errorf("provider %q: invalid upstream URL %q: %w", cfg.Name, cfg.Upstream, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("provider %q: upstream scheme must be http or https, got %q", cfg.Name, u.Scheme)
		}

		// Ensure prefix starts with "/" and has no trailing slash for
		// consistent matching behaviour.
		prefix := cfg.Prefix
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		prefix = strings.TrimRight(prefix, "/")

		providers = append(providers, Provider{
			Name:     cfg.Name,
			Prefix:   prefix,
			Upstream: u,
		})
	}

	return &ProviderRegistry{providers: providers}, nil
}

// Match finds the provider whose prefix matches the given request path.
//
// It returns the matched [Provider], the remaining path after stripping the
// prefix, and a boolean indicating whether a match was found.
//
// Example: given prefix "/v1/openai" and path "/v1/openai/chat/completions",
// Match returns the openai provider, remaining path "/chat/completions", and true.
func (r *ProviderRegistry) Match(path string) (*Provider, string, bool) {
	for i := range r.providers {
		p := &r.providers[i]
		if strings.HasPrefix(path, p.Prefix) {
			remaining := strings.TrimPrefix(path, p.Prefix)
			// Ensure the match is at a path boundary.
			// "/v1/openai" should not match "/v1/openaifoo".
			if remaining != "" && !strings.HasPrefix(remaining, "/") {
				continue
			}
			if remaining == "" {
				remaining = "/"
			}
			return p, remaining, true
		}
	}
	return nil, "", false
}

// Providers returns a copy of all registered providers.
func (r *ProviderRegistry) Providers() []Provider {
	out := make([]Provider, len(r.providers))
	copy(out, r.providers)
	return out
}

// DefaultProviders returns ProviderConfig entries for commonly used API
// providers.  These can be passed directly to [NewProviderRegistry].
func DefaultProviders() []config.ProviderConfig {
	return []config.ProviderConfig{
		{
			Name:     "openai",
			Prefix:   "/v1/openai",
			Upstream: "https://api.openai.com",
		},
		{
			Name:     "anthropic",
			Prefix:   "/v1/anthropic",
			Upstream: "https://api.anthropic.com",
		},
		{
			Name:     "gemini",
			Prefix:   "/v1/gemini",
			Upstream: "https://generativelanguage.googleapis.com",
		},
	}
}
