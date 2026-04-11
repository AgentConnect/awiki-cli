package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHonorsExplicitFalseBoolFromConfigFile(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("output:\n  no_color: false\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

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
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  did_domain: awiki.test\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ANPServiceEndpoint != "https://awiki.test/anp-im/rpc" {
		t.Fatalf("resolved.ANPServiceEndpoint = %q, want %q", resolved.ANPServiceEndpoint, "https://awiki.test/anp-im/rpc")
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

func TestResolveHonorsServiceBaseURLFromConfigFile(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  service_base_url: https://awiki.test/\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != "https://awiki.test" {
		t.Fatalf("resolved.ServiceBaseURL = %q, want https://awiki.test", resolved.ServiceBaseURL)
	}
	if source := resolved.Sources["service_base_url"]; source.Source != "config_file" {
		t.Fatalf("resolved.Sources[service_base_url].Source = %q, want config_file", source.Source)
	}
}

func TestResolveSetsWorkspaceHomeDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", root)

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
	if resolved.Paths.ConfigFile != filepath.Join(root, "config.yaml") {
		t.Fatalf("config file = %q", resolved.Paths.ConfigFile)
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
	if resolved.RuntimeMode != "websocket" {
		t.Fatalf("runtime mode = %q, want websocket", resolved.RuntimeMode)
	}
}

func TestResolveIgnoresDeprecatedWorkspaceEnv(t *testing.T) {
	t.Setenv("AWIKI_WORKSPACE_HOME", t.TempDir())

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Paths.WorkspaceHomeDir == "" {
		t.Fatal("workspace home dir should still resolve when deprecated env is present")
	}
}

func TestResolveIgnoresDeprecatedBusinessEnv(t *testing.T) {
	t.Setenv("AWIKI_USER_SERVICE_URL", "https://awiki.test")

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != "https://awiki.ai" {
		t.Fatalf("resolved.ServiceBaseURL = %q, want default value", resolved.ServiceBaseURL)
	}
}

func TestResolveAllowsLegacyConfigJSONForUpgrade(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceHome, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ConfigExists {
		t.Fatal("resolved.ConfigExists = true, want false until upgrade migrates legacy config")
	}
	if resolved.Paths.ConfigFile != filepath.Join(workspaceHome, "config.yaml") {
		t.Fatalf("config file = %q", resolved.Paths.ConfigFile)
	}
}

func TestResolveRejectsDeprecatedServiceURLFieldsInConfigYAML(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  user_service_url: https://awiki.test\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	_, err := Resolve(Overrides{})
	if err == nil {
		t.Fatal("Resolve() error = nil, want deprecated config field error")
	}
	if !strings.Contains(err.Error(), "services.user_service_url") {
		t.Fatalf("Resolve() error = %q, want deprecated config field name", err.Error())
	}
}
