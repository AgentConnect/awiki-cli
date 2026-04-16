package openclawnotify

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestProbeGatewayPortReadsEnvBeforeConfig(t *testing.T) {
	t.Parallel()

	t.Setenv(GatewayPortEnv, "25307")
	probe := ProbeGatewayPort(DefaultGatewayPort)
	if probe.Port != 25307 {
		t.Fatalf("probe.Port = %d, want 25307", probe.Port)
	}
	if probe.Source != "environment" {
		t.Fatalf("probe.Source = %q, want environment", probe.Source)
	}
}

func TestProbeGatewayPortReadsOpenClawConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{"gateway":{"port":25307}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv(ConfigPathEnv, configPath)
	probe := ProbeGatewayPort(DefaultGatewayPort)
	if probe.Port != 25307 {
		t.Fatalf("probe.Port = %d, want 25307", probe.Port)
	}
	if probe.Source != "openclaw_config" {
		t.Fatalf("probe.Source = %q, want openclaw_config", probe.Source)
	}
	if probe.ConfigPath != configPath {
		t.Fatalf("probe.ConfigPath = %q, want %q", probe.ConfigPath, configPath)
	}
}

func TestResolveSettingsUsesAutoDetectedHookURLWhenConfigHookURLUnset(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runtime:\n  host_notify:\n    sink: openclaw\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	openclawConfigPath := filepath.Join(root, "openclaw.json")
	if err := os.WriteFile(openclawConfigPath, []byte(`{"gateway":{"port":25307}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv(ConfigPathEnv, openclawConfigPath)

	resolved := &appconfig.Resolved{
		Paths:                     appconfig.Paths{ConfigFile: configPath},
		HostNotifySink:            "openclaw",
		HostNotifyOpenClawHookURL: "http://127.0.0.1:18789/hooks/agent",
		Sources: map[string]appconfig.ValueSource{
			"host_notify_openclaw_hook_url": {Source: "default", Value: "http://127.0.0.1:18789/hooks/agent"},
		},
	}

	settings, err := ResolveSettings(resolved)
	if err != nil {
		t.Fatalf("ResolveSettings() error = %v", err)
	}
	if settings.HookURL != "http://127.0.0.1:25307/hooks/agent" {
		t.Fatalf("settings.HookURL = %q, want auto-detected port", settings.HookURL)
	}
	if settings.HookURLSource != "auto_detected" {
		t.Fatalf("settings.HookURLSource = %q, want auto_detected", settings.HookURLSource)
	}
	if settings.DetectedWebhookPort != 25307 {
		t.Fatalf("settings.DetectedWebhookPort = %d, want 25307", settings.DetectedWebhookPort)
	}
}
