package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// LoadConfig loads the Aegis configuration by merging three layers
// (in increasing priority):
//
//  1. Built-in defaults from DefaultConfig()
//  2. YAML configuration file at configPath (if non-empty and the file exists)
//  3. Environment variables prefixed with AEGIS_
//
// Environment variables are transformed so that AEGIS_SERVER_ADDR maps to the
// koanf key "server.addr". Double underscores can be used to represent a
// literal underscore in the key (e.g. AEGIS_CACHE_SQLITE__PATH -> cache.sqlite_path
// is NOT used; instead single underscores map to dots for nesting).
//
// Note: because some koanf keys contain underscores (e.g. "read_timeout_sec"),
// the env provider uses a simple lowercasing + underscore-to-dot transform.
// For keys with underscores in their names, the env var name uses the flat
// representation: AEGIS_SERVER_READ_TIMEOUT_SEC -> server.read.timeout.sec
// which koanf will match against flattened keys.
func LoadConfig(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Layer 1: defaults.
	defaults := DefaultConfig()
	if err := k.Load(structs.Provider(defaults, "koanf"), nil); err != nil {
		return nil, fmt.Errorf("loading default config: %w", err)
	}

	// Layer 2: YAML file (optional).
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			slog.Info("loading config file", "path", configPath)
			if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("loading config file %s: %w", configPath, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("checking config file %s: %w", configPath, err)
		} else {
			slog.Warn("config file not found, using defaults and env", "path", configPath)
		}
	}

	// Layer 3: environment variables.
	// AEGIS_SERVER_ADDR -> server.addr
	envProvider := env.Provider("AEGIS_", ".", func(s string) string {
		// Strip the AEGIS_ prefix, lowercase, and replace _ with . for nesting.
		key := strings.TrimPrefix(s, "AEGIS_")
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "_", ".")
		return key
	})
	if err := k.Load(envProvider, nil); err != nil {
		return nil, fmt.Errorf("loading env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}
