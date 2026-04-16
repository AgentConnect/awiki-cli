package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const (
	ModeHTTP               = "http"
	ModeWebSocket          = "websocket"
	maxUnixSocketPathBytes = 100
)

type Resolved struct {
	Mode       string           `json:"mode"`
	SocketPath string           `json:"socket_path,omitempty"`
	Listener   ListenerConfig   `json:"listener"`
	HostNotify HostNotifyConfig `json:"host_notify"`
}

type ListenerConfig struct {
	Enabled     bool `json:"enabled"`
	AutoInstall bool `json:"auto_install"`
	AutoStart   bool `json:"auto_start"`
}

type HostNotifyConfig struct {
	Enabled  bool           `json:"enabled"`
	Sink     string         `json:"sink"`
	FilePath string         `json:"file_path,omitempty"`
	OpenClaw OpenClawConfig `json:"openclaw,omitempty"`
}

type OpenClawConfig struct {
	HookURL string `json:"hook_url,omitempty"`
}

func Resolve(resolved *appconfig.Resolved) Resolved {
	if resolved == nil {
		return Resolved{
			Mode: ModeWebSocket,
			Listener: ListenerConfig{
				Enabled:     true,
				AutoInstall: true,
				AutoStart:   true,
			},
			HostNotify: HostNotifyConfig{
				Enabled: true,
				Sink:    "log",
			},
		}
	}
	mode := strings.ToLower(strings.TrimSpace(resolved.RuntimeMode))
	if mode != ModeHTTP {
		mode = ModeWebSocket
	}
	socketPath := strings.TrimSpace(resolved.RuntimeSocketPath)
	if socketPath == "" {
		socketPath = defaultBridgeEndpoint(resolved.Paths)
	}
	socketPath = normalizeBridgeEndpoint(socketPath)
	listener := ListenerConfig{
		Enabled:     resolved.RuntimeListenerEnabled,
		AutoInstall: resolved.RuntimeListenerAutoInstall,
		AutoStart:   resolved.RuntimeListenerAutoStart,
	}
	hostNotify := HostNotifyConfig{
		Enabled: resolved.HostNotifyEnabled,
		Sink:    strings.ToLower(strings.TrimSpace(resolved.HostNotifySink)),
	}
	if hostNotify.Sink == "" {
		hostNotify.Sink = "log"
	}
	if hostNotify.Sink == "file" {
		hostNotify.FilePath = strings.TrimSpace(resolved.HostNotifyFilePath)
	}
	if hostNotify.Sink == "openclaw" {
		hostNotify.OpenClaw = OpenClawConfig{
			HookURL: strings.TrimSpace(resolved.HostNotifyOpenClawHookURL),
		}
	}
	return Resolved{
		Mode:       mode,
		SocketPath: socketPath,
		Listener:   listener,
		HostNotify: hostNotify,
	}
}

func IsWebSocketMode(resolved *appconfig.Resolved) bool {
	return Resolve(resolved).Mode == ModeWebSocket
}

type BridgeRequest struct {
	Method       string         `json:"method"`
	Params       map[string]any `json:"params"`
	IdentityName string         `json:"identity_name"`
}

type BridgeResponse struct {
	OK     bool           `json:"ok"`
	Result map[string]any `json:"result,omitempty"`
	Error  *BridgeError   `json:"error,omitempty"`
}

type BridgeError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

func CallLocalBridge(request BridgeRequest, resolved *appconfig.Resolved) (map[string]any, error) {
	bridge := Resolve(resolved)
	if bridge.Mode != ModeWebSocket {
		return nil, fmt.Errorf("runtime mode %s does not use the local websocket bridge", bridge.Mode)
	}
	if strings.TrimSpace(bridge.SocketPath) == "" {
		return nil, fmt.Errorf("runtime websocket bridge socket is not configured")
	}
	if err := prepareBridgeEndpoint(bridge.SocketPath); err != nil {
		return nil, err
	}
	conn, err := dialBridge(bridge.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("local websocket bridge unavailable: %w", err)
	}
	defer conn.Close()
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return nil, fmt.Errorf("write websocket bridge request: %w", err)
	}
	decoder := json.NewDecoder(conn)
	var response BridgeResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode websocket bridge response: %w", err)
	}
	if !response.OK {
		if response.Error == nil {
			return nil, fmt.Errorf("local websocket bridge request failed")
		}
		return nil, fmt.Errorf("local websocket bridge request failed: %s", response.Error.Message)
	}
	if response.Result == nil {
		return map[string]any{}, nil
	}
	return response.Result, nil
}

func normalizeSocketPath(path string) string {
	if len(path) <= maxUnixSocketPathBytes {
		return path
	}
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(tempDir(), "awiki-cli-"+hex.EncodeToString(sum[:8])+".sock")
}
