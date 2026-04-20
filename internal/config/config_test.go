package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHonorsExplicitFalseBoolFromConfigFile(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("output:\n  no_color: false\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

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

func TestResolveDerivesServiceDefaultsFromDIDDomain(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  did_domain: awiki.test\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != "https://awiki.test" {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, "https://awiki.test")
	}
	if source := resolved.Sources["service_base_url"]; source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[service_base_url].Source = %q, want %q", source.Source, "derived_default")
	}
	if resolved.ANPServiceEndpoint != "https://awiki.test/anp-im/rpc" {
		t.Fatalf("resolved.ANPServiceEndpoint = %q, want %q", resolved.ANPServiceEndpoint, "https://awiki.test/anp-im/rpc")
	}
	if resolved.ANPServiceDID != "did:wba:awiki.test" {
		t.Fatalf("resolved.ANPServiceDID = %q, want %q", resolved.ANPServiceDID, "did:wba:awiki.test")
	}
	if source := resolved.Sources["anp_service_endpoint"]; source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[anp_service_endpoint].Source = %q, want %q", source.Source, "derived_default")
	}
	if source := resolved.Sources["anp_service_did"]; source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[anp_service_did].Source = %q, want %q", source.Source, "derived_default")
	}
}

func TestResolveHonorsServiceBaseURLFromConfigFile(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  service_base_url: https://awiki.test/\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != "https://awiki.test" {
		t.Fatalf("resolved.ServiceBaseURL = %q, want https://awiki.test", resolved.ServiceBaseURL)
	}
	if source := resolved.Sources["service_base_url"]; source.Source != "config_file" {
		t.Fatalf("resolved.Sources[service_base_url].Source = %q, want config_file", source.Source)
	}
}

func TestResolveSetsWorkspaceHomeDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", root)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Paths.WorkspaceHomeDir != root {
		t.Fatalf("workspace home dir = %q, want %q", resolved.Paths.WorkspaceHomeDir, root)
	}
	if resolved.Paths.RootDir != root {
		t.Fatalf("root dir = %q, want %q", resolved.Paths.RootDir, root)
	}
	if resolved.Paths.ConfigDir != root {
		t.Fatalf("config dir = %q, want %q", resolved.Paths.ConfigDir, root)
	}
	if resolved.Paths.ConfigFile != filepath.Join(root, "config.yaml") {
		t.Fatalf("config file = %q", resolved.Paths.ConfigFile)
	}
	if resolved.Paths.IdentityDir != filepath.Join(root, "identities") {
		t.Fatalf("identity dir = %q", resolved.Paths.IdentityDir)
	}
	if resolved.Paths.DatabaseFile != filepath.Join(root, "data", "awiki-cli.db") {
		t.Fatalf("database file = %q", resolved.Paths.DatabaseFile)
	}
	if resolved.Paths.StateDir != filepath.Join(root, "runtime") {
		t.Fatalf("state dir = %q", resolved.Paths.StateDir)
	}
	if resolved.Paths.CacheDir != filepath.Join(root, "cache") {
		t.Fatalf("cache dir = %q", resolved.Paths.CacheDir)
	}
	if resolved.Paths.LogsDir != filepath.Join(root, "logs") {
		t.Fatalf("logs dir = %q", resolved.Paths.LogsDir)
	}
	if resolved.RuntimeSocketPath != filepath.Join(root, "runtime", "message-daemon.sock") {
		t.Fatalf("runtime socket path = %q", resolved.RuntimeSocketPath)
	}
	if resolved.RuntimeMode != "websocket" {
		t.Fatalf("runtime mode = %q, want websocket", resolved.RuntimeMode)
	}
	if !resolved.RuntimeListenerEnabled {
		t.Fatal("runtime listener enabled = false, want true")
	}
	if !resolved.RuntimeListenerAutoInstall {
		t.Fatal("runtime listener auto_install = false, want true")
	}
	if !resolved.RuntimeListenerAutoStart {
		t.Fatal("runtime listener auto_start = false, want true")
	}
	if resolved.HostNotifySink != "log" {
		t.Fatalf("host notify sink = %q, want log", resolved.HostNotifySink)
	}
	if !resolved.HostNotifyEnabled {
		t.Fatal("host notify enabled = false, want true")
	}
	if resolved.HostNotifyFilePath != "" {
		t.Fatalf("host notify file path = %q, want empty string", resolved.HostNotifyFilePath)
	}
}

func TestResolveIgnoresDeprecatedWorkspaceEnv(t *testing.T) {
	t.Setenv("AWIKI_WORKSPACE_HOME", t.TempDir())

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Paths.WorkspaceHomeDir == "" {
		t.Fatal("workspace home dir should still resolve when deprecated env is present")
	}
}

func TestResolveIgnoresDeprecatedBusinessEnv(t *testing.T) {
	t.Setenv("AWIKI_USER_SERVICE_URL", "https://awiki.test")

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != "https://awiki.ai" {
		t.Fatalf("resolved.ServiceBaseURL = %q, want default value", resolved.ServiceBaseURL)
	}
}

