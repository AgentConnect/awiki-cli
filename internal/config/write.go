package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/durablefs"
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
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		fileConfig.Runtime.Mode = mode
		if socketPath != "" {
			fileConfig.Runtime.SocketPath = socketPath
		}
		return nil
	})
}

func UpdateActiveIdentity(paths Paths, identityName string) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		fileConfig.Identity.Active = strings.TrimSpace(identityName)
		return nil
	})
}

func UpdateDIDDomain(paths Paths, value string) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		normalized, err := NormalizeDIDDomain(value)
		if err != nil {
			return err
		}
		fileConfig.Services.DIDDomain = normalized
		return nil
	})
}

func UpdateRuntimeListenerSettings(paths Paths, enabled *bool, autoInstall *bool, autoStart *bool) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		if enabled != nil {
			fileConfig.Runtime.Listener.Enabled = boolPtr(*enabled)
		}
		if autoInstall != nil {
			fileConfig.Runtime.Listener.AutoInstall = boolPtr(*autoInstall)
		}
		if autoStart != nil {
			fileConfig.Runtime.Listener.AutoStart = boolPtr(*autoStart)
		}
		return nil
	})
}

func UpdateHostNotifySink(paths Paths, sink string) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		normalized := strings.ToLower(strings.TrimSpace(sink))
		if normalized == "webhook" {
			normalized = "hermes"
		}
		fileConfig.Runtime.HostNotify.Sink = normalized
		fileConfig.Runtime.HostNotify.Enabled = boolPtr(true)
		return nil
	})
}

func UpdateHostNotifyEnabled(paths Paths, enabled bool) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		fileConfig.Runtime.HostNotify.Enabled = boolPtr(enabled)
		return nil
	})
}

func UpdateOpenClawSettings(paths Paths, hookURL *string) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		if hookURL != nil {
			fileConfig.Runtime.HostNotify.OpenClaw.HookURL = strings.TrimSpace(*hookURL)
		}
		return nil
	})
}

func UpdateHermesSettings(paths Paths, notifyURL *string, deliver *string) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		if notifyURL != nil {
			value := strings.TrimSpace(*notifyURL)
			fileConfig.Runtime.HostNotify.Hermes.NotifyURL = value
			fileConfig.Runtime.HostNotify.LegacyWebhook.NotifyURL = value
		}
		if deliver != nil {
			fileConfig.Runtime.HostNotify.Hermes.Deliver = strings.ToLower(strings.TrimSpace(*deliver))
		}
		return nil
	})
}

func ConfigureHermesHostNotify(paths Paths, notifyURL string, secret *string, deliver string, enabled bool) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		value := strings.TrimSpace(notifyURL)
		fileConfig.Runtime.HostNotify.Enabled = boolPtr(enabled)
		fileConfig.Runtime.HostNotify.Sink = "hermes"
		fileConfig.Runtime.HostNotify.Hermes.NotifyURL = value
		fileConfig.Runtime.HostNotify.Hermes.Deliver = strings.ToLower(strings.TrimSpace(deliver))
		fileConfig.Runtime.HostNotify.LegacyWebhook.NotifyURL = value
		if secret != nil {
			trimmed := strings.TrimSpace(*secret)
			fileConfig.Runtime.HostNotify.Hermes.Secret = trimmed
			fileConfig.Runtime.HostNotify.LegacyWebhook.Secret = trimmed
		}
		return nil
	})
}

func SetOpenClawToken(paths Paths, token string) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		fileConfig.Runtime.HostNotify.OpenClaw.Token = token
		return nil
	})
}

func SetHermesSecret(paths Paths, secret string) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		fileConfig.Runtime.HostNotify.Hermes.Secret = secret
		fileConfig.Runtime.HostNotify.LegacyWebhook.Secret = secret
		return nil
	})
}

func ClearHermesSecret(paths Paths) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		fileConfig.Runtime.HostNotify.Hermes.Secret = ""
		fileConfig.Runtime.HostNotify.LegacyWebhook.Secret = ""
		return nil
	})
}

func ClearOpenClawToken(paths Paths) error {
	return updateFileConfig(paths.ConfigFile, func(fileConfig *FileConfig) error {
		fileConfig.Runtime.HostNotify.OpenClaw.Token = ""
		return nil
	})
}

func updateFileConfig(configPath string, mutate func(fileConfig *FileConfig) error) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	fileConfig, _, err := ReadFileConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := mutate(&fileConfig); err != nil {
		return err
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

	if err := durablefs.SyncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync config dir: %w", err)
	}
	return nil
}

func boolPtr(value bool) *bool {
	result := value
	return &result
}
