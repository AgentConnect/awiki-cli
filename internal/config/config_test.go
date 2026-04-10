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

func TestResolveSetsWorkspaceHomeDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWIKI_WORKSPACE_HOME", root)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Paths.WorkspaceHomeDir != root {
		t.Fatalf("workspace home dir = %q, want %q", resolved.Paths.WorkspaceHomeDir, root)
	}
	if resolved.Paths.RootDir != root {
		t.Fatalf("root dir = %q, want %q", resolved.Paths.RootDir, root)
	}
	if resolved.Paths.ConfigDir != root {
		t.Fatalf("config dir = %q, want %q", resolved.Paths.ConfigDir, root)
	}
	if resolved.Paths.IdentityDir != filepath.Join(root, "identities") {
		t.Fatalf("identity dir = %q", resolved.Paths.IdentityDir)
	}
	if resolved.Paths.DatabaseFile != filepath.Join(root, "data", "awiki-cli.db") {
		t.Fatalf("database file = %q", resolved.Paths.DatabaseFile)
	}
	if resolved.Paths.StateDir != filepath.Join(root, "runtime") {
		t.Fatalf("state dir = %q", resolved.Paths.StateDir)
	}
	if resolved.Paths.CacheDir != filepath.Join(root, "cache") {
		t.Fatalf("cache dir = %q", resolved.Paths.CacheDir)
	}
	if resolved.Paths.LogsDir != filepath.Join(root, "logs") {
		t.Fatalf("logs dir = %q", resolved.Paths.LogsDir)
	}
	if resolved.RuntimeSocketPath != filepath.Join(root, "runtime", "message-daemon.sock") {
		t.Fatalf("runtime socket path = %q", resolved.RuntimeSocketPath)
	}
}

func TestResolveSupportsAWIKIHomeAlias(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWIKI_HOME", root)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Paths.WorkspaceHomeDir != root {
		t.Fatalf("workspace home dir = %q, want %q", resolved.Paths.WorkspaceHomeDir, root)
	}
	source := resolved.Sources["workspace_home_dir"]
	if source.Source != "canonical_env" {
		t.Fatalf("resolved.Sources[workspace_home_dir].Source = %q, want %q", source.Source, "canonical_env")
	}
	if source.Key != "AWIKI_HOME" {
		t.Fatalf("resolved.Sources[workspace_home_dir].Key = %q, want %q", source.Key, "AWIKI_HOME")
	}
}
