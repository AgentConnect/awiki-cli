package hermesbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
	serviceNamePrefix        = "awiki-cli-hermes-bridge"
	serviceDisplayNamePrefix = "awiki-cli Hermes Bridge"
)

type BridgeConfig struct {
	NotifyURL          string     `json:"notify_url"`
	HealthURL          string     `json:"health_url"`
	AdapterHost        string     `json:"adapter_host"`
	AdapterPort        int        `json:"adapter_port"`
	NotifySecret       string     `json:"-"`
	NotifySecretSource string     `json:"notify_secret_source"`
	HermesHome         string     `json:"hermes_home"`
	HermesConfigFile   string     `json:"hermes_config_file"`
	HermesWebhookURL   string     `json:"hermes_webhook_url"`
	RouteName          string     `json:"route_name"`
	RouteSecret        string     `json:"-"`
	RouteState         RouteState `json:"route_state"`
	AdapterScript      string     `json:"adapter_script"`
	PythonExecutable   string     `json:"python_executable"`
}

type Status struct {
	ServiceName     string        `json:"service_name"`
	ServicePlatform string        `json:"service_platform,omitempty"`
	Installed       bool          `json:"installed"`
	Running         bool          `json:"running"`
	BridgeAvailable bool          `json:"bridge_available"`
	HealthURL       string        `json:"health_url,omitempty"`
	Config          *BridgeConfig `json:"config,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
}

type serviceProgram struct {
	resolved *appconfig.Resolved
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan error
	cmd      *exec.Cmd
}

type noopProgram struct{}

var (
	newServiceFunc       = newService
	serviceStatusForFunc = serviceStatusFor
	statusForFunc        = StatusFor
)

func (p *serviceProgram) Start(_ servicepkg.Service) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done != nil {
		return nil
	}
	bridgeConfig, err := ResolveBridgeConfig(p.resolved)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(
		ctx,
		bridgeConfig.PythonExecutable,
		bridgeConfig.AdapterScript,
		"--host", bridgeConfig.AdapterHost,
		"--port", fmt.Sprintf("%d", bridgeConfig.AdapterPort),
		"--notify-secret", bridgeConfig.NotifySecret,
		"--hermes-webhook-url", bridgeConfig.HermesWebhookURL,
		"--hermes-route-secret", bridgeConfig.RouteSecret,
		"--log-level", "INFO",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if bridgeConfig.HermesHome != "" {
		cmd.Env = append(os.Environ(), "HERMES_HOME="+bridgeConfig.HermesHome)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start Hermes notify adapter: %w", err)
	}
	done := make(chan error, 1)
	p.cancel = cancel
	p.done = done
	p.cmd = cmd
	go func() {
		defer close(done)
		done <- cmd.Wait()
	}()
	return nil
}

func (p *serviceProgram) Stop(_ servicepkg.Service) error {
	p.mu.Lock()
	cancel := p.cancel
	done := p.done
	cmd := p.cmd
	p.cancel = nil
	p.done = nil
	p.cmd = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			return fmt.Errorf("Hermes bridge stop timed out")
		}
	}
	return nil
}

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
	bridgeConfig, err := ResolveBridgeConfig(resolved)
	if err != nil {
		return nil, err
	}
	serviceConfig := &servicepkg.Config{
		Name:             serviceNameFor(resolved),
		DisplayName:      serviceDisplayNameFor(resolved),
		Description:      "awiki-cli Hermes notify bridge",
		Arguments:        []string{"runtime", "host-notify", "hermes", "bridge", "service-run"},
		WorkingDirectory: resolved.Paths.WorkspaceHomeDir,
		Option: servicepkg.KeyValue{
			"UserService":            true,
			"KeepAlive":              true,
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "1s",
			"LogOutput":              true,
			"LogDirectory":           resolved.Paths.LogsDir,
		},
		EnvVars: map[string]string{
			"AWIKI_CLI_WORKSPACE_HOME_DIR": resolved.Paths.WorkspaceHomeDir,
			"HERMES_HOME":                  bridgeConfig.HermesHome,
		},
	}
	if runtime.GOOS == "windows" {
		serviceConfig.WorkingDirectory = ""
	}
	return servicepkg.New(program, serviceConfig)
}

func ResolveBridgeConfig(resolved *appconfig.Resolved) (*BridgeConfig, error) {
	if resolved == nil {
		return nil, fmt.Errorf("workspace configuration is required")
	}
	notifyURL := strings.TrimSpace(runtimecfg.Resolve(resolved).HostNotify.Hermes.NotifyURL)
	if notifyURL == "" {
		notifyURL = defaultNotifyURL
	}
	parsedURL, host, port, err := ValidateLocalNotifyURL(notifyURL)
	if err != nil {
		return nil, err
	}
	notifySecret, secretSource, err := resolveNotifySecret(resolved)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(notifySecret) == "" {
		return nil, fmt.Errorf("Hermes host notify secret is not configured in awiki-cli")
	}
	hermesHome, err := ResolveHermesHome()
	if err != nil {
		return nil, err
	}
	routeState, err := InspectRoute(hermesHome, defaultWebhookRouteName)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(routeState.RouteSecret) == "" {
		return nil, fmt.Errorf("Hermes notify route secret is not configured")
	}
	pythonExecutable, err := resolvePythonExecutable()
	if err != nil {
		return nil, err
	}
	scriptPath, err := resolveAdapterScriptPath()
	if err != nil {
		return nil, err
	}
	healthURL := (&url.URL{
		Scheme: parsedURL.Scheme,
		Host:   parsedURL.Host,
		Path:   "/healthz",
	}).String()
	return &BridgeConfig{
		NotifyURL:          notifyURL,
		HealthURL:          healthURL,
		AdapterHost:        normalizeAdapterBindHost(host),
		AdapterPort:        port,
		NotifySecret:       notifySecret,
		NotifySecretSource: secretSource,
		HermesHome:         hermesHome,
		HermesConfigFile:   routeState.ConfigFile,
		HermesWebhookURL:   routeState.NotifyWebhookURL,
		RouteName:          routeState.RouteName,
		RouteSecret:        routeState.RouteSecret,
		RouteState:         routeState,
		AdapterScript:      scriptPath,
		PythonExecutable:   pythonExecutable,
	}, nil
}

func StatusFor(resolved *appconfig.Resolved) (Status, error) {
	status := Status{
		ServiceName: serviceNameFor(resolved),
	}
	bridgeConfig, err := ResolveBridgeConfig(resolved)
	if err != nil {
		status.Warnings = append(status.Warnings, err.Error())
		return status, nil
	}
	status.Config = bridgeConfig
	status.HealthURL = bridgeConfig.HealthURL
	installed, running, platform, serviceName, serviceErr := serviceStatusFor(resolved)
	if serviceErr != nil {
		status.Warnings = append(status.Warnings, fmt.Sprintf("Hermes bridge service status unavailable: %v", serviceErr))
	} else {
		status.Installed = installed
		status.Running = running
		status.ServicePlatform = platform
		status.ServiceName = serviceName
	}
	if status.Running {
		status.BridgeAvailable = bridgeHealthAvailable(bridgeConfig.HealthURL)
		if !status.BridgeAvailable {
			status.Warnings = append(status.Warnings, "Hermes bridge health endpoint is not responding")
		}
	}
	status.Warnings = append(status.Warnings, bridgeConfig.RouteState.Warnings...)
	return status, nil
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

func EnsureInstalled(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newService(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, _, _, _, err := serviceStatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		if err := svc.Install(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "exists") {
			return Status{}, err
		}
	}
	return StatusFor(resolved)
}

func StartService(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newServiceFunc(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, running, _, _, err := serviceStatusForFunc(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		if _, err := EnsureInstalled(resolved); err != nil {
			return Status{}, err
		}
	}
	if running {
		return statusForFunc(resolved)
	}
	if err := svc.Start(); err != nil {
		return Status{}, err
	}
	return waitForStatus(resolved, true, 15*time.Second)
}

func StopService(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newServiceFunc(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, running, _, _, err := serviceStatusForFunc(resolved)
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
	return waitForStatus(resolved, false, 15*time.Second)
}

func RestartService(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newServiceFunc(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, _, _, _, err := serviceStatusForFunc(resolved)
	if err != nil {
		return Status{}, err
	}
	if !installed {
		return Status{}, fmt.Errorf("Hermes bridge service is not installed")
	}
	if err := svc.Restart(); err != nil {
		return Status{}, err
	}
	return waitForStatus(resolved, true, 15*time.Second)
}

func Uninstall(resolved *appconfig.Resolved) (Status, error) {
	svc, err := newServiceFunc(resolved, &noopProgram{})
	if err != nil {
		return Status{}, err
	}
	installed, running, _, _, err := serviceStatusForFunc(resolved)
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
	if err := svc.Uninstall(); err != nil && !errors.Is(err, servicepkg.ErrNotInstalled) {
		return Status{}, err
	}
	return StatusFor(resolved)
}

func Apply(resolved *appconfig.Resolved) (Status, error) {
	status, err := StatusFor(resolved)
	if err != nil {
		return Status{}, err
	}
	if !status.Installed {
		if _, err := EnsureInstalled(resolved); err != nil {
			return Status{}, err
		}
		return StartService(resolved)
	}
	if status.Running {
		return RestartService(resolved)
	}
	return StartService(resolved)
}

func RunService(resolved *appconfig.Resolved) error {
	program := &serviceProgram{resolved: resolved}
	svc, err := newService(resolved, program)
	if err != nil {
		return err
	}
	return svc.Run()
}

func waitForStatus(resolved *appconfig.Resolved, wantRunning bool, timeout time.Duration) (Status, error) {
	deadline := time.Now().Add(timeout)
	lastStatus := Status{}
	for {
		status, err := statusForFunc(resolved)
		if err == nil {
			lastStatus = status
			if wantRunning {
				if status.Running && status.BridgeAvailable {
					return status, nil
				}
			} else if !status.Running {
				return status, nil
			}
		}
		if time.Now().After(deadline) {
			return lastStatus, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func bridgeHealthAvailable(healthURL string) bool {
	if strings.TrimSpace(healthURL) == "" {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func resolveNotifySecret(resolved *appconfig.Resolved) (string, string, error) {
	if resolved == nil {
		return "", "unset", nil
	}
	if strings.TrimSpace(resolved.Paths.ConfigFile) != "" {
		fileConfig, exists, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
		if err != nil {
			return "", "unset", err
		}
		if exists {
			if value := strings.TrimSpace(fileConfig.Runtime.HostNotify.Hermes.Secret); value != "" {
				return value, "config_file", nil
			}
			if value := strings.TrimSpace(fileConfig.Runtime.HostNotify.LegacyWebhook.Secret); value != "" {
				return value, "config_file", nil
			}
		}
	}
	if value := strings.TrimSpace(os.Getenv("AWIKI_HOST_NOTIFY_HERMES_SECRET")); value != "" {
		return value, "environment", nil
	}
	if value := strings.TrimSpace(os.Getenv("AWIKI_HOST_NOTIFY_WEBHOOK_SECRET")); value != "" {
		return value, "environment", nil
	}
	return "", "unset", nil
}

func resolvePythonExecutable() (string, error) {
	candidates := []string{"python3", "python"}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python3 or python was not found in PATH")
}

func resolveAdapterScriptPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve awiki-cli executable path: %w", err)
	}
	candidates := []string{
		filepath.Clean(filepath.Join(filepath.Dir(exePath), "..", "scripts", "hermes_notify_adapter.py")),
		filepath.Clean(filepath.Join(filepath.Dir(exePath), "scripts", "hermes_notify_adapter.py")),
		filepath.Clean(filepath.Join(filepath.Dir(exePath), "..", "..", "scripts", "hermes_notify_adapter.py")),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not locate scripts/hermes_notify_adapter.py next to the awiki-cli installation")
}

func normalizeAdapterBindHost(host string) string {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "", "localhost":
		return "127.0.0.1"
	default:
		return host
	}
}
