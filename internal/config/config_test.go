package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/testenv"
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

func TestResolveDerivesANPServiceDefaultsFromServiceBaseURL(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  service_base_url: "+testenv.SubdomainURL("platform")+"/\n  did_domain: tenant.example\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != testenv.SubdomainURL("platform") {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, testenv.SubdomainURL("platform"))
	}
	if resolved.DIDDomain != "tenant.example" {
		t.Fatalf("resolved.DIDDomain = %q, want %q", resolved.DIDDomain, "tenant.example")
	}
	if resolved.ANPServiceEndpoint != testenv.SubdomainURL("platform")+"/anp-im/rpc" {
		t.Fatalf("resolved.ANPServiceEndpoint = %q, want %q", resolved.ANPServiceEndpoint, testenv.SubdomainURL("platform")+"/anp-im/rpc")
	}
	if resolved.ANPServiceDID != "did:wba:"+testenv.Subdomain("platform") {
		t.Fatalf("resolved.ANPServiceDID = %q, want %q", resolved.ANPServiceDID, "did:wba:"+testenv.Subdomain("platform"))
	}
	if source := resolved.Sources["anp_service_endpoint"]; source.Source != "derived_default" || source.Key != "service_base_url" {
		t.Fatalf("resolved.Sources[anp_service_endpoint] = %#v, want derived from service_base_url", source)
	}
	if source := resolved.Sources["anp_service_did"]; source.Source != "derived_default" || source.Key != "service_base_url" {
		t.Fatalf("resolved.Sources[anp_service_did] = %#v, want derived from service_base_url", source)
	}
}

func TestResolveHonorsServiceBaseURLFromConfigFile(t *testing.T) {
	workspaceHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspaceHome, "config.yaml"),
		[]byte("services:\n  service_base_url: "+testenv.BaseURL()+"/\n"),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != testenv.BaseURL() {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, testenv.BaseURL())
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
	t.Setenv("AWIKI_USER_SERVICE_URL", testenv.BaseURL())

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
		[]byte("services:\n  user_service_url: "+testenv.BaseURL()+"\n"),
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
			"  service_base_url: " + testenv.SubdomainURL("api") + "/\n" +
			"  mail_service_url: " + testenv.SubdomainURL("mail") + "/\n",
	)
	if err := os.WriteFile(configPath, configYAML, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != testenv.SubdomainURL("api") {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, testenv.SubdomainURL("api"))
	}
	if resolved.MailServiceURL != testenv.SubdomainURL("mail") {
		t.Fatalf("resolved.MailServiceURL = %q, want %q", resolved.MailServiceURL, testenv.SubdomainURL("mail"))
	}
	source := resolved.Sources["mail_service_url"]
	if source.Source != "config_file" {
		t.Fatalf("resolved.Sources[mail_service_url].Source = %q, want %q", source.Source, "config_file")
	}
	if source.Value != testenv.SubdomainURL("mail") {
		t.Fatalf("resolved.Sources[mail_service_url].Value = %q, want %q", source.Value, testenv.SubdomainURL("mail"))
	}
}

func TestResolveMailServiceURLDerivedFromServiceBaseURL(t *testing.T) {
	workspaceHome := t.TempDir()
	configPath := filepath.Join(workspaceHome, "config.yaml")
	configYAML := []byte(
		"services:\n" +
			"  service_base_url: " + testenv.BaseURL() + "/\n",
	)
	if err := os.WriteFile(configPath, configYAML, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	resolved, err := Resolve(Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ServiceBaseURL != testenv.BaseURL() {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, testenv.BaseURL())
	}
	if resolved.MailServiceURL != testenv.BaseURL() {
		t.Fatalf("resolved.MailServiceURL = %q, want %q", resolved.MailServiceURL, testenv.BaseURL())
	}
	source := resolved.Sources["mail_service_url"]
	if source.Source != "derived_default" {
		t.Fatalf("resolved.Sources[mail_service_url].Source = %q, want %q", source.Source, "derived_default")
	}
	if source.Key != "service_base_url" {
		t.Fatalf("resolved.Sources[mail_service_url].Key = %q, want %q", source.Key, "service_base_url")
	}
	if source.Value != testenv.BaseURL() {
		t.Fatalf("resolved.Sources[mail_service_url].Value = %q, want %q", source.Value, testenv.BaseURL())
	}
}
