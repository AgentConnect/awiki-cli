package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
	"github.com/agentconnect/awiki-cli/internal/transportcfg"
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
	Hermes   HermesConfig   `json:"hermes,omitempty"`
}

type OpenClawConfig struct {
	HookURL  string `json:"hook_url,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
	HookName string `json:"hook_name,omitempty"`
}

type HermesConfig struct {
	NotifyURL string `json:"notify_url,omitempty"`
	Deliver   string `json:"deliver,omitempty"`
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
	if hostNotify.Sink == "webhook" {
		hostNotify.Sink = "hermes"
	}
	if hostNotify.Sink == "" {
		hostNotify.Sink = "log"
	}
	if hostNotify.Sink == "file" {
		hostNotify.FilePath = strings.TrimSpace(resolved.HostNotifyFilePath)
	}
	if hostNotify.Sink == "openclaw" {
		hostNotify.OpenClaw = OpenClawConfig{
			HookURL:  strings.TrimSpace(resolved.HostNotifyOpenClawHookURL),
			AgentID:  strings.TrimSpace(resolved.HostNotifyOpenClawAgentID),
			HookName: strings.TrimSpace(resolved.HostNotifyOpenClawHookName),
		}
	}
	if hostNotify.Sink == "hermes" {
		hostNotify.Hermes = HermesConfig{
			NotifyURL: strings.TrimSpace(resolved.HostNotifyHermesNotifyURL),
			Deliver:   strings.TrimSpace(resolved.HostNotifyHermesDeliver),
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

type BridgeCallError struct {
	Phase   string
	Message string
	Cause   error
}

func (e *BridgeCallError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Message != "" && e.Cause != nil:
		return fmt.Sprintf("%s: %s: %v", "local websocket bridge request failed", e.Message, e.Cause)
	case e.Message != "":
		return fmt.Sprintf("%s: %s", "local websocket bridge request failed", e.Message)
	case e.Cause != nil:
		return fmt.Sprintf("%s: %v", "local websocket bridge request failed", e.Cause)
	default:
		return "local websocket bridge request failed"
	}
}

func (e *BridgeCallError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func CallLocalBridge(ctx context.Context, request BridgeRequest, resolved *appconfig.Resolved) (map[string]any, error) {
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
	timeoutConfig := transportcfg.Resolve()
	probeDone := traceutil.PhaseContext(ctx, "bridge_health_probe")
	if err := BridgeHealthProbe(bridge.SocketPath, timeoutConfig.BridgeHealthProbeTimeout); err != nil {
		probeDone()
		return nil, &BridgeCallError{
			Phase:   "bridge_health_probe",
			Message: "local websocket bridge unavailable",
			Cause:   err,
		}
	}
	probeDone()

	callDone := traceutil.PhaseContext(ctx, "bridge_call")
	defer callDone()
	conn, err := dialBridge(bridge.SocketPath, timeoutConfig.BridgeDialTimeout)
	if err != nil {
		return nil, &BridgeCallError{
			Phase:   "bridge_dial",
			Message: "local websocket bridge unavailable",
			Cause:   err,
		}
	}
	defer conn.Close()
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(timeoutConfig.BridgeWriteTimeout))
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return nil, &BridgeCallError{
			Phase:   "bridge_write",
			Message: "write websocket bridge request",
			Cause:   err,
		}
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeoutConfig.BridgeReadTimeout))
	decoder := json.NewDecoder(conn)
	var response BridgeResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, &BridgeCallError{
			Phase:   "bridge_read",
			Message: "decode websocket bridge response",
			Cause:   err,
		}
	}
	if !response.OK {
		if response.Error == nil {
			return nil, &BridgeCallError{Phase: "bridge_read", Message: "bridge returned failure without details"}
		}
		return nil, &BridgeCallError{Phase: "bridge_read", Message: response.Error.Message}
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
