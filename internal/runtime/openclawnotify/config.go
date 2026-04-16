package openclawnotify

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const (
	HookTokenEnv   = "OPENCLAW_HOOK_TOKEN"
	GatewayPortEnv = "OPENCLAW_GATEWAY_PORT"
	ConfigPathEnv  = "OPENCLAW_CONFIG_PATH"

	DefaultGatewayPort = 18789
	DefaultHookPath    = "/hooks/agent"
	FixedHookName      = "AWiki"
)

type GatewayProbe struct {
	Port       int    `json:"port"`
	Source     string `json:"source"`
	ConfigPath string `json:"config_path,omitempty"`
}

type EffectiveSettings struct {
	HookURL                 string       `json:"hook_url"`
	HookURLSource           string       `json:"hook_url_source"`
	DetectedWebhookPort     int          `json:"detected_webhook_port"`
	DetectedWebhookPortInfo GatewayProbe `json:"-"`
	Token                   string       `json:"-"`
	TokenConfigured         bool         `json:"token_configured"`
	TokenSource             string       `json:"token_source"`
}

func ResolveSettings(resolved *appconfig.Resolved) (EffectiveSettings, error) {
	if resolved == nil {
		return EffectiveSettings{}, fmt.Errorf("resolved config is required")
	}
	probe := ProbeGatewayPort(DefaultGatewayPort)
	hookURL, hookURLSource := resolveEffectiveHookURL(resolved, probe)
	if err := ValidateHookURL(hookURL); err != nil {
		return EffectiveSettings{}, err
	}
	token, tokenSource := ResolveHookToken(resolved.Paths)
	return EffectiveSettings{
		HookURL:                 hookURL,
		HookURLSource:           hookURLSource,
		DetectedWebhookPort:     probe.Port,
		DetectedWebhookPortInfo: probe,
		Token:                   token,
		TokenConfigured:         tokenSource != "unset",
		TokenSource:             tokenSource,
	}, nil
}

func ResolveHookToken(paths appconfig.Paths) (string, string) {
	if strings.TrimSpace(paths.ConfigFile) != "" {
		fileConfig, _, err := appconfig.ReadFileConfig(paths.ConfigFile)
		if err == nil {
			token := strings.TrimSpace(fileConfig.Runtime.HostNotify.OpenClaw.Token)
			if token != "" {
				return token, "config_file"
			}
		}
	}
	if token := strings.TrimSpace(os.Getenv(HookTokenEnv)); token != "" {
		return token, "environment"
	}
	return "", "unset"
}

func ProbeGatewayPort(defaultPort int) GatewayProbe {
	if envPort := strings.TrimSpace(os.Getenv(GatewayPortEnv)); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil && port > 0 {
			return GatewayProbe{Port: port, Source: "environment"}
		}
	}

	configPath := OpenClawConfigPath()
	raw, err := os.ReadFile(configPath)
	if err == nil {
		var payload struct {
			Gateway struct {
				Port int `json:"port"`
			} `json:"gateway"`
		}
		if err := json.Unmarshal(raw, &payload); err == nil && payload.Gateway.Port > 0 {
			return GatewayProbe{
				Port:       payload.Gateway.Port,
				Source:     "openclaw_config",
				ConfigPath: configPath,
			}
		}
	}

	return GatewayProbe{
		Port:       defaultPort,
		Source:     "default",
		ConfigPath: configPath,
	}
}

func OpenClawConfigPath() string {
	if value := strings.TrimSpace(os.Getenv(ConfigPathEnv)); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".openclaw", "openclaw.json")
	}
	return filepath.Join(home, ".openclaw", "openclaw.json")
}

func ValidateHookURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse runtime.host_notify.openclaw.hook_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("runtime.host_notify.openclaw.hook_url must use http or https")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("runtime.host_notify.openclaw.hook_url must include a host")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("runtime.host_notify.openclaw.hook_url must use a loopback host")
	}
	return nil
}

func resolveEffectiveHookURL(resolved *appconfig.Resolved, probe GatewayProbe) (string, string) {
	if resolved != nil {
		if source, ok := resolved.Sources["host_notify_openclaw_hook_url"]; ok && source.Source == "config_file" {
			if value := strings.TrimSpace(resolved.HostNotifyOpenClawHookURL); value != "" {
				return value, "config_file"
			}
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", probe.Port, DefaultHookPath), "auto_detected"
}
