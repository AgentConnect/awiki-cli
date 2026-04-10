package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	appName             = "awiki-cli"
	legacySkillName     = "awiki-agent-id-message"
	defaultService      = "https://awiki.ai"
	defaultDIDDomain    = "awiki.ai"
	defaultANPPath      = "/message/rpc"
	ConfigSchemaVersion = 1
)

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
	SchemaVersion int `yaml:"schema_version,omitempty"`
	Identity      struct {
		Active string `yaml:"active"`
	} `yaml:"identity"`
	Runtime struct {
		Mode       string `yaml:"mode"`
		SocketPath string `yaml:"socket_path"`
	} `yaml:"runtime"`
	Output struct {
		Format  string `yaml:"format"`
		NoColor *bool  `yaml:"no_color"`
	} `yaml:"output"`
	Services struct {
		UserServiceURL      string `yaml:"user_service_url"`
		MessageServiceURL   string `yaml:"message_service_url"`
		MessageServiceWSURL string `yaml:"message_service_ws_url"`
		DIDDomain           string `yaml:"did_domain"`
		ANPServiceEndpoint  string `yaml:"anp_service_endpoint"`
		ANPServiceDID       string `yaml:"anp_service_did"`
		CABundle            string `yaml:"ca_bundle"`
	} `yaml:"services"`
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
	Paths               Paths                  `json:"paths"`
	ConfigSchemaVersion int                    `json:"config_schema_version"`
	ActiveIdentity      string                 `json:"active_identity,omitempty"`
	RuntimeMode         string                 `json:"runtime_mode"`
	RuntimeSocketPath   string                 `json:"runtime_socket_path,omitempty"`
	OutputFormat        string                 `json:"output_format"`
	NoColor             bool                   `json:"no_color"`
	UserServiceURL      string                 `json:"user_service_url"`
	MessageServiceURL   string                 `json:"message_service_url"`
	MessageServiceWSURL string                 `json:"message_service_ws_url,omitempty"`
	DIDDomain           string                 `json:"did_domain"`
	ANPServiceEndpoint  string                 `json:"anp_service_endpoint"`
	ANPServiceDID       string                 `json:"anp_service_did"`
	CABundle            string                 `json:"ca_bundle,omitempty"`
	ConfigExists        bool                   `json:"config_exists"`
	ConfigError         string                 `json:"config_error,omitempty"`
	EnvHits             []EnvHit               `json:"env_hits,omitempty"`
	Sources             map[string]ValueSource `json:"sources"`
}

