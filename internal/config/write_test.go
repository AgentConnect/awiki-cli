package config

import (
	"path/filepath"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/testenv"
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

func TestUpdateDIDDomainCreatesConfigAndNormalizesValue(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	if err := UpdateDIDDomain(paths, " Tenant.Example. "); err != nil {
		t.Fatalf("UpdateDIDDomain() error = %v", err)
	}
	fileConfig, exists, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatal("expected config file to exist")
	}
	if fileConfig.SchemaVersion != ConfigSchemaVersion {
		t.Fatalf("schema version = %d, want %d", fileConfig.SchemaVersion, ConfigSchemaVersion)
	}
	if fileConfig.Services.DIDDomain != "tenant.example" {
		t.Fatalf("did_domain = %q, want tenant.example", fileConfig.Services.DIDDomain)
	}
}

func TestUpdateDIDDomainPreservesExistingServiceSettings(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	initial := FileConfig{}
	initial.Services.ServiceBaseURL = testenv.SubdomainURL("platform")
	initial.Services.ANPServiceEndpoint = testenv.SubdomainURL("rpc") + "/anp"
	initial.Services.ANPServiceDID = "did:wba:" + testenv.Subdomain("rpc")
	if err := WriteFileConfig(paths.ConfigFile, initial); err != nil {
		t.Fatalf("WriteFileConfig() error = %v", err)
	}
	if err := UpdateDIDDomain(paths, "tenant.example"); err != nil {
		t.Fatalf("UpdateDIDDomain() error = %v", err)
	}
	fileConfig, _, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Services.ServiceBaseURL != initial.Services.ServiceBaseURL {
		t.Fatalf("service_base_url = %q, want %q", fileConfig.Services.ServiceBaseURL, initial.Services.ServiceBaseURL)
	}
	if fileConfig.Services.ANPServiceEndpoint != initial.Services.ANPServiceEndpoint {
		t.Fatalf("anp_service_endpoint = %q, want %q", fileConfig.Services.ANPServiceEndpoint, initial.Services.ANPServiceEndpoint)
	}
	if fileConfig.Services.ANPServiceDID != initial.Services.ANPServiceDID {
		t.Fatalf("anp_service_did = %q, want %q", fileConfig.Services.ANPServiceDID, initial.Services.ANPServiceDID)
	}
	if fileConfig.Services.DIDDomain != "tenant.example" {
		t.Fatalf("did_domain = %q, want tenant.example", fileConfig.Services.DIDDomain)
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
	if err := UpdateOpenClawSettings(paths, &hookURL); err != nil {
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
}

func TestHostNotifyMutatorsWriteSinkAndHermesConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	if err := UpdateHostNotifySink(paths, "hermes"); err != nil {
		t.Fatalf("UpdateHostNotifySink() error = %v", err)
	}
	notifyURL := "http://127.0.0.1:8765/notify/host-event"
	deliver := "telegram"
	if err := UpdateHermesSettings(paths, &notifyURL, &deliver); err != nil {
		t.Fatalf("UpdateHermesSettings() error = %v", err)
	}
	if err := SetHermesSecret(paths, "secret-123"); err != nil {
		t.Fatalf("SetHermesSecret() error = %v", err)
	}

	fileConfig, _, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.Sink != "hermes" {
		t.Fatalf("host notify sink = %q, want hermes", fileConfig.Runtime.HostNotify.Sink)
	}
	if fileConfig.Runtime.HostNotify.Hermes.NotifyURL != notifyURL {
		t.Fatalf("notify_url = %q, want %q", fileConfig.Runtime.HostNotify.Hermes.NotifyURL, notifyURL)
	}
	if fileConfig.Runtime.HostNotify.Hermes.Deliver != "telegram" {
		t.Fatalf("deliver = %q, want telegram", fileConfig.Runtime.HostNotify.Hermes.Deliver)
	}
	if fileConfig.Runtime.HostNotify.Hermes.Secret != "secret-123" {
		t.Fatalf("hermes secret = %q, want secret-123", fileConfig.Runtime.HostNotify.Hermes.Secret)
	}

	if err := ClearHermesSecret(paths); err != nil {
		t.Fatalf("ClearHermesSecret() error = %v", err)
	}
	fileConfig, _, err = ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.Hermes.Secret != "" {
		t.Fatalf("hermes secret = %q, want empty string", fileConfig.Runtime.HostNotify.Hermes.Secret)
	}
}

func TestConfigureHermesHostNotifyWritesOneShotConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	secret := "secret-setup"
	if err := ConfigureHermesHostNotify(paths, "http://127.0.0.1:8765/notify/host-event", &secret, "telegram", true); err != nil {
		t.Fatalf("ConfigureHermesHostNotify() error = %v", err)
	}

	fileConfig, _, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.Sink != "hermes" {
		t.Fatalf("host notify sink = %q, want hermes", fileConfig.Runtime.HostNotify.Sink)
	}
	if fileConfig.Runtime.HostNotify.Enabled == nil || !*fileConfig.Runtime.HostNotify.Enabled {
		t.Fatalf("host notify enabled = %#v, want true", fileConfig.Runtime.HostNotify.Enabled)
	}
	if fileConfig.Runtime.HostNotify.Hermes.NotifyURL != "http://127.0.0.1:8765/notify/host-event" {
		t.Fatalf("notify_url = %q", fileConfig.Runtime.HostNotify.Hermes.NotifyURL)
	}
	if fileConfig.Runtime.HostNotify.Hermes.Deliver != "telegram" {
		t.Fatalf("deliver = %q, want telegram", fileConfig.Runtime.HostNotify.Hermes.Deliver)
	}
	if fileConfig.Runtime.HostNotify.Hermes.Secret != secret {
		t.Fatalf("hermes secret = %q, want %q", fileConfig.Runtime.HostNotify.Hermes.Secret, secret)
	}
	if fileConfig.Runtime.HostNotify.LegacyWebhook.NotifyURL != "http://127.0.0.1:8765/notify/host-event" {
		t.Fatalf("legacy webhook notify_url = %q", fileConfig.Runtime.HostNotify.LegacyWebhook.NotifyURL)
	}
	if fileConfig.Runtime.HostNotify.LegacyWebhook.Secret != secret {
		t.Fatalf("legacy webhook secret = %q, want %q", fileConfig.Runtime.HostNotify.LegacyWebhook.Secret, secret)
	}
}

func TestUpdateHostNotifySinkNormalizesWebhookAliasToHermes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := Paths{ConfigFile: filepath.Join(root, "config.yaml")}
	if err := UpdateHostNotifySink(paths, "webhook"); err != nil {
		t.Fatalf("UpdateHostNotifySink() error = %v", err)
	}
	fileConfig, _, err := ReadFileConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if fileConfig.Runtime.HostNotify.Sink != "hermes" {
		t.Fatalf("host notify sink = %q, want hermes", fileConfig.Runtime.HostNotify.Sink)
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
