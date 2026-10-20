package crushfling

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EndpointConfigFileName is written inside the data folder.
const EndpointConfigFileName = "api.json"

// LoadEndpointConfig reads <dataDir>/api.json, creating it with the defaults
// when it does not exist. Missing keys fall back to the defaults, so a partial
// file is safe.
func LoadEndpointConfig(dataDir string) (EndpointConfig, error) {
	cfg := DefaultEndpointConfig()
	if strings.TrimSpace(dataDir) == "" {
		return cfg, nil
	}
	path := filepath.Join(dataDir, EndpointConfigFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if werr := writeEndpointConfig(path, cfg); werr != nil {
				return cfg, fmt.Errorf("write %s: %w", path, werr)
			}
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return DefaultEndpointConfig(), fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return normalizeEndpointConfig(cfg), nil
}

func normalizeEndpointConfig(cfg EndpointConfig) EndpointConfig {
	def := DefaultEndpointConfig()
	cfg.AccountPath = normalizePath(cfg.AccountPath, def.AccountPath)
	// Empty CSRFPath/MagicLinkPath means "discover it from the page", so they
	// are allowed to stay empty.
	cfg.CSRFPath = normalizePath(cfg.CSRFPath, "")
	cfg.MagicLinkPath = normalizePath(cfg.MagicLinkPath, "")
	cfg.LivewirePath = normalizePath(cfg.LivewirePath, def.LivewirePath)
	cfg.UploadPath = normalizePath(cfg.UploadPath, def.UploadPath)
	cfg.PublicProfilePath = normalizeProfilePath(cfg.PublicProfilePath, def.PublicProfilePath)
	if len(cfg.ProfilePaths) == 0 {
		cfg.ProfilePaths = def.ProfilePaths
	}
	return cfg
}

// normalizeProfilePath keeps the {username} placeholder intact (it is a path
// template, not a URL to request as-is).
func normalizeProfilePath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func normalizePath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func writeEndpointConfig(path string, cfg EndpointConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

// ensureEndpointConfigFile is called from EnsureDataLayout; it never overwrites
// an existing file, so operator calibration survives upgrades.
func ensureEndpointConfigFile(dataDir string) error {
	path := filepath.Join(dataDir, EndpointConfigFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return writeEndpointConfig(path, DefaultEndpointConfig())
}
