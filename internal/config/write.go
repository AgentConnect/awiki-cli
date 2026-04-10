package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// UpdateRuntimeSettings updates the runtime.mode in config.json and ensures the
// config directory exists. The socketPath parameter is currently ignored and
// kept only for forward-compatibility with potential future extensions.
func UpdateRuntimeSettings(paths Paths, mode string, socketPath string) error {
	configPath := paths.ConfigFile
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	fileConfig, _, err := loadFileConfig(configPath)
	if err != nil {
		return err
	}
	fileConfig.Runtime.Mode = mode
	return WriteFileConfig(configPath, fileConfig)
}

// WriteFileConfig writes the given FileConfig to the target path using
// indented JSON and 0600 permissions.
func WriteFileConfig(path string, fileConfig FileConfig) error {
	raw, err := json.MarshalIndent(fileConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config json: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write config json: %w", err)
	}
	return nil
}
