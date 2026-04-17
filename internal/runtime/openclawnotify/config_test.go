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
	if err := os.WriteFile(configPath, []byte(`{"gateway":{"port":25307},"hooks":{"path":"/custom-hooks","token":"hook-token"}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv(ConfigPathEnv, configPath)
	probe := ProbeOpenClawConfig(DefaultGatewayPort, DefaultHooksPath)
	if probe.Gateway.Port != 25307 {
		t.Fatalf("probe.Gateway.Port = %d, want 25307", probe.Gateway.Port)
	}
	if probe.Gateway.Source != "openclaw_config" {
		t.Fatalf("probe.Gateway.Source = %q, want openclaw_config", probe.Gateway.Source)
	}
	if probe.Gateway.ConfigPath != configPath {
		t.Fatalf("probe.Gateway.ConfigPath = %q, want %q", probe.Gateway.ConfigPath, configPath)
	}
	if probe.HooksPath.Path != "/custom-hooks" {
		t.Fatalf("probe.HooksPath.Path = %q, want /custom-hooks", probe.HooksPath.Path)
	}
	if probe.HooksPath.Source != "openclaw_config" {
		t.Fatalf("probe.HooksPath.Source = %q, want openclaw_config", probe.HooksPath.Source)
	}
	if probe.HookToken != "hook-token" {
		t.Fatalf("probe.HookToken = %q, want hook-token", probe.HookToken)
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
	if err := os.WriteFile(openclawConfigPath, []byte(`{"gateway":{"port":25307},"hooks":{"path":"/custom-hooks","token":"hook-token"}}`), 0o600); err != nil {
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
	if settings.HookURL != "http://127.0.0.1:25307/custom-hooks/agent" {
		t.Fatalf("settings.HookURL = %q, want auto-detected port and path", settings.HookURL)
	}
	if settings.HookURLSource != "auto_detected" {
		t.Fatalf("settings.HookURLSource = %q, want auto_detected", settings.HookURLSource)
	}
	if settings.DetectedWebhookPort != 25307 {
		t.Fatalf("settings.DetectedWebhookPort = %d, want 25307", settings.DetectedWebhookPort)
	}
	if settings.DetectedWebhookPath != "/custom-hooks/agent" {
		t.Fatalf("settings.DetectedWebhookPath = %q, want /custom-hooks/agent", settings.DetectedWebhookPath)
	}
	if settings.DetectedWebhookPathInfo.Source != "openclaw_config" {
		t.Fatalf("settings.DetectedWebhookPathInfo.Source = %q, want openclaw_config", settings.DetectedWebhookPathInfo.Source)
	}
	if !settings.TokenConfigured {
		t.Fatal("settings.TokenConfigured = false, want true")
	}
	if settings.TokenSource != "openclaw_config" {
		t.Fatalf("settings.TokenSource = %q, want openclaw_config", settings.TokenSource)
	}
	if settings.Token != "hook-token" {
		t.Fatalf("settings.Token = %q, want hook-token", settings.Token)
	}
}

func TestResolveSettingsPrefersConfigTokenOverEnvironmentAndOpenClawConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runtime:\n  host_notify:\n    sink: openclaw\n    openclaw:\n      token: config-token\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	openclawConfigPath := filepath.Join(root, "openclaw.json")
	if err := os.WriteFile(openclawConfigPath, []byte(`{"gateway":{"port":25307},"hooks":{"token":"hook-token"}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv(ConfigPathEnv, openclawConfigPath)
	t.Setenv(HookTokenEnv, "env-token")

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
	if settings.Token != "config-token" {
		t.Fatalf("settings.Token = %q, want config-token", settings.Token)
	}
	if settings.TokenSource != "config_file" {
		t.Fatalf("settings.TokenSource = %q, want config_file", settings.TokenSource)
	}
}

func TestResolveSettingsPrefersEnvironmentTokenOverOpenClawConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runtime:\n  host_notify:\n    sink: openclaw\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	openclawConfigPath := filepath.Join(root, "openclaw.json")
	if err := os.WriteFile(openclawConfigPath, []byte(`{"gateway":{"port":25307},"hooks":{"token":"hook-token"}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv(ConfigPathEnv, openclawConfigPath)
	t.Setenv(HookTokenEnv, "env-token")

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
	if settings.Token != "env-token" {
		t.Fatalf("settings.Token = %q, want env-token", settings.Token)
	}
	if settings.TokenSource != "environment" {
		t.Fatalf("settings.TokenSource = %q, want environment", settings.TokenSource)
	}
}
