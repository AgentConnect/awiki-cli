package listener

import (
	"fmt"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
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
			AgentID:  runtimeResolved.HostNotify.OpenClaw.AgentID,
			HookName: runtimeResolved.HostNotify.OpenClaw.HookName,
		},
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
	if saved, err := readStatus(statusFile); err == nil {
		status.StartedAt = saved.StartedAt
		status.Sessions = saved.Sessions
		status.PID = saved.PID
		status.HostNotify.LastError = saved.HostNotify.LastError
	}
	if pid, err := readPID(pidFile); err == nil {
		status.PID = pid
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
	status.Warnings = append(status.Warnings, SessionWarnings(status.Sessions)...)
	return status, nil
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
