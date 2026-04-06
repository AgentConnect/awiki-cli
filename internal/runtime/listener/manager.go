package listener

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
)

func StatusFor(resolved *appconfig.Resolved) (Status, error) {
	pidFile, logFile, statusFile, socketPath, err := paths(resolved)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Mode:       runtimecfg.Resolve(resolved).Mode,
		PIDFile:    pidFile,
		LogFile:    logFile,
		StatusFile: statusFile,
		SocketPath: socketPath,
	}
	if saved, err := readStatus(statusFile); err == nil {
		status = saved
		status.PIDFile = pidFile
		status.LogFile = logFile
		status.StatusFile = statusFile
		status.SocketPath = socketPath
	}
	pid, err := readPID(pidFile)
	if err == nil {
		status.PID = pid
		status.Running = processExists(pid)
	}
	if !status.Running {
		status.Warnings = append(status.Warnings, "listener process is not running")
	}
	if _, err := os.Stat(socketPath); err != nil {
		status.Warnings = append(status.Warnings, "listener socket is not available")
	}
	return status, nil
}

func Start(resolved *appconfig.Resolved) (Status, error) {
	if runtimecfg.Resolve(resolved).Mode != runtimecfg.ModeWebSocket {
		return Status{}, fmt.Errorf("runtime mode must be websocket before starting the listener")
	}
	status, err := StatusFor(resolved)
	if err == nil && status.Running {
		return status, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Dir(status.LogFile), 0o700); err != nil {
		return Status{}, err
	}
	logFile, err := os.OpenFile(status.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return Status{}, err
	}
	defer logFile.Close()
	command := exec.Command(executable, "runtime", "listener", "run")
	command.Stdout = logFile
	command.Stderr = logFile
	command.Env = os.Environ()
	if runtime.GOOS != "windows" {
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := command.Start(); err != nil {
		return Status{}, err
	}
	pid := command.Process.Pid
	if err := writePID(status.PIDFile, pid); err != nil {
		return Status{}, err
	}
	for index := 0; index < 20; index++ {
		time.Sleep(250 * time.Millisecond)
		status, _ = StatusFor(resolved)
		if status.Running {
			return status, nil
		}
	}
	return StatusFor(resolved)
}

func Stop(resolved *appconfig.Resolved) (Status, error) {
	status, err := StatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if status.PID != 0 && processExists(status.PID) {
		process, err := os.FindProcess(status.PID)
		if err == nil {
			_ = process.Signal(syscall.SIGTERM)
		}
		for index := 0; index < 20; index++ {
			time.Sleep(250 * time.Millisecond)
			if !processExists(status.PID) {
				break
			}
		}
	}
	_ = os.Remove(status.PIDFile)
	_ = os.Remove(status.SocketPath)
	_ = os.Remove(status.StatusFile)
	return StatusFor(resolved)
}

func Restart(resolved *appconfig.Resolved) (Status, error) {
	if _, err := Stop(resolved); err != nil {
		return Status{}, err
	}
	return Start(resolved)
}

func RunForeground(resolved *appconfig.Resolved) error {
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		return err
	}
	defer supervisor.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return supervisor.Run(ctx)
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
