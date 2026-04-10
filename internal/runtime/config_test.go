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
