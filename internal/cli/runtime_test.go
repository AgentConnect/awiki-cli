package cli

import (
	"os"
	"path/filepath"
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

func TestRefreshListenerForHostNotifyChangeRestartsRunningListener(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
