package listener

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	servicepkg "github.com/kardianos/service"
)

const (
	serviceNamePrefix        = "awiki-cli-listener"
	serviceDisplayNamePrefix = "awiki-cli Listener"
)

type serviceProgram struct {
	resolved   *appconfig.Resolved
	mu         sync.Mutex
	supervisor *Supervisor
	cancel     context.CancelFunc
	done       chan error
}

func (p *serviceProgram) Start(_ servicepkg.Service) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done != nil {
		return nil
	}
	supervisor, err := NewSupervisor(p.resolved)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	p.supervisor = supervisor
	p.cancel = cancel
	p.done = done
	go func() {
		defer close(done)
		defer cleanupRuntimeArtifacts(p.resolved)
		done <- supervisor.Run(ctx)
	}()
	return nil
}

func (p *serviceProgram) Stop(_ servicepkg.Service) error {
	p.mu.Lock()
	cancel := p.cancel
	done := p.done
	supervisor := p.supervisor
	p.cancel = nil
	p.done = nil
	p.supervisor = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if supervisor != nil {
		_ = supervisor.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			return fmt.Errorf("listener service stop timed out")
		}
	}
	return nil
}

type noopProgram struct{}

func (p *noopProgram) Start(servicepkg.Service) error { return nil }
func (p *noopProgram) Stop(servicepkg.Service) error  { return nil }

func serviceNameFor(resolved *appconfig.Resolved) string {
	workspace := "default"
	if resolved != nil {
		workspace = strings.TrimSpace(resolved.Paths.WorkspaceHomeDir)
	}
	if workspace == "" {
		return serviceNamePrefix
	}
	sum := sha256.Sum256([]byte(workspace))
	return serviceNamePrefix + "-" + hex.EncodeToString(sum[:6])
}

func serviceDisplayNameFor(resolved *appconfig.Resolved) string {
	if resolved == nil {
		return serviceDisplayNamePrefix
	}
	base := filepath.Base(strings.TrimSpace(resolved.Paths.WorkspaceHomeDir))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return serviceDisplayNamePrefix
	}
	return fmt.Sprintf("%s (%s)", serviceDisplayNamePrefix, base)
}

func newService(resolved *appconfig.Resolved, program servicepkg.Interface) (servicepkg.Service, error) {
	executableConfig := &servicepkg.Config{
		Name:             serviceNameFor(resolved),
		DisplayName:      serviceDisplayNameFor(resolved),
		Description:      "awiki-cli realtime websocket listener",
		Arguments:        []string{"runtime", "listener", "service-run"},
		WorkingDirectory: resolved.Paths.WorkspaceHomeDir,
		Option: servicepkg.KeyValue{
			"RunAtLoad":              true,
			"KeepAlive":              true,
			"UserService":            true,
			"DelayedAutoStart":       true,
			"StartType":              "automatic",
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "1s",
			"LogOutput":              true,
			"LogDirectory":           resolved.Paths.LogsDir,
			"PIDFile":                filepath.Join(resolved.Paths.StateDir, "listener.service.pid"),
		},
		EnvVars: map[string]string{
			"AWIKI_CLI_WORKSPACE_HOME_DIR": resolved.Paths.WorkspaceHomeDir,
		},
	}
	if runtime.GOOS == "windows" {
		executableConfig.WorkingDirectory = ""
	}
	return servicepkg.New(program, executableConfig)
}

func serviceStatusFor(resolved *appconfig.Resolved) (installed bool, running bool, platform string, serviceName string, err error) {
	svc, err := newService(resolved, &noopProgram{})
	if err != nil {
		return false, false, "", "", err
	}
	status, err := svc.Status()
	if err != nil {
		if errors.Is(err, servicepkg.ErrNotInstalled) {
			return false, false, svc.Platform(), serviceNameFor(resolved), nil
		}
		return false, false, svc.Platform(), serviceNameFor(resolved), err
	}
	return true, status == servicepkg.StatusRunning, svc.Platform(), serviceNameFor(resolved), nil
}

