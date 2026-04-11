package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func EnsureConfigSchemaVersion(path string) error {
	fileConfig, exists, err := ReadFileConfig(path)
	if err != nil {
		return fmt.Errorf("read config yaml: %w", err)
	}
	if !exists {
		return nil
	}
	fileConfig.SchemaVersion = ConfigSchemaVersion
	if err := WriteFileConfig(path, fileConfig); err != nil {
		return fmt.Errorf("write config yaml: %w", err)
	}
	return nil
}

func UpdateRuntimeSettings(paths Paths, mode string, socketPath string) error {
	configPath := paths.ConfigFile
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	fileConfig, _, err := ReadFileConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	fileConfig.Runtime.Mode = mode
	if socketPath != "" {
		fileConfig.Runtime.SocketPath = socketPath
	}
	return WriteFileConfig(configPath, fileConfig)
}

func WriteFileConfig(path string, fileConfig FileConfig) error {
	fileConfig.SchemaVersion = ConfigSchemaVersion
	raw, err := yaml.Marshal(fileConfig)
	if err != nil {
		return fmt.Errorf("marshal config yaml: %w", err)
	}
	if err := writeAtomicFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write config yaml: %w", err)
	}
	return nil
}

func writeAtomicFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp config file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp config file: %w", err)
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("chmod temp config file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}
	cleanup = false

	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open config dir: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync config dir: %w", err)
	}
	return nil
}
