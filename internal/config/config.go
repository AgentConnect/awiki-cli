package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	appName                    = "awiki-cli"
	configFileName             = "config.yaml"
	legacyConfigFileName       = "config.json"
	legacySkillName            = "awiki-agent-id-message"
	defaultServiceBaseURL      = "https://awiki.ai"
	defaultDIDDomain           = "awiki.ai"
	defaultANPPath             = "/anp-im/rpc"
	defaultRuntimeMode         = "websocket"
	defaultOutputFormat        = "json"
	defaultListenerEnabled     = true
	defaultListenerAutoInstall = true
	defaultListenerAutoStart   = true
	defaultHostNotifyEnabled   = true
	defaultHostNotifySink      = "log"
	defaultHostNotifyFile      = "host-notify.events.jsonl"
	defaultOpenClawHookURL     = "http://127.0.0.1:18789/hooks/agent"
	defaultOpenClawAgentID     = "main"
	defaultOpenClawHookName    = "AWiki"
	defaultHermesNotifyURL     = "http://127.0.0.1:8765/notify/host-event"
	defaultHermesDeliverTarget = "feishu"

	ConfigSchemaVersion = 1
)

var deprecatedEnvironmentVariables = []string{
	"AWIKI_WORKSPACE",
	"AWIKI_WORKSPACE_HOME",
	"AWIKI_HOME",
	"AVIKI_WORKSPACE_HOME",
	"AWIKI_CONFIG_DIR",
	"AWIKI_DATA_DIR",
	"AWIKI_STATE_DIR",
	"AWIKI_CACHE_DIR",
	"AWIKI_IDENTITY",
	"AWIKI_RUNTIME_MODE",
	"AWIKI_RUNTIME_SOCKET",
	"AWIKI_FORMAT",
	"AWIKI_NO_COLOR",
	"AWIKI_USER_SERVICE_URL",
	"AWIKI_MESSAGE_SERVICE_URL",
	"AWIKI_MESSAGE_WS_URL",
	"AWIKI_DID_DOMAIN",
	"AWIKI_ANP_SERVICE_ENDPOINT",
	"AWIKI_ANP_SERVICE_DID",
	"AWIKI_CA_BUNDLE",
	"AVIKI_CONFIG_DIR",
	"AVIKI_DATA_DIR",
	"AVIKI_STATE_DIR",
	"AVIKI_CACHE_DIR",
	"AVIKI_IDENTITY",
	"AVIKI_RUNTIME_MODE",
	"AVIKI_RUNTIME_SOCKET",
	"AVIKI_FORMAT",
	"AVIKI_NO_COLOR",
	"AVIKI_USER_SERVICE_URL",
	"AVIKI_MESSAGE_SERVICE_URL",
	"AVIKI_MESSAGE_WS_URL",
	"AVIKI_DID_DOMAIN",
	"AVIKI_ANP_SERVICE_ENDPOINT",
	"AVIKI_ANP_SERVICE_DID",
	"AVIKI_CA_BUNDLE",
	"E2E_USER_SERVICE_URL",
	"E2E_MOLT_MESSAGE_URL",
	"E2E_MOLT_MESSAGE_WS_URL",
	"E2E_DID_DOMAIN",
	"E2E_CA_BUNDLE",
}

type Overrides struct {
	Identity        string
	IdentityChanged bool
	Format          string
	FormatChanged   bool
}

type Paths struct {
	WorkspaceHomeDir     string `json:"workspace_home_dir"`
	RootDir              string `json:"root_dir"`
	ConfigDir            string `json:"config_dir"`
	DataDir              string `json:"data_dir"`
	StateDir             string `json:"state_dir"`
	CacheDir             string `json:"cache_dir"`
	LogsDir              string `json:"logs_dir"`
	ConfigFile           string `json:"config_file"`
	IdentityDir          string `json:"identity_dir"`
	DatabaseFile         string `json:"database_file"`
	LegacyCredentialsDir string `json:"legacy_credentials_dir"`
	LegacyDataDir        string `json:"legacy_data_dir"`
}

