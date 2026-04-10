package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	appName          = "awiki-cli"
	legacySkillName  = "awiki-agent-id-message"
	defaultDomain    = "awiki.ai"
	defaultService   = "https://" + defaultDomain
	defaultDIDDomain = defaultDomain
)

type Overrides struct {
	Identity        string
	IdentityChanged bool
	Format          string
	FormatChanged   bool
}

// Paths describes all filesystem locations used by awiki-cli.
// After the config/workdir refactor, everything is derived from a single
// root directory:
//
//	rootDir/config.json
//	rootDir/db/awiki-cli.db
//	rootDir/identities/...
//	rootDir/logs/...
//	rootDir/cache/...
//	rootDir/tmp/...
//
// Legacy v1 artefacts still live under ~/.openclaw/... and are exposed via
// LegacyCredentialsDir / LegacyDataDir for import/doctor commands.
type Paths struct {
	RootDir              string `json:"root_dir"`
	ConfigDir            string `json:"config_dir"`
	DataDir              string `json:"data_dir"`
	StateDir             string `json:"state_dir"`
	CacheDir             string `json:"cache_dir"`
	ConfigFile           string `json:"config_file"`
	IdentityDir          string `json:"identity_dir"`
	DatabaseFile         string `json:"database_file"`
	LogsDir              string `json:"logs_dir"`
	LegacyCredentialsDir string `json:"legacy_credentials_dir"`
	LegacyDataDir        string `json:"legacy_data_dir"`
}

// HomePointer is a small JSON file stored under the default workdir root
// (usually $HOME/.awiki-cli). It records the "real" workdir root when the
// user has chosen a custom directory via an explicit init flow.
//
// When present, and when AWIKI_HOME is not set, Resolve() will prefer the
// pointer target over the built-in default root.
type HomePointer struct {
	RootDir string `json:"root_dir"`
}

// FileConfig mirrors the on-disk JSON config structure under <AWIKI_HOME>/config.json.
type FileConfig struct {
	Services struct {
		Domain string `json:"domain"`
	} `json:"services"`
	Identity struct {
		Active string `json:"active"`
	} `json:"identity"`
	Runtime struct {
		Mode string `json:"mode"`
	} `json:"runtime"`
	Output struct {
		Format  string `json:"format"`
		NoColor *bool  `json:"no_color"`
	} `json:"output"`
	Update struct {
		DisableStrictVersion    bool `json:"disable_strict_version"`
		MetadataCacheTTLSeconds int  `json:"metadata_cache_ttl_seconds"`
	} `json:"update"`
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
	Paths               Paths  `json:"paths"`
	ActiveIdentity      string `json:"active_identity,omitempty"`
	RuntimeMode         string `json:"runtime_mode"`
	RuntimeSocketPath   string `json:"runtime_socket_path,omitempty"`
	OutputFormat        string `json:"output_format"`
	NoColor             bool   `json:"no_color"`
	UserServiceURL      string `json:"user_service_url"`
	MessageServiceURL   string `json:"message_service_url"`
	MessageServiceWSURL string `json:"message_service_ws_url,omitempty"`
	DIDDomain           string `json:"did_domain"`
	CABundle            string `json:"ca_bundle,omitempty"`

	UpdateDisableStrictVersion    bool `json:"update_disable_strict_version"`
	UpdateMetadataCacheTTLSeconds int  `json:"update_metadata_cache_ttl_seconds"`

	ConfigExists bool                   `json:"config_exists"`
	ConfigError  string                 `json:"config_error,omitempty"`
	EnvHits      []EnvHit               `json:"env_hits,omitempty"`
	Sources      map[string]ValueSource `json:"sources"`
}