func TestResolveAllowsLegacyConfigJSONForUpgrade(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceHome, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ConfigExists {
		t.Fatal("resolved.ConfigExists = true, want false until upgrade migrates legacy config")
	}
	if resolved.Paths.ConfigFile != filepath.Join(workspaceHome, "config.yaml") {
		t.Fatalf("config file = %q", resolved.Paths.ConfigFile)
	}
}

func TestResolveRejectsDeprecatedServiceURLFieldsInConfigYAML(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  user_service_url: https://awiki.test\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	_, err := Resolve(Overrides{})
	if err == nil {
		t.Fatal("Resolve() error = nil, want deprecated config field error")
	}
	if !strings.Contains(err.Error(), "services.user_service_url") {
		t.Fatalf("Resolve() error = %q, want deprecated config field name", err.Error())
	}
}

func TestResolveDerivesHostNotifyFilePathForFileSink(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("runtime:\n  host_notify:\n    enabled: true\n    sink: file\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !resolved.HostNotifyEnabled {
		t.Fatal("resolved.HostNotifyEnabled = false, want true")
	}
	if resolved.HostNotifySink != "file" {
		t.Fatalf("resolved.HostNotifySink = %q, want file", resolved.HostNotifySink)
	}
	wantPath := filepath.Join(workspaceHome, "runtime", "host-notify.events.jsonl")
	if resolved.HostNotifyFilePath != wantPath {
		t.Fatalf("resolved.HostNotifyFilePath = %q, want %q", resolved.HostNotifyFilePath, wantPath)
	}
	if source := resolved.Sources["host_notify_file_path"]; source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[host_notify_file_path].Source = %q, want derived_default", source.Source)
	}
}

func TestResolveIncludesOpenClawHostNotifyConfig(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("runtime:\n  host_notify:\n    enabled: true\n    sink: openclaw\n    openclaw:\n      hook_url: http://127.0.0.1:18789/hooks/agent\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.HostNotifySink != "openclaw" {
		t.Fatalf("resolved.HostNotifySink = %q, want openclaw", resolved.HostNotifySink)
	}
	if resolved.HostNotifyOpenClawHookURL != "http://127.0.0.1:18789/hooks/agent" {
		t.Fatalf("resolved.HostNotifyOpenClawHookURL = %q", resolved.HostNotifyOpenClawHookURL)
	}
}

func TestResolveIncludesHermesHostNotifyConfig(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("runtime:\n  host_notify:\n    enabled: true\n    sink: hermes\n    hermes:\n      notify_url: http://127.0.0.1:8765/notify/host-event\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.HostNotifySink != "hermes" {
		t.Fatalf("resolved.HostNotifySink = %q, want hermes", resolved.HostNotifySink)
	}
	if resolved.HostNotifyHermesNotifyURL != "http://127.0.0.1:8765/notify/host-event" {
		t.Fatalf("resolved.HostNotifyHermesNotifyURL = %q", resolved.HostNotifyHermesNotifyURL)
	}
	if resolved.HostNotifyHermesDeliver != "feishu" {
		t.Fatalf("resolved.HostNotifyHermesDeliver = %q, want feishu", resolved.HostNotifyHermesDeliver)
	}
}

func TestResolveIncludesHermesDeliverTarget(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("runtime:\n  host_notify:\n    enabled: true\n    sink: hermes\n    hermes:\n      notify_url: http://127.0.0.1:8765/notify/host-event\n      deliver: telegram\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.HostNotifyHermesDeliver != "telegram" {
		t.Fatalf("resolved.HostNotifyHermesDeliver = %q, want telegram", resolved.HostNotifyHermesDeliver)
	}
}

func TestResolveAcceptsLegacyWebhookSinkAlias(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("runtime:\n  host_notify:\n    enabled: true\n    sink: webhook\n    webhook:\n      notify_url: http://127.0.0.1:8765/notify/host-event\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.HostNotifySink != "hermes" {
		t.Fatalf("resolved.HostNotifySink = %q, want hermes", resolved.HostNotifySink)
	}
}

func TestResolveHonorsRuntimeListenerConfigFromFile(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("runtime:\n  listener:\n    enabled: false\n    auto_install: false\n    auto_start: false\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.RuntimeListenerEnabled {
		t.Fatal("resolved.RuntimeListenerEnabled = true, want false")
	}
	if resolved.RuntimeListenerAutoInstall {
		t.Fatal("resolved.RuntimeListenerAutoInstall = true, want false")
	}
	if resolved.RuntimeListenerAutoStart {
		t.Fatal("resolved.RuntimeListenerAutoStart = true, want false")
	}
}

