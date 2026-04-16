package cli

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
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