func Resolve(overrides Overrides) (*Resolved, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}

	rootDir, rootSource := resolveRootDir(home)
	paths, err := buildPaths(home, rootDir)
	if err != nil {
		return nil, err
	}

	resolved := &Resolved{
		Paths:             paths,
		RuntimeMode:       "http",
		OutputFormat:      "json",
		UserServiceURL:    defaultService,
		MessageServiceURL: defaultService,
		DIDDomain:         defaultDIDDomain,
		CABundle:          "",
		Sources: map[string]ValueSource{
			"root_dir": rootSource,
		},
	}
	resolved.EnvHits = collectEnvHits()

	fileConfig, configExists, configError := loadFileConfig(paths.ConfigFile)
	resolved.ConfigExists = configExists
	if configError != nil {
		resolved.ConfigError = configError.Error()
	}

	// Identity
	resolved.ActiveIdentity, resolved.Sources["active_identity"] = resolveString(
		overrides.Identity,
		overrides.IdentityChanged,
		"AWIKI_IDENTITY",
		fileConfig.Identity.Active,
		"",
	)

	// Runtime mode
	resolved.RuntimeMode, resolved.Sources["runtime_mode"] = resolveString(
		"",
		false,
		"AWIKI_RUNTIME_MODE",
		fileConfig.Runtime.Mode,
		"http",
	)

	// Runtime socket path: derived from state dir, not user-configurable for now.
	defaultSocket := filepath.Join(paths.StateDir, "runtime", "message-daemon.sock")
	resolved.RuntimeSocketPath = defaultSocket
	resolved.Sources["runtime_socket_path"] = ValueSource{
		Source: "derived",
		Value:  defaultSocket,
	}

	// Output format
	resolved.OutputFormat, resolved.Sources["output_format"] = resolveString(
		overrides.Format,
		overrides.FormatChanged,
		"AWIKI_FORMAT",
		fileConfig.Output.Format,
		"json",
	)
	if strings.TrimSpace(resolved.OutputFormat) == "" {
		resolved.OutputFormat = "json"
	}

	// NoColor bool with env > config > default precedence.
	resolved.NoColor, resolved.Sources["no_color"] = resolveBool(
		"AWIKI_NO_COLOR",
		fileConfig.Output.NoColor,
		false,
	)

	// Services domain and derived URLs.
	domain, domainSource := resolveString(
		"",
		false,
		"",
		fileConfig.Services.Domain,
		defaultDomain,
	)
	domain = strings.TrimSpace(domain)
	resolved.Sources["services_domain"] = domainSource

	resolved.DIDDomain = domain
	resolved.Sources["did_domain"] = ValueSource{
		Source: domainSource.Source,
		Value:  domain,
	}

	resolved.UserServiceURL = "https://" + domain
	resolved.Sources["user_service_url"] = ValueSource{
		Source: "derived",
		Value:  resolved.UserServiceURL,
	}

	resolved.MessageServiceURL = "https://" + domain + "/message-service"
	resolved.Sources["message_service_url"] = ValueSource{
		Source: "derived",
		Value:  resolved.MessageServiceURL,
	}

	resolved.MessageServiceWSURL = "wss://" + domain + "/message-service/ws"
	resolved.Sources["message_service_ws_url"] = ValueSource{
		Source: "derived",
		Value:  resolved.MessageServiceWSURL,
	}

	// Update-related knobs (no env overrides for now).
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

	return resolved, nil
}

func Snapshot(resolved *Resolved) map[string]any {
	if resolved == nil {
		return map[string]any{}
	}
	return map[string]any{
		"paths":                             resolved.Paths,
		"active_identity":                   resolved.ActiveIdentity,
		"runtime_mode":                      resolved.RuntimeMode,
		"runtime_socket_path":               resolved.RuntimeSocketPath,
		"output_format":                     resolved.OutputFormat,
		"no_color":                          resolved.NoColor,
		"user_service_url":                  resolved.UserServiceURL,
		"message_service_url":               resolved.MessageServiceURL,
		"message_service_ws_url":            resolved.MessageServiceWSURL,
		"did_domain":                        resolved.DIDDomain,
		"ca_bundle":                         resolved.CABundle,
		"update_disable_strict_version":     resolved.UpdateDisableStrictVersion,
		"update_metadata_cache_ttl_seconds": resolved.UpdateMetadataCacheTTLSeconds,
		"config_exists":                     resolved.ConfigExists,
		"config_error":                      resolved.ConfigError,
		"env_hits":                          resolved.EnvHits,
		"sources":                           resolved.Sources,
	}
}

// DefaultRootDir returns the built-in default workdir root for the current
// platform without considering AWIKI_HOME or any pointer files.
func DefaultRootDir(home string) string {
	if runtime.GOOS == "windows" {
		base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "AwikiCli")
	}
	return filepath.Join(home, "."+appName)
}

// LoadHomePointer, if present and valid, returns the target root directory
// recorded in the default workdir's home.json pointer file.
//
// The pointer file is intentionally tiny and human-inspectable. The current
// format is:
//
//	{ "root_dir": "/custom/path" }
//
// If the file does not exist, is empty, or cannot be parsed, LoadHomePointer
// returns an empty string and a nil error.
func LoadHomePointer(home string) (string, error) {
	defaultRoot := DefaultRootDir(home)
	pointerPath := filepath.Join(defaultRoot, "home.json")
	raw, err := os.ReadFile(pointerPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", nil
	}

	// Prefer JSON, but fall back to treating the file as a plain path for
	// robustness if someone hand-edits it.
	var pointer HomePointer
	if err := json.Unmarshal([]byte(text), &pointer); err == nil {
		if strings.TrimSpace(pointer.RootDir) != "" {
			return pointer.RootDir, nil
		}
		// Invalid or empty JSON payload is treated as "no pointer".
		return "", nil
	}
	// Not valid JSON – treat the raw content as the path.
	return text, nil
}