type FileConfig struct {
	SchemaVersion int `json:"schema_version,omitempty" yaml:"schema_version,omitempty"`
	Identity      struct {
		Active string `json:"active" yaml:"active"`
	} `json:"identity" yaml:"identity"`
	Runtime struct {
		Mode       string `json:"mode" yaml:"mode"`
		SocketPath string `json:"socket_path" yaml:"socket_path"`
		Listener   struct {
			Enabled     *bool `json:"enabled" yaml:"enabled"`
			AutoInstall *bool `json:"auto_install" yaml:"auto_install"`
			AutoStart   *bool `json:"auto_start" yaml:"auto_start"`
		} `json:"listener" yaml:"listener"`
		HostNotify struct {
			Enabled  *bool  `json:"enabled" yaml:"enabled"`
			Sink     string `json:"sink" yaml:"sink"`
			FilePath string `json:"file_path" yaml:"file_path"`
			OpenClaw struct {
				HookURL  string `json:"hook_url" yaml:"hook_url"`
				AgentID  string `json:"agent_id" yaml:"agent_id"`
				HookName string `json:"hook_name" yaml:"hook_name"`
				Token    string `json:"token" yaml:"token"`
			} `json:"openclaw" yaml:"openclaw"`
			Hermes struct {
				NotifyURL string `json:"notify_url" yaml:"notify_url"`
				Deliver   string `json:"deliver" yaml:"deliver"`
				Secret    string `json:"secret" yaml:"secret"`
			} `json:"hermes" yaml:"hermes"`
			LegacyWebhook struct {
				NotifyURL string `json:"notify_url" yaml:"notify_url"`
				Secret    string `json:"secret" yaml:"secret"`
			} `json:"webhook" yaml:"webhook"`
		} `json:"host_notify" yaml:"host_notify"`
	} `json:"runtime" yaml:"runtime"`
	Output struct {
		Format  string `json:"format" yaml:"format"`
		NoColor *bool  `json:"no_color" yaml:"no_color"`
	} `json:"output" yaml:"output"`
	Services struct {
		ServiceBaseURL     string `json:"service_base_url" yaml:"service_base_url"`
		DIDDomain          string `json:"did_domain" yaml:"did_domain"`
		ANPServiceEndpoint string `json:"anp_service_endpoint" yaml:"anp_service_endpoint"`
		ANPServiceDID      string `json:"anp_service_did" yaml:"anp_service_did"`
		CABundle           string `json:"ca_bundle" yaml:"ca_bundle"`
		MailServiceURL     string `json:"mail_service_url,omitempty" yaml:"mail_service_url,omitempty"`
	} `json:"services" yaml:"services"`
	Update struct {
		DisableStrictVersion    bool `json:"disable_strict_version" yaml:"disable_strict_version"`
		MetadataCacheTTLSeconds int  `json:"metadata_cache_ttl_seconds" yaml:"metadata_cache_ttl_seconds"`
	} `json:"update" yaml:"update"`
}

type EnvHit struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Tier   string `json:"tier"`
	Target string `json:"target"`
}

