package cli

import "testing"

func TestRuntimeDryRunPlansCoverStableActions(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantSummary string
		wantAction  string
	}{
		{name: "runtime apply", args: []string{"--dry-run", "runtime", "apply"}, wantSummary: "Dry run: runtime apply planned", wantAction: "runtime_apply"},
		{name: "runtime setup", args: []string{"--dry-run", "runtime", "setup", "--mode", "http"}, wantSummary: "Dry run: runtime setup planned", wantAction: "runtime_setup"},
		{name: "runtime mode set", args: []string{"--dry-run", "runtime", "mode", "set", "websocket"}, wantSummary: "Dry run: runtime mode change planned", wantAction: "runtime_mode_set"},
		{name: "listener start", args: []string{"--dry-run", "runtime", "listener", "start"}, wantSummary: "Dry run: listener start planned", wantAction: "listener_start"},
		{name: "listener stop", args: []string{"--dry-run", "runtime", "listener", "stop"}, wantSummary: "Dry run: listener stop planned", wantAction: "listener_stop"},
		{name: "listener restart", args: []string{"--dry-run", "runtime", "listener", "restart"}, wantSummary: "Dry run: listener restart planned", wantAction: "listener_restart"},
		{name: "listener install", args: []string{"--dry-run", "runtime", "listener", "install"}, wantSummary: "Dry run: listener install planned", wantAction: "listener_install"},
		{name: "listener uninstall", args: []string{"--dry-run", "runtime", "listener", "uninstall"}, wantSummary: "Dry run: listener uninstall planned", wantAction: "listener_uninstall"},
		{name: "listener config set", args: []string{"--dry-run", "runtime", "listener", "config", "set", "--enabled=false"}, wantSummary: "Dry run: listener config change planned", wantAction: "listener_config_set"},
		{name: "listener enable", args: []string{"--dry-run", "runtime", "listener", "enable"}, wantSummary: "Dry run: listener enablement change planned", wantAction: "listener_enable_toggle"},
		{name: "host notify config", args: []string{"--dry-run", "runtime", "host-notify", "config", "set", "--sink", "noop"}, wantSummary: "Dry run: host notify config change planned", wantAction: "host_notify_config_set"},
		{name: "host notify disable", args: []string{"--dry-run", "runtime", "host-notify", "disable"}, wantSummary: "Dry run: host notify enablement change planned", wantAction: "host_notify_enable_toggle"},
		{name: "openclaw set", args: []string{"--dry-run", "runtime", "host-notify", "openclaw", "set", "--hook-url", "http://127.0.0.1:18789/hooks/agent"}, wantSummary: "Dry run: OpenClaw host notify config change planned", wantAction: "host_notify_openclaw_set"},
		{name: "openclaw set token", args: []string{"--dry-run", "runtime", "host-notify", "openclaw", "set-token", "--value", "token"}, wantSummary: "Dry run: OpenClaw token update planned", wantAction: "host_notify_openclaw_set_token"},
		{name: "openclaw clear token", args: []string{"--dry-run", "runtime", "host-notify", "openclaw", "clear-token"}, wantSummary: "Dry run: OpenClaw token clear planned", wantAction: "host_notify_openclaw_clear_token"},
		{name: "openclaw route add", args: []string{"--dry-run", "runtime", "host-notify", "openclaw", "route", "add", "--channel", "feishu", "--to", "chat-1"}, wantSummary: "Dry run: OpenClaw route add planned", wantAction: "host_notify_openclaw_route_add"},
		{name: "openclaw route remove", args: []string{"--dry-run", "runtime", "host-notify", "openclaw", "route", "remove", "--channel", "feishu", "--to", "chat-1"}, wantSummary: "Dry run: OpenClaw route remove planned", wantAction: "host_notify_openclaw_route_remove"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())
			exitCode, stdout, stderr := executeRootCommand(t, tc.args...)
			if exitCode != 0 {
				t.Fatalf("executeRootCommand(%v) exitCode = %d, want 0; stderr = %q", tc.args, exitCode, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			envelope := decodeSuccessEnvelope(t, stdout)
			if envelope.Summary != tc.wantSummary {
				t.Fatalf("summary = %q, want %q", envelope.Summary, tc.wantSummary)
			}
			plan := mustMap(t, envelope.Data["plan"], "data.plan")
			if plan["action"] != tc.wantAction {
				t.Fatalf("plan.action = %#v, want %q", plan["action"], tc.wantAction)
			}
		})
	}
}

func TestRuntimeValidationErrorsUseStableCodes(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "bad setup mode", args: []string{"runtime", "setup", "--mode", "bad"}},
		{name: "missing mode set arg", args: []string{"runtime", "mode", "set"}},
		{name: "bad host notify sink", args: []string{"runtime", "host-notify", "config", "set", "--sink", "bad"}},
		{name: "missing listener config flag", args: []string{"runtime", "listener", "config", "set"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())
			exitCode, stdout, stderr := executeRootCommand(t, tc.args...)
			if exitCode != 2 {
				t.Fatalf("exitCode = %d, want 2; stdout=%q stderr=%q", exitCode, stdout, stderr)
			}
			envelope := decodeErrorEnvelope(t, stderr)
			if envelope.Error.Code != "invalid_argument" {
				t.Fatalf("error.code = %q, want invalid_argument", envelope.Error.Code)
			}
		})
	}
}
