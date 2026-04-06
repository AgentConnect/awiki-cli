package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func UpdateRuntimeSettings(paths Paths, mode string, socketPath string) error {
	configPath := paths.ConfigFile
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	fileConfig, _, err := loadFileConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	fileConfig.Runtime.Mode = mode
	if socketPath != "" {
		fileConfig.Runtime.SocketPath = socketPath
	}
	return writeFileConfig(configPath, fileConfig)
}

func writeFileConfig(path string, fileConfig FileConfig) error {
	raw, err := yaml.Marshal(fileConfig)
	if err != nil {
		return fmt.Errorf("marshal config yaml: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write config yaml: %w", err)
	}
	return nil
}