func TestResolveRejectsUnsupportedHostNotifySink(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("runtime:\n  host_notify:\n    enabled: true\n    sink: stdout\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	_, err := Resolve(Overrides{})
	if err == nil {
		t.Fatal("Resolve() error = nil, want unsupported host notify sink error")
	}
	if !strings.Contains(err.Error(), "runtime.host_notify.sink") {
		t.Fatalf("Resolve() error = %q, want runtime.host_notify.sink", err.Error())
	}
}

func TestResolveMailServiceURLFromConfigFile(t *testing.T) {
	workspaceHome := t.TempDir()
	configPath := filepath.Join(workspaceHome, "config.yaml")
	configYAML := []byte(
		"services:\n" +
			"  service_base_url: https://api.awiki.test/\n" +
			"  mail_service_url: https://mail.awiki.test/\n",
	)
	if err := os.WriteFile(configPath, configYAML, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != "https://api.awiki.test" {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, "https://api.awiki.test")
	}
	if resolved.MailServiceURL != "https://mail.awiki.test" {
		t.Fatalf("resolved.MailServiceURL = %q, want %q", resolved.MailServiceURL, "https://mail.awiki.test")
	}
	source := resolved.Sources["mail_service_url"]
	if source.Source != "config_file" {
		t.Fatalf("resolved.Sources[mail_service_url].Source = %q, want %q", source.Source, "config_file")
	}
	if source.Value != "https://mail.awiki.test" {
		t.Fatalf("resolved.Sources[mail_service_url].Value = %q, want %q", source.Value, "https://mail.awiki.test")
	}
}

func TestResolveMailServiceURLDerivedFromServiceBaseURL(t *testing.T) {
	workspaceHome := t.TempDir()
	configPath := filepath.Join(workspaceHome, "config.yaml")
	configYAML := []byte(
		"services:\n" +
			"  service_base_url: https://awiki.test/\n",
	)
	if err := os.WriteFile(configPath, configYAML, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != "https://awiki.test" {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, "https://awiki.test")
	}
	if resolved.MailServiceURL != "https://awiki.test" {
		t.Fatalf("resolved.MailServiceURL = %q, want %q", resolved.MailServiceURL, "https://awiki.test")
	}
	source := resolved.Sources["mail_service_url"]
	if source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[mail_service_url].Source = %q, want %q", source.Source, "derived_default")
	}
	if source.Key != "service_base_url" {
		t.Fatalf("resolved.Sources[mail_service_url].Key = %q, want %q", source.Key, "service_base_url")
	}
	if source.Value != "https://awiki.test" {
		t.Fatalf("resolved.Sources[mail_service_url].Value = %q, want %q", source.Value, "https://awiki.test")
	}
}

func TestNormalizeDomainRejectsURLAndHostPort(t *testing.T) {
	got, err := NormalizeDomain("  A.Example.COM.  ")
	if err != nil {
		t.Fatalf("NormalizeDomain() error = %v", err)
	}
	if got != "a.example.com" {
		t.Fatalf("NormalizeDomain() = %q, want a.example.com", got)
	}
	for _, raw := range []string{"https://a.example.com", "a.example.com:8443", "a.example.com/path", ""} {
		if _, err := NormalizeDomain(raw); err == nil {
			t.Fatalf("NormalizeDomain(%q) error = nil, want error", raw)
		}
	}
}

func TestDefaultServicesForDomainDerivesCanonicalFields(t *testing.T) {
	services, err := DefaultServicesForDomain("B.Example.COM.")
	if err != nil {
		t.Fatalf("DefaultServicesForDomain() error = %v", err)
	}
	if services.DIDDomain != "b.example.com" {
		t.Fatalf("DIDDomain = %q, want b.example.com", services.DIDDomain)
	}
	if services.ServiceBaseURL != "https://b.example.com" {
		t.Fatalf("ServiceBaseURL = %q, want https://b.example.com", services.ServiceBaseURL)
	}
	if services.ANPServiceEndpoint != "https://b.example.com/anp-im/rpc" {
		t.Fatalf("ANPServiceEndpoint = %q", services.ANPServiceEndpoint)
	}
	if services.ANPServiceDID != "did:wba:b.example.com" {
		t.Fatalf("ANPServiceDID = %q", services.ANPServiceDID)
	}
}

func TestValidateServicesReportsAdvancedOverrideWarnings(t *testing.T) {
	diagnostics := ValidateServices(ServicesConfig{
		ServiceBaseURL:     "https://api.a.example.com",
		DIDDomain:          "a.example.com",
		ANPServiceEndpoint: "https://gateway.a.example.com/anp-im/rpc",
		ANPServiceDID:      "did:wba:service.a.example.com",
	})
	if len(diagnostics) != 2 {
		t.Fatalf("len(ValidateServices()) = %d, want 2: %#v", len(diagnostics), diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != ServiceSeverityWarn {
			t.Fatalf("diagnostic severity = %q, want warn: %#v", diagnostic.Severity, diagnostic)
		}
	}
}

func TestValidateServicesReportsBlockingErrors(t *testing.T) {
	diagnostics := ValidateServices(ServicesConfig{
		ServiceBaseURL:     "ftp://api.example.com",
		DIDDomain:          "https://a.example.com",
		ANPServiceEndpoint: "http://127.0.0.1/anp-im/rpc",
		ANPServiceDID:      "did:wba:a.example.com:services:message:e1",
	})
	if len(diagnostics) < 4 {
		t.Fatalf("len(ValidateServices()) = %d, want at least 4: %#v", len(diagnostics), diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "" {
			t.Fatalf("diagnostic missing severity: %#v", diagnostic)
		}
	}
}