type ValueSource struct {
	Source string `json:"source"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}

type Resolved struct {
	Paths                         Paths                  `json:"paths"`
	ConfigSchemaVersion           int                    `json:"config_schema_version"`
	ActiveIdentity                string                 `json:"active_identity,omitempty"`
	RuntimeMode                   string                 `json:"runtime_mode"`
	RuntimeSocketPath             string                 `json:"runtime_socket_path,omitempty"`
	RuntimeListenerEnabled        bool                   `json:"runtime_listener_enabled"`
	RuntimeListenerAutoInstall    bool                   `json:"runtime_listener_auto_install"`
	RuntimeListenerAutoStart      bool                   `json:"runtime_listener_auto_start"`
	HostNotifyEnabled             bool                   `json:"host_notify_enabled"`
	HostNotifySink                string                 `json:"host_notify_sink"`
	HostNotifyFilePath            string                 `json:"host_notify_file_path,omitempty"`
	HostNotifyOpenClawHookURL     string                 `json:"host_notify_openclaw_hook_url,omitempty"`
	HostNotifyOpenClawAgentID     string                 `json:"host_notify_openclaw_agent_id,omitempty"`
	HostNotifyOpenClawHookName    string                 `json:"host_notify_openclaw_hook_name,omitempty"`
	HostNotifyHermesNotifyURL     string                 `json:"host_notify_hermes_notify_url,omitempty"`
	HostNotifyHermesDeliver       string                 `json:"host_notify_hermes_deliver,omitempty"`
	OutputFormat                  string                 `json:"output_format"`
	NoColor                       bool                   `json:"no_color"`
	ServiceBaseURL                string                 `json:"service_base_url"`
	DIDDomain                     string                 `json:"did_domain"`
	ANPServiceEndpoint            string                 `json:"anp_service_endpoint"`
	ANPServiceDID                 string                 `json:"anp_service_did"`
	MailServiceURL                string                 `json:"mail_service_url"`
	CABundle                      string                 `json:"ca_bundle,omitempty"`
	UpdateDisableStrictVersion    bool                   `json:"update_disable_strict_version"`
	UpdateMetadataCacheTTLSeconds int                    `json:"update_metadata_cache_ttl_seconds"`
	ConfigExists                  bool                   `json:"config_exists"`
	ConfigError                   string                 `json:"config_error,omitempty"`
	EnvHits                       []EnvHit               `json:"env_hits,omitempty"`
	Sources                       map[string]ValueSource `json:"sources"`
}

type PolicyError struct {
	Message string
	Hint    string
}

func (e *PolicyError) Error() string {
	return e.Message
}

func DefaultWorkspaceHomeDir(home string) string {
	return filepath.Join(home, "."+appName)
}

func Resolve(overrides Overrides) (*Resolved, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}

	workspaceHomeDir, workspaceHomeSource := resolveWorkspaceHome(home)
	paths := buildPaths(home, workspaceHomeDir)
	if err := validateUnsupportedConfiguration(paths); err != nil {
		return nil, err
	}

	resolved := &Resolved{
		Paths:          paths,
		RuntimeMode:    defaultRuntimeMode,
		OutputFormat:   defaultOutputFormat,
		ServiceBaseURL: defaultServiceBaseURL,
		DIDDomain:      defaultDIDDomain,
		EnvHits:        collectEnvHits(),
		Sources: map[string]ValueSource{
			"workspace_home_dir": workspaceHomeSource,
			"root_dir":           workspaceHomeSource,
			"config_dir": {
				Source: "derived",
				Key:    "workspace_home_dir",
				Value:  paths.ConfigDir,
			},
			"data_dir": {
				Source: "derived",
				Key:    "workspace_home_dir",
				Value:  paths.DataDir,
			},
			"state_dir": {
				Source: "derived",
				Key:    "workspace_home_dir",
				Value:  paths.StateDir,
			},
			"cache_dir": {
				Source: "derived",
				Key:    "workspace_home_dir",
				Value:  paths.CacheDir,
			},
			"logs_dir": {
				Source: "derived",
				Key:    "workspace_home_dir",
				Value:  paths.LogsDir,
			},
		},
	}

	fileConfig, configExists, configError := ReadFileConfig(paths.ConfigFile)
	resolved.ConfigExists = configExists
	resolved.ConfigSchemaVersion = normalizedConfigSchemaVersion(fileConfig.SchemaVersion)
	if configError != nil {
		resolved.ConfigError = configError.Error()
	}

	resolved.ActiveIdentity, resolved.Sources["active_identity"] = chooseValue(
		overrides.Identity,
		overrides.IdentityChanged,
		fileConfig.Identity.Active,
		"",
	)
	resolved.RuntimeMode, resolved.Sources["runtime_mode"] = chooseValue(
		"",
		false,
		fileConfig.Runtime.Mode,
		defaultRuntimeMode,
	)
	resolved.RuntimeSocketPath, resolved.Sources["runtime_socket_path"] = chooseValue(
		"",
		false,
		fileConfig.Runtime.SocketPath,
		defaultRuntimeBridgePath(paths),
	)
	resolved.RuntimeListenerEnabled, resolved.Sources["runtime_listener_enabled"] = chooseBool(fileConfig.Runtime.Listener.Enabled, defaultListenerEnabled)
	resolved.RuntimeListenerAutoInstall, resolved.Sources["runtime_listener_auto_install"] = chooseBool(fileConfig.Runtime.Listener.AutoInstall, defaultListenerAutoInstall)
	resolved.RuntimeListenerAutoStart, resolved.Sources["runtime_listener_auto_start"] = chooseBool(fileConfig.Runtime.Listener.AutoStart, defaultListenerAutoStart)
	resolved.HostNotifyEnabled, resolved.Sources["host_notify_enabled"] = chooseBool(fileConfig.Runtime.HostNotify.Enabled, defaultHostNotifyEnabled)
	resolved.HostNotifySink, resolved.Sources["host_notify_sink"] = chooseValue(
		"",
		false,
		fileConfig.Runtime.HostNotify.Sink,
		defaultHostNotifySink,
	)
	resolved.HostNotifySink = strings.ToLower(strings.TrimSpace(resolved.HostNotifySink))
	if resolved.HostNotifySink == "webhook" {
		resolved.HostNotifySink = "hermes"
		resolved.Sources["host_notify_sink"] = ValueSource{
			Source: "legacy_alias",
			Key:    "runtime.host_notify.sink",
			Value:  "hermes",
		}
	}
	if err := validateHostNotifySink(resolved.HostNotifySink); err != nil {
		return nil, err
	}
	hostNotifyFilePath := expandHome(home, fileConfig.Runtime.HostNotify.FilePath)
	if resolved.HostNotifySink == "file" {
		resolved.HostNotifyFilePath, resolved.Sources["host_notify_file_path"] = chooseValue(
			"",
			false,
			hostNotifyFilePath,
			"",
		)
		if strings.TrimSpace(resolved.HostNotifyFilePath) == "" {
			resolved.HostNotifyFilePath = filepath.Join(paths.StateDir, defaultHostNotifyFile)
			resolved.Sources["host_notify_file_path"] = ValueSource{
				Source: "derived_default",
				Key:    "state_dir",
				Value:  resolved.HostNotifyFilePath,
			}
		}
	} else {
		resolved.HostNotifyFilePath = ""
		resolved.Sources["host_notify_file_path"] = ValueSource{
			Source: "default",
			Value:  "",
		}
	}
	if resolved.HostNotifySink == "openclaw" {
		resolved.HostNotifyOpenClawHookURL, resolved.Sources["host_notify_openclaw_hook_url"] = chooseValue(
			"",
			false,
			fileConfig.Runtime.HostNotify.OpenClaw.HookURL,
			defaultOpenClawHookURL,
		)
		resolved.HostNotifyOpenClawAgentID, resolved.Sources["host_notify_openclaw_agent_id"] = chooseValue(
			"",
			false,
			fileConfig.Runtime.HostNotify.OpenClaw.AgentID,
			defaultOpenClawAgentID,
		)
		resolved.HostNotifyOpenClawHookName, resolved.Sources["host_notify_openclaw_hook_name"] = chooseValue(
			"",
			false,
			fileConfig.Runtime.HostNotify.OpenClaw.HookName,
			defaultOpenClawHookName,
		)
	} else {
		resolved.HostNotifyOpenClawHookURL = ""
		resolved.Sources["host_notify_openclaw_hook_url"] = ValueSource{Source: "default", Value: ""}
		resolved.HostNotifyOpenClawAgentID = ""
		resolved.Sources["host_notify_openclaw_agent_id"] = ValueSource{Source: "default", Value: ""}
		resolved.HostNotifyOpenClawHookName = ""
		resolved.Sources["host_notify_openclaw_hook_name"] = ValueSource{Source: "default", Value: ""}
	}
	hermesNotifyURL := strings.TrimSpace(fileConfig.Runtime.HostNotify.Hermes.NotifyURL)
	if hermesNotifyURL == "" {
		hermesNotifyURL = strings.TrimSpace(fileConfig.Runtime.HostNotify.LegacyWebhook.NotifyURL)
	}
	if resolved.HostNotifySink == "hermes" {
		resolved.HostNotifyHermesNotifyURL, resolved.Sources["host_notify_hermes_notify_url"] = chooseValue(
			"",
			false,
			hermesNotifyURL,
			defaultHermesNotifyURL,
		)
		resolved.HostNotifyHermesDeliver, resolved.Sources["host_notify_hermes_deliver"] = chooseValue(
			"",
			false,
			fileConfig.Runtime.HostNotify.Hermes.Deliver,
			defaultHermesDeliverTarget,
		)
		resolved.HostNotifyHermesDeliver = strings.ToLower(strings.TrimSpace(resolved.HostNotifyHermesDeliver))
	} else {
		resolved.HostNotifyHermesNotifyURL = ""
		resolved.Sources["host_notify_hermes_notify_url"] = ValueSource{Source: "default", Value: ""}
		resolved.HostNotifyHermesDeliver = ""
		resolved.Sources["host_notify_hermes_deliver"] = ValueSource{Source: "default", Value: ""}
	}
	resolved.OutputFormat, resolved.Sources["output_format"] = chooseValue(
		overrides.Format,
		overrides.FormatChanged,
		fileConfig.Output.Format,
		defaultOutputFormat,
	)
	resolved.NoColor, resolved.Sources["no_color"] = chooseBool(fileConfig.Output.NoColor, false)
	resolved.ServiceBaseURL, resolved.Sources["service_base_url"] = chooseValue(
		"",
		false,
		fileConfig.Services.ServiceBaseURL,
		defaultServiceBaseURL,
	)
	resolved.ServiceBaseURL = NormalizeBaseURL(resolved.ServiceBaseURL)
	if source, ok := resolved.Sources["service_base_url"]; ok {
		source.Value = resolved.ServiceBaseURL
		resolved.Sources["service_base_url"] = source
	}
	resolved.DIDDomain, resolved.Sources["did_domain"] = chooseValue(
		"",
		false,
		fileConfig.Services.DIDDomain,
		defaultDIDDomain,
	)
	resolved.ANPServiceEndpoint, resolved.Sources["anp_service_endpoint"] = chooseValue(
		"",
		false,
		fileConfig.Services.ANPServiceEndpoint,
		"",
	)
	if strings.TrimSpace(resolved.ANPServiceEndpoint) == "" {
		resolved.ANPServiceEndpoint = DeriveANPServiceEndpoint(resolved.ServiceBaseURL)
		resolved.Sources["anp_service_endpoint"] = ValueSource{
			Source: "derived_default",
			Key:    "service_base_url",
			Value:  resolved.ANPServiceEndpoint,
		}
	}
	resolved.ANPServiceDID, resolved.Sources["anp_service_did"] = chooseValue(
		"",
		false,
		fileConfig.Services.ANPServiceDID,
		"",
	)
	if strings.TrimSpace(resolved.ANPServiceDID) == "" {
		resolved.ANPServiceDID = DeriveANPServiceDID(resolved.ServiceBaseURL)
		resolved.Sources["anp_service_did"] = ValueSource{
			Source: "derived_default",
			Key:    "service_base_url",
			Value:  resolved.ANPServiceDID,
		}
	}

	// Mail service URL derives from explicit config if present, otherwise from service_base_url.
	mailServiceURL := strings.TrimSpace(fileConfig.Services.MailServiceURL)
	mailServiceSource := ValueSource{
		Source: "derived_default",
		Key:    "service_base_url",
		Value:  resolved.ServiceBaseURL,
	}
	if mailServiceURL != "" {
		mailServiceURL = NormalizeBaseURL(mailServiceURL)
		mailServiceSource = ValueSource{Source: "config_file", Value: mailServiceURL}
	} else {
		mailServiceURL = resolved.ServiceBaseURL
	}
	resolved.MailServiceURL = mailServiceURL
	resolved.Sources["mail_service_url"] = mailServiceSource

	resolved.CABundle, resolved.Sources["ca_bundle"] = chooseValue(
		"",
		false,
		fileConfig.Services.CABundle,
		"",
	)

	// Update-related knobs (no env overrides at this layer; env is handled in internal/update).
	resolved.UpdateDisableStrictVersion = fileConfig.Update.DisableStrictVersion
	resolved.Sources["update_disable_strict_version"] = ValueSource{
		Source: "config_file_or_default",
		Value:  fmt.Sprintf("%t", resolved.UpdateDisableStrictVersion),
	}

	resolved.UpdateMetadataCacheTTLSeconds = fileConfig.Update.MetadataCacheTTLSeconds
	resolved.Sources["update_metadata_cache_ttl_seconds"] = ValueSource{
		Source: "config_file_or_default",
		Value:  fmt.Sprintf("%d", resolved.UpdateMetadataCacheTTLSeconds),
	}

	if resolved.OutputFormat == "" {
		resolved.OutputFormat = defaultOutputFormat
	}
	return resolved, nil
}

func Snapshot(resolved *Resolved) map[string]any {
	if resolved == nil {
		return map[string]any{}
	}
	return map[string]any{
		"paths":                             resolved.Paths,
		"config_schema_version":             resolved.ConfigSchemaVersion,
		"active_identity":                   resolved.ActiveIdentity,
		"runtime_mode":                      resolved.RuntimeMode,
		"runtime_socket_path":               resolved.RuntimeSocketPath,
		"runtime_listener_enabled":          resolved.RuntimeListenerEnabled,
		"runtime_listener_auto_install":     resolved.RuntimeListenerAutoInstall,
		"runtime_listener_auto_start":       resolved.RuntimeListenerAutoStart,
		"host_notify_enabled":               resolved.HostNotifyEnabled,
		"host_notify_sink":                  resolved.HostNotifySink,
		"host_notify_file_path":             resolved.HostNotifyFilePath,
		"host_notify_openclaw_hook_url":     resolved.HostNotifyOpenClawHookURL,
		"host_notify_openclaw_agent_id":     resolved.HostNotifyOpenClawAgentID,
		"host_notify_openclaw_hook_name":    resolved.HostNotifyOpenClawHookName,
		"host_notify_hermes_notify_url":     resolved.HostNotifyHermesNotifyURL,
		"host_notify_hermes_deliver":        resolved.HostNotifyHermesDeliver,
		"output_format":                     resolved.OutputFormat,
		"no_color":                          resolved.NoColor,
		"service_base_url":                  resolved.ServiceBaseURL,
		"did_domain":                        resolved.DIDDomain,
		"anp_service_endpoint":              resolved.ANPServiceEndpoint,
		"anp_service_did":                   resolved.ANPServiceDID,
		"mail_service_url":                  resolved.MailServiceURL,
		"ca_bundle":                         resolved.CABundle,
		"update_disable_strict_version":     resolved.UpdateDisableStrictVersion,
		"update_metadata_cache_ttl_seconds": resolved.UpdateMetadataCacheTTLSeconds,
		"config_exists":                     resolved.ConfigExists,
		"config_error":                      resolved.ConfigError,
		"env_hits":                          resolved.EnvHits,
		"sources":                           resolved.Sources,
	}
}

func ReadFileConfig(path string) (FileConfig, bool, error) {
	var config FileConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config, false, nil
		}
		return config, false, err
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return config, true, err
	}
	return config, true, nil
}

func normalizedConfigSchemaVersion(version int) int {
	if version < 0 {
		return 0
	}
	return version
}

func resolveWorkspaceHome(home string) (string, ValueSource) {
	return resolvePath(home, "AWIKI_CLI_WORKSPACE_HOME_DIR", DefaultWorkspaceHomeDir(home))
}

func resolvePath(home string, key string, defaultValue string) (string, ValueSource) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		expanded := expandHome(home, value)
		return expanded, ValueSource{Source: "canonical_env", Key: key, Value: expanded}
	}
	return defaultValue, ValueSource{Source: "default", Value: defaultValue}
}

func chooseValue(flagValue string, flagChanged bool, fileValue string, defaultValue string) (string, ValueSource) {
	if flagChanged && strings.TrimSpace(flagValue) != "" {
		value := strings.TrimSpace(flagValue)
		return value, ValueSource{Source: "flag", Value: value}
	}
	if strings.TrimSpace(fileValue) != "" {
		value := strings.TrimSpace(fileValue)
		return value, ValueSource{Source: "config_file", Value: value}
	}
	return defaultValue, ValueSource{Source: "default", Value: defaultValue}
}

func chooseBool(fileValue *bool, defaultValue bool) (bool, ValueSource) {
	if fileValue != nil {
		return *fileValue, ValueSource{Source: "config_file", Value: fmt.Sprintf("%t", *fileValue)}
	}
	return defaultValue, ValueSource{Source: "default", Value: fmt.Sprintf("%t", defaultValue)}
}

func expandHome(home string, value string) string {
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}

// DeriveANPServiceEndpoint returns the default public ANP RPC endpoint for a service base URL.
func DeriveANPServiceEndpoint(serviceBaseURL string) string {
	normalizedBaseURL := NormalizeBaseURL(serviceBaseURL)
	if normalizedBaseURL == "" {
		normalizedBaseURL = defaultServiceBaseURL
	}
	return JoinBaseURL(normalizedBaseURL, defaultANPPath)
}

// DeriveANPServiceDID returns the default bare-domain service DID for a service base URL.
func DeriveANPServiceDID(serviceBaseURL string) string {
	return "did:wba:" + serviceHostFromBaseURL(serviceBaseURL)
}

func serviceHostFromBaseURL(serviceBaseURL string) string {
	trimmed := NormalizeBaseURL(serviceBaseURL)
	if trimmed == "" {
		trimmed = defaultServiceBaseURL
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}
	if parsed, err := url.Parse("//" + trimmed); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}
	return defaultDIDDomain
}

func defaultRuntimeBridgePath(paths Paths) string {
	if goruntime.GOOS == "windows" {
		workspace := strings.TrimSpace(paths.WorkspaceHomeDir)
		if workspace == "" {
			workspace = filepath.Join(os.TempDir(), "awiki-cli")
		}
		sum := sha256Bytes(workspace)
		return `\\.\pipe\awiki-cli-` + sum[:16]
	}
	stateDir := strings.TrimSpace(paths.StateDir)
	if stateDir == "" {
		stateDir = filepath.Join(paths.WorkspaceHomeDir, "runtime")
	}
	return filepath.Join(stateDir, "message-daemon.sock")
}

func sha256Bytes(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func LegacyConfigPath(paths Paths) string {
	return filepath.Join(paths.ConfigDir, legacyConfigFileName)
}

func buildPaths(home string, workspaceHomeDir string) Paths {
	configDir := workspaceHomeDir
	dataDir := filepath.Join(workspaceHomeDir, "data")
	stateDir := filepath.Join(workspaceHomeDir, "runtime")
	cacheDir := filepath.Join(workspaceHomeDir, "cache")
	logsDir := filepath.Join(workspaceHomeDir, "logs")

	return Paths{
		WorkspaceHomeDir:     workspaceHomeDir,
		RootDir:              workspaceHomeDir,
		ConfigDir:            configDir,
		DataDir:              dataDir,
		StateDir:             stateDir,
		CacheDir:             cacheDir,
		LogsDir:              logsDir,
		ConfigFile:           filepath.Join(configDir, configFileName),
		IdentityDir:          filepath.Join(configDir, "identities"),
		DatabaseFile:         filepath.Join(dataDir, appName+".db"),
		LegacyCredentialsDir: filepath.Join(home, ".openclaw", "credentials", legacySkillName),
		LegacyDataDir:        filepath.Join(home, ".openclaw", "workspace", "data", legacySkillName),
	}
}

func validateUnsupportedConfiguration(paths Paths) error {
	issues := make([]string, 0, 2)

	legacyConfigPath := LegacyConfigPath(paths)
	configExists := fileExists(paths.ConfigFile)
	legacyConfigExists := fileExists(legacyConfigPath)
	if configExists && legacyConfigExists {
		issues = append(issues, fmt.Sprintf(
			"found both %s and unsupported legacy %s in the workspace",
			paths.ConfigFile,
			legacyConfigPath,
		))
	}
	if deprecatedFields, err := collectDeprecatedConfigFields(paths.ConfigFile); err != nil {
		issues = append(issues, err.Error())
	} else if len(deprecatedFields) > 0 {
		issues = append(issues, fmt.Sprintf(
			"deprecated config.yaml fields are no longer supported: %s",
			strings.Join(deprecatedFields, ", "),
		))
	}

	if len(issues) == 0 {
		return nil
	}
	return &PolicyError{
		Message: strings.Join(issues, "; "),
		Hint: fmt.Sprintf(
			"Use AWIKI_CLI_WORKSPACE_HOME_DIR only for workspace selection and move all CLI settings into %s.",
			paths.ConfigFile,
		),
	}
}

func collectEnvHits() []EnvHit {
	value := strings.TrimSpace(os.Getenv("AWIKI_CLI_WORKSPACE_HOME_DIR"))
	if value == "" {
		return nil
	}
	return []EnvHit{{
		Key:    "AWIKI_CLI_WORKSPACE_HOME_DIR",
		Value:  value,
		Tier:   "canonical_env",
		Target: "workspace_home_dir",
	}}
}

func collectDeprecatedEnvKeys() []string {
	keys := make([]string, 0, len(deprecatedEnvironmentVariables))
	for _, key := range deprecatedEnvironmentVariables {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func collectDeprecatedConfigFields(path string) ([]string, error) {
	if !fileExists(path) {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config yaml for policy validation: %w", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, nil
	}
	services, ok := parsed["services"].(map[string]any)
	if !ok {
		return nil, nil
	}
	deprecated := make([]string, 0, 3)
	for _, key := range []string{"user_service_url", "message_service_url", "message_service_ws_url"} {
		if _, exists := services[key]; exists {
			deprecated = append(deprecated, "services."+key)
		}
	}
	return deprecated, nil
}

func validateHostNotifySink(value string) error {
	switch value {
	case "", "noop", "log", "file", "openclaw", "hermes", "webhook":
		return nil
	default:
		return &PolicyError{
			Message: fmt.Sprintf("unsupported runtime.host_notify.sink %q", value),
			Hint:    "Use runtime.host_notify.sink = noop, log, file, openclaw, or hermes in config.yaml.",
		}
	}
}

func NormalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func JoinBaseURL(baseURL string, path string) string {
	normalizedBaseURL := NormalizeBaseURL(baseURL)
	if normalizedBaseURL == "" {
		return strings.TrimSpace(path)
	}
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		return normalizedBaseURL
	}
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	return normalizedBaseURL + normalizedPath
}

func DeriveWebSocketURL(baseURL string, path string) string {
	httpURL := JoinBaseURL(baseURL, path)
	trimmed := strings.TrimSpace(httpURL)
	switch {
	case strings.HasPrefix(trimmed, "https://"):
		return "wss://" + strings.TrimPrefix(trimmed, "https://")
	case strings.HasPrefix(trimmed, "http://"):
		return "ws://" + strings.TrimPrefix(trimmed, "http://")
	default:
		return trimmed
	}
}
