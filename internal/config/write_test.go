package config

import (
	"path/filepath"
	"testing"
)

func TestUpdateRuntimeSettingsWritesConfigSchemaVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.json")}
	if err := UpdateRuntimeSettings(paths, "websocket", filepath.Join(root, "runtime.sock")); err != nil {
		t.Fatalf("UpdateRuntimeSettings() error = %v", err)
	}
	fileConfig, exists, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected config file to exist")
	}
	if fileConfig.SchemaVersion != ConfigSchemaVersion {
		t.Fatalf("schema version = %d, want %d", fileConfig.SchemaVersion, ConfigSchemaVersion)
	}
	if fileConfig.Runtime.Mode != "websocket" {
		t.Fatalf("runtime mode = %q", fileConfig.Runtime.Mode)
	}
}