func Install(resolved *appconfig.Resolved) (Status, error) {
	return EnsureInstalled(resolved)
}

func EnsureInstalled(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newService(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, running, _, _, err := serviceStatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		if err := svc.Install(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "exists") {
			return Status{}, err
		}
	}
	if installed && running {
		return StatusFor(resolved)
	}
	return StatusFor(resolved)
}

func StartService(resolved *appconfig.Resolved) (Status, error) {
	if runtimeMode := strings.ToLower(strings.TrimSpace(resolved.RuntimeMode)); runtimeMode != "websocket" {
		return Status{}, fmt.Errorf("runtime mode must be websocket before starting the listener")
	}
	svc, err := newService(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, running, _, _, err := serviceStatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		return Status{}, fmt.Errorf("listener service is not installed")
	}
	if running {
		return StatusFor(resolved)
	}
	if err := svc.Start(); err != nil {
		return Status{}, err
	}
	return waitForServiceStatus(resolved, true)
}

func StopService(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newService(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, running, _, _, err := serviceStatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		return StatusFor(resolved)
	}
	if running {
		if err := svc.Stop(); err != nil {
			return Status{}, err
		}
	}
	cleanupRuntimeArtifacts(resolved)
	return waitForServiceStatus(resolved, false)
}

func RestartService(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newService(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, _, _, _, err := serviceStatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		return Status{}, fmt.Errorf("listener service is not installed")
	}
	if err := svc.Restart(); err != nil {
		return Status{}, err
	}
	return waitForServiceStatus(resolved, true)
}

func Uninstall(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newService(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, running, _, _, err := serviceStatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		cleanupRuntimeArtifacts(resolved)
		return StatusFor(resolved)
	}
	if running {
		if err := svc.Stop(); err != nil {
			return Status{}, err
		}
	}
	if err := svc.Uninstall(); err != nil && !errors.Is(err, servicepkg.ErrNotInstalled) {
		return Status{}, err
	}
	cleanupRuntimeArtifacts(resolved)
	return StatusFor(resolved)
}

func RunService(resolved *appconfig.Resolved) error {
	program := &serviceProgram{resolved: resolved}
	svc, err := newService(resolved, program)
	if err != nil {
		return err
	}
	return svc.Run()
}

func ApplyRuntimePolicy(resolved *appconfig.Resolved) (Status, error) {
	runtimeResolved := runtimecfg.Resolve(resolved)
	if runtimeResolved.Mode != runtimecfg.ModeWebSocket || !runtimeResolved.Listener.Enabled {
		return StopService(resolved)
	}
	if runtimeResolved.Listener.AutoInstall {
		status, err := EnsureInstalled(resolved)
		if err != nil {
			return Status{}, err
		}
		if runtimeResolved.Listener.AutoStart {
			return StartService(resolved)
		}
		return status, nil
	}
	if runtimeResolved.Listener.AutoStart {
		installed, _, _, _, err := serviceStatusFor(resolved)
		if err != nil {
			return Status{}, err
		}
		if !installed {
			return StatusFor(resolved)
		}
		return StartService(resolved)
	}
	return StatusFor(resolved)
}

func waitForServiceStatus(resolved *appconfig.Resolved, wantRunning bool) (Status, error) {
	deadline := time.Now().Add(15 * time.Second)
	for {
		status, err := StatusFor(resolved)
		if err == nil {
			if wantRunning {
				if status.Installed && status.Running {
					return status, nil
				}
			} else if !status.Running {
				return status, nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return Status{}, err
			}
			return status, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func cleanupRuntimeArtifacts(resolved *appconfig.Resolved) {
	if resolved == nil {
		return
	}
	pidFile, _, statusFile, socketPath, err := paths(resolved)
	if err != nil {
		return
	}
	_ = os.Remove(pidFile)
	_ = os.Remove(statusFile)
	_ = os.Remove(socketPath)
}
