package crushfling

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEndpointConfigWritesDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadEndpointConfig(dir)
	if err != nil {
		t.Fatalf("LoadEndpointConfig: %v", err)
	}
	// The sign-in flow is discovered from the page, so the magic-link path is
	// deliberately empty; the NextAuth paths answer 404 on the live site.
	if cfg.MagicLinkPath != "" {
		t.Fatalf("MagicLinkPath = %q, want empty (discovered)", cfg.MagicLinkPath)
	}
	if cfg.CSRFPath != "" {
		t.Fatalf("CSRFPath = %q, want empty (meta tag on the account page)", cfg.CSRFPath)
	}
	if cfg.AccountPath != "/account" {
		t.Fatalf("AccountPath = %q", cfg.AccountPath)
	}
	if len(cfg.ProfilePaths) == 0 {
		t.Fatal("ProfilePaths should default to a non-empty list")
	}
	if _, err := os.Stat(filepath.Join(dir, EndpointConfigFileName)); err != nil {
		t.Fatalf("api.json was not created: %v", err)
	}
}

func TestLoadEndpointConfigPartialOverrideKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	body := `{"magic_link_path": "api/auth/custom", "profile_paths": ["/api/custom-profile"]}`
	if err := os.WriteFile(filepath.Join(dir, EndpointConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write api.json: %v", err)
	}

	cfg, err := LoadEndpointConfig(dir)
	if err != nil {
		t.Fatalf("LoadEndpointConfig: %v", err)
	}
	if cfg.MagicLinkPath != "/api/auth/custom" {
		t.Fatalf("MagicLinkPath = %q, want the normalized override", cfg.MagicLinkPath)
	}
	if cfg.CSRFPath != "" {
		t.Fatalf("CSRFPath = %q, want empty (discovered)", cfg.CSRFPath)
	}
	if len(cfg.ProfilePaths) != 1 || cfg.ProfilePaths[0] != "/api/custom-profile" {
		t.Fatalf("ProfilePaths = %#v", cfg.ProfilePaths)
	}
}

func TestLoadEndpointConfigRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, EndpointConfigFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write api.json: %v", err)
	}
	if _, err := LoadEndpointConfig(dir); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}
