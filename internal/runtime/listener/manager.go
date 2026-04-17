package listener

import (
	"fmt"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/runtime/openclawnotify"
)

func StatusFor(resolved *appconfig.Resolved) (Status, error) {
	pidFile, logFile, statusFile, socketPath, err := paths(resolved)
	if err != nil {
		return Status{}, err
	}
	runtimeResolved := runtimecfg.Resolve(resolved)
	status := Status{
		Mode:        runtimeResolved.Mode,
		PIDFile:     pidFile,
		LogFile:     logFile,
		StatusFile:  statusFile,
		SocketPath:  socketPath,
		ServiceName: serviceNameFor(resolved),
		HostNotify: HostNotifyStatus{
			Enabled:  runtimeResolved.HostNotify.Enabled,
			Sink:     runtimeResolved.HostNotify.Sink,
			FilePath: runtimeResolved.HostNotify.FilePath,
			HookURL:  runtimeResolved.HostNotify.OpenClaw.HookURL,
		},
	}
	if runtimeResolved.HostNotify.Sink == "openclaw" {
		settings, err := openclawnotify.ResolveSettings(resolved)
		if err != nil {
			status.Warnings = append(status.Warnings, fmt.Sprintf("openclaw host notify config unavailable: %v", err))
		} else {
			status.HostNotify.HookURL = settings.HookURL
		}
	}
	installed, running, platform, serviceName, serviceErr := serviceStatusFor(resolved)
	if serviceErr != nil {
		status.Warnings = append(status.Warnings, fmt.Sprintf("listener service status unavailable: %v", serviceErr))
	} else {
		status.Installed = installed
		status.Running = running
		status.ServicePlatform = platform
		status.ServiceName = serviceName
	}
	if pid, err := readPID(pidFile); err == nil {
		status.PID = pid
	}
	if saved, err := readStatus(statusFile); err == nil {
		mergeSavedRuntimeStatus(&status, saved)
	}
	if runtimeResolved.Listener.Enabled && !status.Installed {
		status.Warnings = append(status.Warnings, "listener service is not installed")
	}
	if runtimeResolved.Listener.Enabled && !status.Running {
		status.Warnings = append(status.Warnings, "listener service is not running")
	}
	if runtimeResolved.Listener.Enabled {
		status.BridgeAvailable = runtimecfg.BridgeEndpointAvailable(socketPath)
		if !status.BridgeAvailable {
			status.Warnings = append(status.Warnings, "listener socket is not available")
		}
	} else {
		status.BridgeAvailable = false
		status.Warnings = append(status.Warnings, "listener is disabled by configuration")
	}
	return status, nil
}

func mergeSavedRuntimeStatus(status *Status, saved Status) {
	if status == nil {
		return
	}
	if status.PID != 0 && saved.PID != 0 && saved.PID != status.PID {
		return
	}
	status.StartedAt = saved.StartedAt
	status.Sessions = saved.Sessions
	if status.PID == 0 {
		status.PID = saved.PID
	}
	status.BootID = saved.BootID
	status.HostNotify.LastError = saved.HostNotify.LastError
	if !status.Running {
		return
	}
	status.HostNotify.Enabled = saved.HostNotify.Enabled
	if sink := strings.TrimSpace(saved.HostNotify.Sink); sink != "" {
		status.HostNotify.Sink = sink
	}
	status.HostNotify.FilePath = saved.HostNotify.FilePath
	if hookURL := strings.TrimSpace(saved.HostNotify.HookURL); hookURL != "" {
		status.HostNotify.HookURL = hookURL
	}
}

func Start(resolved *appconfig.Resolved) (Status, error) {
	if runtimecfg.Resolve(resolved).Mode != runtimecfg.ModeWebSocket {
		return Status{}, fmt.Errorf("runtime mode must be websocket before starting the listener")
	}
	return StartService(resolved)
}

func Stop(resolved *appconfig.Resolved) (Status, error) {
	return StopService(resolved)
}

func Restart(resolved *appconfig.Resolved) (Status, error) {
	return RestartService(resolved)
}

func RunForeground(resolved *appconfig.Resolved) error {
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		return err
	}
	defer supervisor.Close()
	defer cleanupRuntimeArtifacts(resolved)
	return runForegroundSignals(resolved, supervisor)
}