// WriteHomePointer writes or updates the home.json pointer file under the
// default workdir root to record the chosen root directory. It ensures the
// default root exists with 0700 permissions and writes the pointer file
// with 0600 permissions.
func WriteHomePointer(home, targetRoot string) error {
	defaultRoot := DefaultRootDir(home)
	if err := os.MkdirAll(defaultRoot, 0o700); err != nil {
		return fmt.Errorf("create default workdir root %s: %w", defaultRoot, err)
	}
	pointer := HomePointer{RootDir: targetRoot}
	raw, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal home pointer: %w", err)
	}
	pointerPath := filepath.Join(defaultRoot, "home.json")
	if err := os.WriteFile(pointerPath, raw, 0o600); err != nil {
		return fmt.Errorf("write home pointer: %w", err)
	}
	return nil
}

func resolveRootDir(home string) (string, ValueSource) {
	// 1. Explicit AWIKI_HOME override (advanced / CI use).
	raw := strings.TrimSpace(os.Getenv("AWIKI_HOME"))
	if raw != "" {
		root := ExpandHome(home, raw)
		return root, ValueSource{
			Source: "canonical_env",
			Key:    "AWIKI_HOME",
			Value:  root,
		}
	}

	// 2. Pointer file under the default root (set by awiki-cli init --home).
	if pointer, err := LoadHomePointer(home); err == nil {
		if trimmed := strings.TrimSpace(pointer); trimmed != "" {
			root := ExpandHome(home, trimmed)
			return root, ValueSource{
				Source: "home_pointer",
				Key:    "home.json",
				Value:  root,
			}
		}
	}

	// 3. Fallback to the built-in default root.
	root := DefaultRootDir(home)
	return root, ValueSource{
		Source: "default",
		Value:  root,
	}
}

func buildPaths(home, root string) (Paths, error) {
	// 尽早为用户创建工作目录根（AWIKI_HOME），便于发现 config.json 等文件。
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Paths{}, fmt.Errorf("create workdir root %s: %w", root, err)
	}

	configDir := root
	dataDir := filepath.Join(root, "db")
	stateDir := filepath.Join(root, "tmp")
	cacheDir := filepath.Join(root, "cache")
	identityDir := filepath.Join(root, "identities")
	logsDir := filepath.Join(root, "logs")

	legacyCredentialsDir := filepath.Join(home, ".openclaw", "credentials", legacySkillName)
	legacyDataDir := filepath.Join(home, ".openclaw", "workspace", "data", legacySkillName)

	return Paths{
		RootDir:              root,
		ConfigDir:            configDir,
		DataDir:              dataDir,
		StateDir:             stateDir,
		CacheDir:             cacheDir,
		ConfigFile:           filepath.Join(configDir, "config.json"),
		IdentityDir:          identityDir,
		DatabaseFile:         filepath.Join(dataDir, appName+".db"),
		LogsDir:              logsDir,
		LegacyCredentialsDir: legacyCredentialsDir,
		LegacyDataDir:        legacyDataDir,
	}, nil
}

func loadFileConfig(path string) (FileConfig, bool, error) {
	var config FileConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config, false, nil
		}
		return config, false, err
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, true, err
	}
	return config, true, nil
}

func resolveString(flagValue string, flagChanged bool, envKey string, fileValue string, defaultValue string) (string, ValueSource) {
	if flagChanged && strings.TrimSpace(flagValue) != "" {
		value := strings.TrimSpace(flagValue)
		return value, ValueSource{Source: "flag", Value: value}
	}
	if strings.TrimSpace(envKey) != "" {
		if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
			return value, ValueSource{Source: "canonical_env", Key: envKey, Value: value}
		}
	}
	if strings.TrimSpace(fileValue) != "" {
		value := strings.TrimSpace(fileValue)
		return value, ValueSource{Source: "config_file", Value: value}
	}
	return defaultValue, ValueSource{Source: "default", Value: defaultValue}
}

func resolveBool(envKey string, fileValue *bool, defaultValue bool) (bool, ValueSource) {
	if strings.TrimSpace(envKey) != "" {
		if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
			parsed := strings.EqualFold(value, "1") ||
				strings.EqualFold(value, "true") ||
				strings.EqualFold(value, "yes")
			return parsed, ValueSource{Source: "canonical_env", Key: envKey, Value: value}
		}
	}
	if fileValue != nil {
		return *fileValue, ValueSource{Source: "config_file", Value: fmt.Sprintf("%t", *fileValue)}
	}
	return defaultValue, ValueSource{Source: "default", Value: fmt.Sprintf("%t", defaultValue)}
}

// ExpandHome resolves "~/..." style paths against the provided home directory.
func ExpandHome(home string, value string) string {
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}

func collectEnvHits() []EnvHit {
	definitions := []EnvHit{
		{Key: "AWIKI_HOME", Tier: "canonical_env", Target: "root_dir"},
		{Key: "AWIKI_IDENTITY", Tier: "canonical_env", Target: "active_identity"},
		{Key: "AWIKI_RUNTIME_MODE", Tier: "canonical_env", Target: "runtime_mode"},
		{Key: "AWIKI_FORMAT", Tier: "canonical_env", Target: "output_format"},
		{Key: "AWIKI_NO_COLOR", Tier: "canonical_env", Target: "no_color"},
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