type option struct {
	key    string
	target string
	tier   string
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
	configDir, configDirSource := resolvePath(
		home,
		envOptionSet("config_dir", "AWIKI_CONFIG_DIR", "AVIKI_CONFIG_DIR"),
		workspaceHomeDir,
	)
	dataDir, dataDirSource := resolvePath(
		home,
		envOptionSet("data_dir", "AWIKI_DATA_DIR", "AVIKI_DATA_DIR"),
		filepath.Join(workspaceHomeDir, "data"),
	)
	stateDir, stateDirSource := resolvePath(
		home,
		envOptionSet("state_dir", "AWIKI_STATE_DIR", "AVIKI_STATE_DIR"),
		filepath.Join(workspaceHomeDir, "runtime"),
	)
	cacheDir, cacheDirSource := resolvePath(
		home,
		envOptionSet("cache_dir", "AWIKI_CACHE_DIR", "AVIKI_CACHE_DIR"),
		filepath.Join(workspaceHomeDir, "cache"),
	)
	logsDir := filepath.Join(workspaceHomeDir, "logs")

	legacyWorkspace := strings.TrimSpace(os.Getenv("AWIKI_WORKSPACE"))
	legacyDataDir := filepath.Join(home, ".openclaw", "workspace", "data", legacySkillName)
	if legacyWorkspace != "" {
		legacyDataDir = filepath.Join(expandHome(home, legacyWorkspace), "data", legacySkillName)
	}

	paths := Paths{
		WorkspaceHomeDir:     workspaceHomeDir,
		RootDir:              workspaceHomeDir,
		ConfigDir:            configDir,
		DataDir:              dataDir,
		StateDir:             stateDir,
		CacheDir:             cacheDir,
		LogsDir:              logsDir,
		ConfigFile:           filepath.Join(configDir, "config.yaml"),
		IdentityDir:          filepath.Join(configDir, "identities"),
		DatabaseFile:         filepath.Join(dataDir, appName+".db"),
		LegacyCredentialsDir: filepath.Join(home, ".openclaw", "credentials", legacySkillName),
		LegacyDataDir:        legacyDataDir,
	}

	resolved := &Resolved{
		Paths:               paths,
		RuntimeMode:         "http",
		OutputFormat:        "json",
		UserServiceURL:      defaultService,
		MessageServiceURL:   defaultService,
		MessageServiceWSURL: "",
		DIDDomain:           defaultDIDDomain,
		Sources: map[string]ValueSource{
			"workspace_home_dir": workspaceHomeSource,
			"root_dir":           workspaceHomeSource,
			"config_dir":         configDirSource,
			"data_dir":           dataDirSource,
			"state_dir":          stateDirSource,
			"cache_dir":          cacheDirSource,
			"logs_dir": {
				Source: "derived",
				Key:    "workspace_home_dir",
				Value:  logsDir,
			},
		},
	}
	resolved.EnvHits = collectEnvHits()

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
		envOptionSet("active_identity", "AWIKI_IDENTITY", "AVIKI_IDENTITY"),
		"",
	)
	resolved.RuntimeMode, resolved.Sources["runtime_mode"] = chooseValue(
		"",
		false,
		fileConfig.Runtime.Mode,
		envOptionSet("runtime_mode", "AWIKI_RUNTIME_MODE", "AVIKI_RUNTIME_MODE"),
		"http",
	)
	resolved.RuntimeSocketPath, resolved.Sources["runtime_socket_path"] = chooseValue(
		"",
		false,
		fileConfig.Runtime.SocketPath,
		envOptionSet("runtime_socket_path", "AWIKI_RUNTIME_SOCKET", "AVIKI_RUNTIME_SOCKET"),
		filepath.Join(paths.StateDir, "message-daemon.sock"),
	)
	resolved.OutputFormat, resolved.Sources["output_format"] = chooseValue(
		overrides.Format,
		overrides.FormatChanged,
		fileConfig.Output.Format,
		envOptionSet("output_format", "AWIKI_FORMAT", "AVIKI_FORMAT"),
		"json",
	)
	resolved.NoColor, resolved.Sources["no_color"] = chooseBool(
		fileConfig.Output.NoColor,
		envOptionSet("no_color", "AWIKI_NO_COLOR", "AVIKI_NO_COLOR"),
		false,
	)
	resolved.UserServiceURL, resolved.Sources["user_service_url"] = chooseValue(
		"",
		false,
		fileConfig.Services.UserServiceURL,
		append(
			envOptionSet("user_service_url", "AWIKI_USER_SERVICE_URL", "AVIKI_USER_SERVICE_URL"),
			option{key: "E2E_USER_SERVICE_URL", target: "user_service_url", tier: "legacy_env"},
		),
		defaultService,
	)
	resolved.MessageServiceURL, resolved.Sources["message_service_url"] = chooseValue(
		"",
		false,
		fileConfig.Services.MessageServiceURL,
		append(
			envOptionSet("message_service_url", "AWIKI_MESSAGE_SERVICE_URL", "AVIKI_MESSAGE_SERVICE_URL"),
			option{key: "E2E_MOLT_MESSAGE_URL", target: "message_service_url", tier: "legacy_env"},
		),
		defaultService,
	)
	resolved.MessageServiceWSURL, resolved.Sources["message_service_ws_url"] = chooseValue(
		"",
		false,
		fileConfig.Services.MessageServiceWSURL,
		append(
			envOptionSet("message_service_ws_url", "AWIKI_MESSAGE_WS_URL", "AVIKI_MESSAGE_WS_URL"),
			option{key: "E2E_MOLT_MESSAGE_WS_URL", target: "message_service_ws_url", tier: "legacy_env"},
		),
		"",
	)
	resolved.DIDDomain, resolved.Sources["did_domain"] = chooseValue(
		"",
		false,
		fileConfig.Services.DIDDomain,
		append(
			envOptionSet("did_domain", "AWIKI_DID_DOMAIN", "AVIKI_DID_DOMAIN"),
			option{key: "E2E_DID_DOMAIN", target: "did_domain", tier: "legacy_env"},
		),
		defaultDIDDomain,
	)
	resolved.ANPServiceEndpoint, resolved.Sources["anp_service_endpoint"] = chooseValue(
		"",
		false,
		fileConfig.Services.ANPServiceEndpoint,
		envOptionSet("anp_service_endpoint", "AWIKI_ANP_SERVICE_ENDPOINT", "AVIKI_ANP_SERVICE_ENDPOINT"),
		"",
	)
	if strings.TrimSpace(resolved.ANPServiceEndpoint) == "" {
		resolved.ANPServiceEndpoint = defaultANPServiceEndpoint(resolved.DIDDomain)
		resolved.Sources["anp_service_endpoint"] = ValueSource{
			Source: "derived_default",
			Key:    "did_domain",
			Value:  resolved.ANPServiceEndpoint,
		}
	}
	resolved.ANPServiceDID, resolved.Sources["anp_service_did"] = chooseValue(
		"",
		false,
		fileConfig.Services.ANPServiceDID,
		envOptionSet("anp_service_did", "AWIKI_ANP_SERVICE_DID", "AVIKI_ANP_SERVICE_DID"),
		"",
	)
	if strings.TrimSpace(resolved.ANPServiceDID) == "" {
		resolved.ANPServiceDID = defaultANPServiceDID(resolved.DIDDomain)
		resolved.Sources["anp_service_did"] = ValueSource{
			Source: "derived_default",
			Key:    "did_domain",
			Value:  resolved.ANPServiceDID,
		}
	}
	resolved.CABundle, resolved.Sources["ca_bundle"] = chooseValue(
		"",
		false,
		fileConfig.Services.CABundle,
		append(
			envOptionSet("ca_bundle", "AWIKI_CA_BUNDLE", "AVIKI_CA_BUNDLE"),
			option{key: "E2E_CA_BUNDLE", target: "ca_bundle", tier: "legacy_env"},
		),
		"",
	)

	if resolved.OutputFormat == "" {
		resolved.OutputFormat = "json"
	}
	return resolved, nil
}

