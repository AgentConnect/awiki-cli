package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHonorsExplicitFalseBoolFromConfigFile(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(configDir, "config.yaml"),
		[]byte("output:\n  no_color: false\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CONFIG_DIR", configDir)
	t.Setenv("AWIKI_NO_COLOR", "1")

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.NoColor {
		t.Fatalf("resolved.NoColor = true, want false")
	}
	source := resolved.Sources["no_color"]
	if source.Source != "config_file" {
		t.Fatalf("resolved.Sources[no_color].Source = %q, want %q", source.Source, "config_file")
	}
	if source.Value != "false" {
		t.Fatalf("resolved.Sources[no_color].Value = %q, want %q", source.Value, "false")
	}
}

func TestResolveDerivesANPServiceDefaultsFromDIDDomain(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(configDir, "config.yaml"),
		[]byte("services:\n  did_domain: awiki.test\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CONFIG_DIR", configDir)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ANPServiceEndpoint != "https://awiki.test/message/rpc" {
		t.Fatalf("resolved.ANPServiceEndpoint = %q, want %q", resolved.ANPServiceEndpoint, "https://awiki.test/message/rpc")
	}
	if resolved.ANPServiceDID != "did:wba:awiki.test" {
		t.Fatalf("resolved.ANPServiceDID = %q, want %q", resolved.ANPServiceDID, "did:wba:awiki.test")
	}
	if source := resolved.Sources["anp_service_endpoint"]; source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[anp_service_endpoint].Source = %q, want %q", source.Source, "derived_default")
	}
	if source := resolved.Sources["anp_service_did"]; source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[anp_service_did].Source = %q, want %q", source.Source, "derived_default")
	}
}
