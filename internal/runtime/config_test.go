package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestResolveShortensLongSocketPath(t *testing.T) {
	t.Parallel()

	longWorkspaceHome := filepath.Join("/tmp", strings.Repeat("very-long-runtime-dir-", 10))
	resolved := Resolve(&appconfig.Resolved{
		RuntimeMode: "websocket",
		Paths: appconfig.Paths{
			StateDir: longWorkspaceHome,
		},
	})
	if resolved.Mode != ModeWebSocket {
		t.Fatalf("resolved.Mode = %q, want %q", resolved.Mode, ModeWebSocket)
	}
	if len(resolved.SocketPath) > maxUnixSocketPathBytes {
		t.Fatalf("len(resolved.SocketPath) = %d, want <= %d", len(resolved.SocketPath), maxUnixSocketPathBytes)
	}
	if !strings.HasPrefix(resolved.SocketPath, filepath.Join(os.TempDir(), "awiki-cli-")) {
		t.Fatalf("resolved.SocketPath = %q, want shortened temp-dir path", resolved.SocketPath)
	}
}

func TestResolveKeepsShortSocketPath(t *testing.T) {
	t.Parallel()

	resolved := Resolve(&appconfig.Resolved{
		RuntimeMode:       "websocket",
		RuntimeSocketPath: "/tmp/custom-awiki.sock",
		Paths: appconfig.Paths{
			StateDir: "/tmp/.awiki-cli/runtime",
		},
	})
	if resolved.SocketPath != "/tmp/custom-awiki.sock" {
		t.Fatalf("resolved.SocketPath = %q, want /tmp/custom-awiki.sock", resolved.SocketPath)
	}
}

func TestResolveDefaultsToWebSocketMode(t *testing.T) {
	t.Parallel()

	resolved := Resolve(nil)
	if resolved.Mode != ModeWebSocket {
		t.Fatalf("resolved.Mode = %q, want %q", resolved.Mode, ModeWebSocket)
	}
	if resolved.HostNotify.Sink != "log" {
		t.Fatalf("resolved.HostNotify.Sink = %q, want log", resolved.HostNotify.Sink)
	}
	if resolved.HostNotify.Enabled {
		t.Fatal("resolved.HostNotify.Enabled = true, want false")
	}
	if !resolved.Listener.Enabled {
		t.Fatal("resolved.Listener.Enabled = false, want true")
	}
	if !resolved.Listener.AutoInstall {
		t.Fatal("resolved.Listener.AutoInstall = false, want true")
	}
	if !resolved.Listener.AutoStart {
		t.Fatal("resolved.Listener.AutoStart = false, want true")
	}
}

func TestResolveIncludesHostNotifyConfig(t *testing.T) {
	t.Parallel()

	resolved := Resolve(&appconfig.Resolved{
		RuntimeMode:        "websocket",
		HostNotifyEnabled:  true,
		HostNotifySink:     "file",
		HostNotifyFilePath: "/tmp/host-notify.events.jsonl",
	})
	if !resolved.HostNotify.Enabled {
		t.Fatal("resolved.HostNotify.Enabled = false, want true")
	}
	if resolved.HostNotify.Sink != "file" {
		t.Fatalf("resolved.HostNotify.Sink = %q, want file", resolved.HostNotify.Sink)
	}
	if resolved.HostNotify.FilePath != "/tmp/host-notify.events.jsonl" {
		t.Fatalf("resolved.HostNotify.FilePath = %q, want /tmp/host-notify.events.jsonl", resolved.HostNotify.FilePath)
	}
}

func TestResolveIncludesOpenClawHostNotifyConfig(t *testing.T) {
	t.Parallel()

	resolved := Resolve(&appconfig.Resolved{
		RuntimeMode:                "websocket",
		HostNotifyEnabled:          true,
		HostNotifySink:             "openclaw",
		HostNotifyOpenClawHookURL:  "http://127.0.0.1:18789/hooks/agent",
		HostNotifyOpenClawAgentID:  "notify",
		HostNotifyOpenClawHookName: "AWiki",
	})
	if !resolved.HostNotify.Enabled {
		t.Fatal("resolved.HostNotify.Enabled = false, want true")
	}
	if resolved.HostNotify.Sink != "openclaw" {
		t.Fatalf("resolved.HostNotify.Sink = %q, want openclaw", resolved.HostNotify.Sink)
	}
	if resolved.HostNotify.OpenClaw.HookURL != "http://127.0.0.1:18789/hooks/agent" {
		t.Fatalf("resolved.HostNotify.OpenClaw.HookURL = %q", resolved.HostNotify.OpenClaw.HookURL)
	}
	if resolved.HostNotify.OpenClaw.AgentID != "notify" {
		t.Fatalf("resolved.HostNotify.OpenClaw.AgentID = %q", resolved.HostNotify.OpenClaw.AgentID)
	}
	if resolved.HostNotify.OpenClaw.HookName != "AWiki" {
		t.Fatalf("resolved.HostNotify.OpenClaw.HookName = %q", resolved.HostNotify.OpenClaw.HookName)
	}
}

func TestResolveIncludesListenerConfig(t *testing.T) {
	t.Parallel()

	resolved := Resolve(&appconfig.Resolved{
		RuntimeMode:                "websocket",
		RuntimeListenerEnabled:     false,
		RuntimeListenerAutoInstall: false,
		RuntimeListenerAutoStart:   false,
	})
	if resolved.Listener.Enabled {
		t.Fatal("resolved.Listener.Enabled = true, want false")
	}
	if resolved.Listener.AutoInstall {
		t.Fatal("resolved.Listener.AutoInstall = true, want false")
	}
	if resolved.Listener.AutoStart {
		t.Fatal("resolved.Listener.AutoStart = true, want false")
	}
}
