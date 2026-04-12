//go:build !windows

package runtime

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func defaultBridgeEndpoint(paths appconfig.Paths) string {
	stateDir := strings.TrimSpace(paths.StateDir)
	if stateDir == "" {
		stateDir = filepath.Join(paths.WorkspaceHomeDir, "runtime")
	}
	return filepath.Join(stateDir, "message-daemon.sock")
}

func normalizeBridgeEndpoint(path string) string {
	return normalizeSocketPath(path)
}

func prepareBridgeEndpoint(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("prepare websocket bridge socket dir: %w", err)
	}
	return nil
}

func dialBridge(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}

func ListenBridge(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	return net.Listen("unix", path)
}

func BridgeEndpointAvailable(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func tempDir() string {
	return os.TempDir()
}

func serviceStopSignal() os.Signal {
	return os.Interrupt
}

func BridgeHealthProbe(path string) error {
	conn, err := net.DialTimeout("unix", path, 300*time.Millisecond)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