func Snapshot(resolved *Resolved) map[string]any {
	if resolved == nil {
		return map[string]any{}
	}
	return map[string]any{
		"paths":                  resolved.Paths,
		"config_schema_version":  resolved.ConfigSchemaVersion,
		"active_identity":        resolved.ActiveIdentity,
		"runtime_mode":           resolved.RuntimeMode,
		"runtime_socket_path":    resolved.RuntimeSocketPath,
		"output_format":          resolved.OutputFormat,
		"no_color":               resolved.NoColor,
		"user_service_url":       resolved.UserServiceURL,
		"message_service_url":    resolved.MessageServiceURL,
		"message_service_ws_url": resolved.MessageServiceWSURL,
		"did_domain":             resolved.DIDDomain,
		"anp_service_endpoint":   resolved.ANPServiceEndpoint,
		"anp_service_did":        resolved.ANPServiceDID,
		"ca_bundle":              resolved.CABundle,
		"config_exists":          resolved.ConfigExists,
		"config_error":           resolved.ConfigError,
		"env_hits":               resolved.EnvHits,
		"sources":                resolved.Sources,
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

func envOptionSet(target string, keys ...string) []option {
	options := make([]option, 0, len(keys))
	for _, key := range keys {
		tier := "canonical_env"
		if strings.HasPrefix(key, "AVIKI_") {
			tier = "draft_alias_env"
		}
		options = append(options, option{key: key, target: target, tier: tier})
	}
	return options
}

func resolveWorkspaceHome(home string) (string, ValueSource) {
	options := []option{
		{key: "AWIKI_WORKSPACE_HOME", target: "workspace_home_dir", tier: "canonical_env"},
		{key: "AWIKI_HOME", target: "workspace_home_dir", tier: "canonical_env"},
		{key: "AVIKI_WORKSPACE_HOME", target: "workspace_home_dir", tier: "draft_alias_env"},
	}
	return resolvePath(home, options, DefaultWorkspaceHomeDir(home))
}

func resolvePath(home string, options []option, defaultValue string) (string, ValueSource) {
	for _, opt := range options {
		if value := strings.TrimSpace(os.Getenv(opt.key)); value != "" {
			expanded := expandHome(home, value)
			return expanded, ValueSource{Source: opt.tier, Key: opt.key, Value: expanded}
		}
	}
	return defaultValue, ValueSource{Source: "default", Value: defaultValue}
}

func chooseValue(flagValue string, flagChanged bool, fileValue string, envOptions []option, defaultValue string) (string, ValueSource) {
	if flagChanged && strings.TrimSpace(flagValue) != "" {
		value := strings.TrimSpace(flagValue)
		return value, ValueSource{Source: "flag", Value: value}
	}
	if strings.TrimSpace(fileValue) != "" {
		value := strings.TrimSpace(fileValue)
		return value, ValueSource{Source: "config_file", Value: value}
	}
	for _, opt := range envOptions {
		if value := strings.TrimSpace(os.Getenv(opt.key)); value != "" {
			return value, ValueSource{Source: opt.tier, Key: opt.key, Value: value}
		}
	}
	return defaultValue, ValueSource{Source: "default", Value: defaultValue}
}

func chooseBool(fileValue *bool, envOptions []option, defaultValue bool) (bool, ValueSource) {
	if fileValue != nil {
		return *fileValue, ValueSource{Source: "config_file", Value: fmt.Sprintf("%t", *fileValue)}
	}
	for _, opt := range envOptions {
		if value := strings.TrimSpace(os.Getenv(opt.key)); value != "" {
			parsed := strings.EqualFold(value, "1") || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
			return parsed, ValueSource{Source: opt.tier, Key: opt.key, Value: value}
		}
	}
	return defaultValue, ValueSource{Source: "default", Value: fmt.Sprintf("%t", defaultValue)}
}

func expandHome(home string, value string) string {
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}

func defaultANPServiceEndpoint(didDomain string) string {
	trimmedDomain := strings.TrimSpace(didDomain)
	if trimmedDomain == "" {
		trimmedDomain = defaultDIDDomain
	}
	return "https://" + trimmedDomain + defaultANPPath
}

func defaultANPServiceDID(didDomain string) string {
	trimmedDomain := strings.TrimSpace(didDomain)
	if trimmedDomain == "" {
		trimmedDomain = defaultDIDDomain
	}
	return "did:wba:" + trimmedDomain
}

func collectEnvHits() []EnvHit {
	definitions := []EnvHit{
		{Key: "AWIKI_WORKSPACE_HOME", Tier: "canonical_env", Target: "workspace_home_dir"},
		{Key: "AWIKI_HOME", Tier: "canonical_env", Target: "workspace_home_dir"},
		{Key: "AWIKI_CONFIG_DIR", Tier: "canonical_env", Target: "config_dir"},
		{Key: "AWIKI_DATA_DIR", Tier: "canonical_env", Target: "data_dir"},
		{Key: "AWIKI_STATE_DIR", Tier: "canonical_env", Target: "state_dir"},
		{Key: "AWIKI_CACHE_DIR", Tier: "canonical_env", Target: "cache_dir"},
		{Key: "AWIKI_IDENTITY", Tier: "canonical_env", Target: "active_identity"},
		{Key: "AWIKI_RUNTIME_MODE", Tier: "canonical_env", Target: "runtime_mode"},
		{Key: "AWIKI_RUNTIME_SOCKET", Tier: "canonical_env", Target: "runtime_socket_path"},
		{Key: "AWIKI_FORMAT", Tier: "canonical_env", Target: "output_format"},
		{Key: "AWIKI_NO_COLOR", Tier: "canonical_env", Target: "no_color"},
		{Key: "AWIKI_USER_SERVICE_URL", Tier: "canonical_env", Target: "user_service_url"},
		{Key: "AWIKI_MESSAGE_SERVICE_URL", Tier: "canonical_env", Target: "message_service_url"},
		{Key: "AWIKI_MESSAGE_WS_URL", Tier: "canonical_env", Target: "message_service_ws_url"},
		{Key: "AWIKI_DID_DOMAIN", Tier: "canonical_env", Target: "did_domain"},
		{Key: "AWIKI_ANP_SERVICE_ENDPOINT", Tier: "canonical_env", Target: "anp_service_endpoint"},
		{Key: "AWIKI_ANP_SERVICE_DID", Tier: "canonical_env", Target: "anp_service_did"},
		{Key: "AWIKI_CA_BUNDLE", Tier: "canonical_env", Target: "ca_bundle"},
		{Key: "AWIKI_WORKSPACE", Tier: "canonical_env", Target: "legacy_workspace"},
		{Key: "AVIKI_WORKSPACE_HOME", Tier: "draft_alias_env", Target: "workspace_home_dir"},
		{Key: "AVIKI_CONFIG_DIR", Tier: "draft_alias_env", Target: "config_dir"},
		{Key: "AVIKI_DATA_DIR", Tier: "draft_alias_env", Target: "data_dir"},
		{Key: "AVIKI_STATE_DIR", Tier: "draft_alias_env", Target: "state_dir"},
		{Key: "AVIKI_CACHE_DIR", Tier: "draft_alias_env", Target: "cache_dir"},
		{Key: "AVIKI_IDENTITY", Tier: "draft_alias_env", Target: "active_identity"},
		{Key: "AVIKI_RUNTIME_MODE", Tier: "draft_alias_env", Target: "runtime_mode"},
		{Key: "AVIKI_RUNTIME_SOCKET", Tier: "draft_alias_env", Target: "runtime_socket_path"},
		{Key: "AVIKI_FORMAT", Tier: "draft_alias_env", Target: "output_format"},
		{Key: "AVIKI_NO_COLOR", Tier: "draft_alias_env", Target: "no_color"},
		{Key: "AVIKI_USER_SERVICE_URL", Tier: "draft_alias_env", Target: "user_service_url"},
		{Key: "AVIKI_MESSAGE_SERVICE_URL", Tier: "draft_alias_env", Target: "message_service_url"},
		{Key: "AVIKI_MESSAGE_WS_URL", Tier: "draft_alias_env", Target: "message_service_ws_url"},
		{Key: "AVIKI_DID_DOMAIN", Tier: "draft_alias_env", Target: "did_domain"},
		{Key: "AVIKI_ANP_SERVICE_ENDPOINT", Tier: "draft_alias_env", Target: "anp_service_endpoint"},
		{Key: "AVIKI_ANP_SERVICE_DID", Tier: "draft_alias_env", Target: "anp_service_did"},
		{Key: "AVIKI_CA_BUNDLE", Tier: "draft_alias_env", Target: "ca_bundle"},
		{Key: "E2E_USER_SERVICE_URL", Tier: "legacy_env", Target: "user_service_url"},
		{Key: "E2E_MOLT_MESSAGE_URL", Tier: "legacy_env", Target: "message_service_url"},
		{Key: "E2E_MOLT_MESSAGE_WS_URL", Tier: "legacy_env", Target: "message_service_ws_url"},
		{Key: "E2E_DID_DOMAIN", Tier: "legacy_env", Target: "did_domain"},
		{Key: "E2E_CA_BUNDLE", Tier: "legacy_env", Target: "ca_bundle"},
	}
	hits := make([]EnvHit, 0, len(definitions))
	for _, definition := range definitions {
		if value := strings.TrimSpace(os.Getenv(definition.Key)); value != "" {
			definition.Value = value
			hits = append(hits, definition)
		}
	}
	return hits
}
