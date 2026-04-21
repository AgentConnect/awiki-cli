//go:build windows

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	winio "github.com/Microsoft/go-winio"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func defaultBridgeEndpoint(paths appconfig.Paths) string {
	workspace := paths.WorkspaceHomeDir
	if strings.TrimSpace(workspace) == "" {
		workspace = filepath.Join(tempDir(), "awiki-cli")
	}
	sum := sha256.Sum256([]byte(workspace))
	return `\\.\pipe\awiki-cli-` + hex.EncodeToString(sum[:8])
}

func normalizeBridgeEndpoint(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return defaultBridgeEndpoint(appconfig.Paths{})
	}
	return trimmed
}

func prepareBridgeEndpoint(path string) error {
	if !strings.HasPrefix(strings.ToLower(path), `\\.\pipe\`) {
		return fmt.Errorf("windows websocket bridge socket must use a named pipe path")
	}
	return nil
}

func dialBridge(path string, timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(path, &timeout)
}

func ListenBridge(path string) (net.Listener, error) {
	return winio.ListenPipe(path, nil)
}

func BridgeEndpointAvailable(path string) bool {
	timeout := time.Duration(0)
	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func tempDir() string {
	return os.TempDir()
}

func BridgeHealthProbe(path string, timeout time.Duration) error {
	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
