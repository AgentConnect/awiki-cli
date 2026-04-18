package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	listenerrt "github.com/agentconnect/awiki-cli/internal/runtime/listener"
)

func TestHostNotifyConfigViewRedactsOpenClawTokenValue(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`runtime:
  host_notify:
    enabled: true
    sink: openclaw
    openclaw:
      hook_url: http://127.0.0.1:18789/hooks/agent
      token: super-secret-token
`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	resolved := &appconfig.Resolved{
		Paths:                     appconfig.Paths{ConfigFile: configPath},
		HostNotifyEnabled:         true,
		HostNotifySink:            "openclaw",
		HostNotifyOpenClawHookURL: "http://127.0.0.1:18789/hooks/agent",
		Sources: map[string]appconfig.ValueSource{
			"host_notify_openclaw_hook_url": {Source: "config_file", Value: "http://127.0.0.1:18789/hooks/agent"},
		},
	}

	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		t.Fatalf("hostNotifyConfigView() error = %v", err)
	}
	openclawView, ok := view["openclaw"].(map[string]any)
	if !ok {
		t.Fatalf("openclaw view type = %T, want map[string]any", view["openclaw"])
	}
	if openclawView["token_configured"] != true {
		t.Fatalf("token_configured = %#v, want true", openclawView["token_configured"])
	}
	if openclawView["token_source"] != "config_file" {
		t.Fatalf("token_source = %#v, want config_file", openclawView["token_source"])
	}
	if _, exists := openclawView["token"]; exists {
		t.Fatalf("openclaw view unexpectedly exposes token field: %#v", openclawView)
	}
}

func TestHostNotifyConfigViewRedactsHermesSecretValue(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`runtime:
  host_notify:
    enabled: true
    sink: hermes
    hermes:
      notify_url: http://127.0.0.1:8765/notify/host-event
      secret: webhook-secret
`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	resolved := &appconfig.Resolved{
		Paths:                     appconfig.Paths{ConfigFile: configPath},
		HostNotifyEnabled:         true,
		HostNotifySink:            "hermes",
		HostNotifyHermesNotifyURL: "http://127.0.0.1:8765/notify/host-event",
		HostNotifyHermesDeliver:   "telegram",
	}

	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		t.Fatalf("hostNotifyConfigView() error = %v", err)
	}
	hermesView, ok := view["hermes"].(map[string]any)
	if !ok {
		t.Fatalf("hermes view type = %T, want map[string]any", view["hermes"])
	}
	if hermesView["secret_configured"] != true {
		t.Fatalf("secret_configured = %#v, want true", hermesView["secret_configured"])
	}
	if hermesView["secret_source"] != "config_file" {
		t.Fatalf("secret_source = %#v, want config_file", hermesView["secret_source"])
	}
	if hermesView["deliver"] != "telegram" {
		t.Fatalf("deliver = %#v, want telegram", hermesView["deliver"])
	}
	if _, exists := hermesView["secret"]; exists {
		t.Fatalf("hermes view unexpectedly exposes secret field: %#v", hermesView)
	}
}

func TestResolveHermesNotifyURLFallsBackToConfigFileWhenSinkIsNotHermes(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`runtime:
  host_notify:
    sink: log
    hermes:
      notify_url: https://notify.example.com/notify/host-event
`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	resolved := &appconfig.Resolved{
		Paths:          appconfig.Paths{ConfigFile: configPath},
		HostNotifySink: "log",
	}
	got := resolveHermesNotifyURL(resolved)
	if got != "https://notify.example.com/notify/host-event" {
		t.Fatalf("resolveHermesNotifyURL() = %q, want config-file notify URL", got)
	}
}

