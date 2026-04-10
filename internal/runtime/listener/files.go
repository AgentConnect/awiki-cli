package listener

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/runtime"
)

func paths(resolved *appconfig.Resolved) (pidFile string, logFile string, statusFile string, socketPath string, err error) {
	bridge := runtime.Resolve(resolved)
	root := resolved.Paths.StateDir
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(resolved.Paths.WorkspaceHomeDir, "runtime")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", "", "", "", fmt.Errorf("create runtime state dir: %w", err)
	}
	return filepath.Join(root, "listener.pid"),
		filepath.Join(root, "listener.log"),
		filepath.Join(root, "listener.status.json"),
		bridge.SocketPath,
		nil
}

func writePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

func readPID(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(raw)))
}

func writeStatus(path string, status Status) error {
	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func readStatus(path string) (Status, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(raw, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}
