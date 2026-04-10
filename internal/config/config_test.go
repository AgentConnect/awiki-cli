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

func TestLoadHomePointer_MissingFileReturnsEmpty(t *testing.T) {
	home := t.TempDir()

	root, err := LoadHomePointer(home)
	if err != nil {
		t.Fatalf("LoadHomePointer() error = %v", err)
	}
	if root != "" {
		t.Fatalf("LoadHomePointer() = %q, want empty string for missing pointer", root)
	}
}

func TestLoadHomePointer_JSONPointerRoundTrip(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "custom-root")

	if err := WriteHomePointer(home, target); err != nil {
		t.Fatalf("WriteHomePointer() error = %v", err)
	}

	root, err := LoadHomePointer(home)
	if err != nil {
		t.Fatalf("LoadHomePointer() error = %v", err)
	}
	if root != target {
		t.Fatalf("LoadHomePointer() = %q, want %q", root, target)
	}
}

func TestResolveRootDir_PrefersEnvOverPointer(t *testing.T) {
	home := t.TempDir()
	pointerTarget := filepath.Join(home, "pointer-root")
	envTarget := filepath.Join(home, "env-root")

	if err := WriteHomePointer(home, pointerTarget); err != nil {
		t.Fatalf("WriteHomePointer() error = %v", err)
	}
	t.Setenv("AWIKI_HOME", envTarget)

	root, source := resolveRootDir(home)
	if root != envTarget {
		t.Fatalf("resolveRootDir() root = %q, want %q from AWIKI_HOME", root, envTarget)
	}
	if source.Source != "canonical_env" || source.Key != "AWIKI_HOME" {
		t.Fatalf("resolveRootDir() source = %+v, want canonical_env AWIKI_HOME", source)
	}
}

func TestResolveRootDir_UsesHomePointerWhenNoEnv(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "pointer-root")

	if err := WriteHomePointer(home, target); err != nil {
		t.Fatalf("WriteHomePointer() error = %v", err)
	}
	t.Setenv("AWIKI_HOME", "")

	root, source := resolveRootDir(home)
	if root != target {
		t.Fatalf("resolveRootDir() root = %q, want %q from home pointer", root, target)
	}
	if source.Source != "home_pointer" || source.Key != "home.json" {
		t.Fatalf("resolveRootDir() source = %+v, want home_pointer home.json", source)
	}
}