func TestBuildHermesHostNotifyGuideViewPrefersHomeChannelGuidance(t *testing.T) {
	resolved := &appconfig.Resolved{
		HostNotifyEnabled:         true,
		HostNotifySink:            "hermes",
		HostNotifyHermesNotifyURL: "http://127.0.0.1:8765/notify/host-event",
		HostNotifyHermesDeliver:   "telegram",
	}
	view := buildHermesHostNotifyGuideView(resolved, "http://127.0.0.1:8765/notify/host-event", "telegram", "config_file")
	hermesView, ok := view["hermes"].(map[string]any)
	if !ok {
		t.Fatalf("hermes guide type = %T, want map[string]any", view["hermes"])
	}
	route, ok := hermesView["recommended_route"].(string)
	if !ok {
		t.Fatalf("recommended_route type = %T, want string", hermesView["recommended_route"])
	}
	if !strings.Contains(route, `deliver: "telegram"`) {
		t.Fatalf("recommended_route = %q, want deliver telegram", route)
	}
	if strings.Contains(route, "deliver_extra") {
		t.Fatalf("recommended_route unexpectedly contains deliver_extra: %q", route)
	}
	targeting, ok := hermesView["targeting"].([]string)
	if !ok {
		t.Fatalf("targeting type = %T, want []string", hermesView["targeting"])
	}
	joined := strings.Join(targeting, "\n")
	if !strings.Contains(joined, "TELEGRAM_HOME_CHANNEL") {
		t.Fatalf("targeting = %q, want TELEGRAM_HOME_CHANNEL guidance", joined)
	}
	if !strings.Contains(joined, "/sethome") {
		t.Fatalf("targeting = %q, want /sethome guidance", joined)
	}
}

func TestRefreshListenerForHostNotifyChangeRestartsRunningListener(t *testing.T) {
	originalStatusFor := listenerStatusForFunc
	originalStop := listenerStopFunc
	originalBootstrap := runtimeBootstrapFunc
	t.Cleanup(func() {
		listenerStatusForFunc = originalStatusFor
		listenerStopFunc = originalStop
		runtimeBootstrapFunc = originalBootstrap
	})

	stopCalled := false
	bootstrapCalled := false
	listenerStatusForFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		return listenerrt.Status{Running: true}, nil
	}
	listenerStopFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		stopCalled = true
		return listenerrt.Status{}, nil
	}
	runtimeBootstrapFunc = func(_ *App, _ *appconfig.Resolved) (listenerrt.Status, error) {
		bootstrapCalled = true
		return listenerrt.Status{Running: true, HostNotify: listenerrt.HostNotifyStatus{Sink: "openclaw"}}, nil
	}

	app := &App{}
	resolved := &appconfig.Resolved{
		RuntimeMode:            "websocket",
		RuntimeListenerEnabled: true,
		HostNotifyEnabled:      true,
		HostNotifySink:         "openclaw",
	}

	status, warnings, err := app.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		t.Fatalf("refreshListenerForHostNotifyChange() error = %v", err)
	}
	if !stopCalled {
		t.Fatal("listenerStopFunc was not called")
	}
	if !bootstrapCalled {
		t.Fatal("runtimeBootstrapFunc was not called")
	}
	if status == nil || !status.Running {
		t.Fatalf("status = %#v, want running listener status", status)
	}
	if len(warnings) == 0 || warnings[0] != "Listener restarted to apply host notify configuration." {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestRefreshListenerForHostNotifyChangeWarnsWhenListenerIsStopped(t *testing.T) {
	originalStatusFor := listenerStatusForFunc
	originalStop := listenerStopFunc
	originalBootstrap := runtimeBootstrapFunc
	t.Cleanup(func() {
		listenerStatusForFunc = originalStatusFor
		listenerStopFunc = originalStop
		runtimeBootstrapFunc = originalBootstrap
	})

	listenerStatusForFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		return listenerrt.Status{Running: false}, nil
	}
	listenerStopFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		t.Fatal("listenerStopFunc should not be called when listener is stopped")
		return listenerrt.Status{}, nil
	}
	runtimeBootstrapFunc = func(_ *App, _ *appconfig.Resolved) (listenerrt.Status, error) {
		t.Fatal("runtimeBootstrapFunc should not be called when listener is stopped")
		return listenerrt.Status{}, nil
	}

	app := &App{}
	resolved := &appconfig.Resolved{
		RuntimeMode:            "websocket",
		RuntimeListenerEnabled: true,
		HostNotifyEnabled:      true,
		HostNotifySink:         "openclaw",
	}

	status, warnings, err := app.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		t.Fatalf("refreshListenerForHostNotifyChange() error = %v", err)
	}
	if status == nil {
		t.Fatal("status = nil, want current listener status")
	}
	if len(warnings) != 1 || warnings[0] != "Host notify changes will apply the next time the listener starts." {
		t.Fatalf("warnings = %#v", warnings)
	}
}
