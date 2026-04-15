package config

import (
	"path/filepath"
	"testing"
)

func TestUpdateRuntimeSettingsWritesConfigSchemaVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
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

func TestUpdateRuntimeListenerSettingsWritesBooleanPointers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	enabled := false
	autoInstall := false
	autoStart := true
	if err := UpdateRuntimeListenerSettings(paths, &enabled, &autoInstall, &autoStart); err != nil {
		t.Fatalf("UpdateRuntimeListenerSettings() error = %v", err)
	}
	fileConfig, exists, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatal("expected config file to exist")
	}
	if fileConfig.Runtime.Listener.Enabled == nil || *fileConfig.Runtime.Listener.Enabled {
		t.Fatalf("listener.enabled = %#v, want false", fileConfig.Runtime.Listener.Enabled)
	}
	if fileConfig.Runtime.Listener.AutoInstall == nil || *fileConfig.Runtime.Listener.AutoInstall {
		t.Fatalf("listener.auto_install = %#v, want false", fileConfig.Runtime.Listener.AutoInstall)
	}
	if fileConfig.Runtime.Listener.AutoStart == nil || !*fileConfig.Runtime.Listener.AutoStart {
		t.Fatalf("listener.auto_start = %#v, want true", fileConfig.Runtime.Listener.AutoStart)
	}
}

func TestOpenClawTokenMutatorsWriteAndClearToken(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	if err := SetOpenClawToken(paths, "token-123"); err != nil {
		t.Fatalf("SetOpenClawToken() error = %v", err)
	}
	fileConfig, _, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.OpenClaw.Token != "token-123" {
		t.Fatalf("openclaw token = %q, want token-123", fileConfig.Runtime.HostNotify.OpenClaw.Token)
	}
	if err := ClearOpenClawToken(paths); err != nil {
		t.Fatalf("ClearOpenClawToken() error = %v", err)
	}
	fileConfig, _, err = ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.OpenClaw.Token != "" {
		t.Fatalf("openclaw token = %q, want empty string", fileConfig.Runtime.HostNotify.OpenClaw.Token)
	}
}

func TestHostNotifyMutatorsWriteSinkAndOpenClawConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	if err := UpdateHostNotifySink(paths, "openclaw"); err != nil {
		t.Fatalf("UpdateHostNotifySink() error = %v", err)
	}
	hookURL := "http://127.0.0.1:18789/hooks/agent"
	agentID := "notify"
	hookName := "AWiki"
	if err := UpdateOpenClawSettings(paths, &hookURL, &agentID, &hookName); err != nil {
		t.Fatalf("UpdateOpenClawSettings() error = %v", err)
	}
	fileConfig, _, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.Sink != "openclaw" {
		t.Fatalf("host notify sink = %q, want openclaw", fileConfig.Runtime.HostNotify.Sink)
	}
	if fileConfig.Runtime.HostNotify.Enabled == nil || !*fileConfig.Runtime.HostNotify.Enabled {
		t.Fatalf("host notify enabled = %#v, want true", fileConfig.Runtime.HostNotify.Enabled)
	}
	if fileConfig.Runtime.HostNotify.OpenClaw.HookURL != hookURL {
		t.Fatalf("hook_url = %q, want %q", fileConfig.Runtime.HostNotify.OpenClaw.HookURL, hookURL)
	}
	if fileConfig.Runtime.HostNotify.OpenClaw.AgentID != agentID {
		t.Fatalf("agent_id = %q, want %q", fileConfig.Runtime.HostNotify.OpenClaw.AgentID, agentID)
	}
	if fileConfig.Runtime.HostNotify.OpenClaw.HookName != hookName {
		t.Fatalf("hook_name = %q, want %q", fileConfig.Runtime.HostNotify.OpenClaw.HookName, hookName)
	}
}

func TestUpdateHostNotifyEnabledWritesBooleanPointer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	if err := UpdateHostNotifyEnabled(paths, false); err != nil {
		t.Fatalf("UpdateHostNotifyEnabled() error = %v", err)
	}
	fileConfig, _, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.Enabled == nil || *fileConfig.Runtime.HostNotify.Enabled {
		t.Fatalf("host notify enabled = %#v, want false", fileConfig.Runtime.HostNotify.Enabled)
	}
}
