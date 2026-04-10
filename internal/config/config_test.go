package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, root string, cfg FileConfig) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.json"), raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}

// Env should override config for no_color when AWIKI_NO_COLOR is set.
func TestResolve_NoColorEnvOverridesConfig(t *testing.T) {
	root := t.TempDir()
	cfg := FileConfig{}
	falseVal := false
	cfg.Output.NoColor = &falseVal
	writeTestConfig(t, root, cfg)

	t.Setenv("AWIKI_HOME", root)
	t.Setenv("AWIKI_NO_COLOR", "1")

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !resolved.NoColor {
		t.Fatalf("resolved.NoColor = false, want true")
	}
	source := resolved.Sources["no_color"]
	if source.Source != "canonical_env" {
		t.Fatalf("resolved.Sources[no_color].Source = %q, want %q", source.Source, "canonical_env")
	}
	if source.Key != "AWIKI_NO_COLOR" {
		t.Fatalf("resolved.Sources[no_color].Key = %q, want %q", source.Key, "AWIKI_NO_COLOR")
	}
}

// When AWIKI_NO_COLOR is not set, config value should be used.
func TestResolve_NoColorFromConfigWhenNoEnv(t *testing.T) {
	root := t.TempDir()
	cfg := FileConfig{}
	falseVal := false
	cfg.Output.NoColor = &falseVal
	writeTestConfig(t, root, cfg)

	t.Setenv("AWIKI_HOME", root)

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

func TestResolveRootDir_PrefersEnvOverDefaultRoot(t *testing.T) {
	home := t.TempDir()
	envTarget := filepath.Join(home, "env-root")

	t.Setenv("AWIKI_HOME", envTarget)

	root, source := resolveRootDir(home)
	if root != envTarget {
		t.Fatalf("resolveRootDir() root = %q, want %q from AWIKI_HOME", root, envTarget)
	}
	if source.Source != "canonical_env" || source.Key != "AWIKI_HOME" {
		t.Fatalf("resolveRootDir() source = %+v, want canonical_env AWIKI_HOME", source)
	}
}

func TestResolveRootDir_UsesDefaultRootWhenNoEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWIKI_HOME", "")

	root, source := resolveRootDir(home)
	expected := DefaultRootDir(home)
	if root != expected {
		t.Fatalf("resolveRootDir() root = %q, want %q from default root", root, expected)
	}
	if source.Source != "default" || source.Key != "" {
		t.Fatalf("resolveRootDir() source = %+v, want default with empty key", source)
	}
}
