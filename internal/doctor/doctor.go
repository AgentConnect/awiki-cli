package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

type Check struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Summary string         `json:"summary"`
	Details map[string]any `json:"details,omitempty"`
}

type Report struct {
	Checks  []Check `json:"checks"`
	Summary string  `json:"summary"`
	Counts  Counts  `json:"counts"`
}

type Counts struct {
	OK    int `json:"ok"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
	Info  int `json:"info"`
}

func Run(resolved *config.Resolved) Report {
	checks := []Check{
		buildCheck(resolved),
		configFileCheck(resolved),
		envCheck(resolved),
		identityStoreCheck(resolved),
		sqliteCheck(resolved),
		legacyCheck(resolved),
	}
	counts := Counts{}
	for _, check := range checks {
		switch check.Status {
		case "ok":
			counts.OK++
		case "warn":
			counts.Warn++
		case "error":
			counts.Error++
		default:
			counts.Info++
		}
	}
	summary := "Doctor completed successfully"
	if counts.Error > 0 {
		summary = "Doctor found blocking issues"
	} else if counts.Warn > 0 {
		summary = "Doctor found warnings"
	}
	return Report{Checks: checks, Summary: summary, Counts: counts}
}

func buildCheck(resolved *config.Resolved) Check {
	info := buildinfo.Current()
	status := "ok"
	summary := "Pure-Go build target is aligned"
	if strings.EqualFold(info.CGOEnabled, "1") || strings.EqualFold(info.CGOEnabled, "true") {
		status = "warn"
		summary = "Build metadata indicates CGO was enabled"
	}
	return Check{
		Name:    "build",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"go_version":  info.GoVersion,
			"goos":        info.GOOS,
			"goarch":      info.GOARCH,
			"compiler":    info.Compiler,
			"cgo_enabled": info.CGOEnabled,
		},
	}
}

func configFileCheck(resolved *config.Resolved) Check {
	status := "warn"
	summary := "No config file found yet"
	if resolved.ConfigExists {
		status = "ok"
		summary = "Config file loaded"
	}
	if resolved.ConfigError != "" {
		status = "error"
		summary = "Config file exists but failed to parse"
	}
	return Check{
		Name:    "config_file",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"path":   resolved.Paths.ConfigFile,
			"exists": resolved.ConfigExists,
			"error":  resolved.ConfigError,
		},
	}
}

func envCheck(resolved *config.Resolved) Check {
	status := "info"
	summary := "No environment overrides detected"
	aliasHits := 0
	legacyHits := 0
	for _, hit := range resolved.EnvHits {
		if hit.Tier == "draft_alias_env" {
			aliasHits++
		}
		if hit.Tier == "legacy_env" {
			legacyHits++
		}
	}
	if len(resolved.EnvHits) > 0 {
		status = "ok"
		summary = "Environment overrides detected"
	}
	if aliasHits > 0 || legacyHits > 0 {
		status = "warn"
		summary = "Compatibility environment variables are in use"
	}
	return Check{
		Name:    "environment",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"hits": resolved.EnvHits,
		},
	}
}

func identityStoreCheck(resolved *config.Resolved) Check {
	manager := identity.NewManager(resolved.Paths)
	indexPath := filepath.Join(resolved.Paths.IdentityDir, identity.IndexFileName)
	identityDirExists := pathExists(resolved.Paths.IdentityDir)
	indexExists := pathExists(indexPath)
	status := "warn"
	summary := "Identity store has not been initialized"
	if identityDirExists || indexExists {
		status = "ok"
		summary = "Identity store path resolved"
	}
	index, indexErr := manager.LoadIndex()
	if indexErr != nil {
		status = "error"
		summary = "Identity index exists but failed to parse"
	}
	current, currentErr := manager.Current()
	if currentErr != nil && !errors.Is(currentErr, identity.ErrNoDefaultIdentity) && len(index.Credentials) > 0 {
		status = "error"
		summary = "Identity index is missing a valid default identity"
	}
	return Check{
		Name:    "identity_store",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"identity_dir":     resolved.Paths.IdentityDir,
			"dir_exists":       identityDirExists,
			"index_path":       indexPath,
			"index_exists":     indexExists,
			"index_entries":    len(index.Credentials),
			"default_identity": current,
			"index_error":      errorText(indexErr),
		},
	}
}

func sqliteCheck(resolved *config.Resolved) Check {
	databaseExists := pathExists(resolved.Paths.DatabaseFile)
	status := "info"
	summary := "SQLite target path resolved"
	if databaseExists {
		status = "ok"
		summary = "SQLite database file already exists"
	}
	return Check{
		Name:    "sqlite",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"database_file": resolved.Paths.DatabaseFile,
			"exists":        databaseExists,
			"parent_dir":    filepath.Dir(resolved.Paths.DatabaseFile),
		},
	}
}

func legacyCheck(resolved *config.Resolved) Check {
	manager := identity.NewManager(resolved.Paths)
	scan, scanErr := manager.ScanLegacy()
	credentialsExists := pathExists(resolved.Paths.LegacyCredentialsDir)
	dataExists := pathExists(resolved.Paths.LegacyDataDir)
	status := "info"
	summary := "No legacy v1 paths detected"
	if scanErr != nil {
		status = "error"
		summary = "Legacy credential scan failed"
	} else if scan != nil && scan.HasLegacy {
		status = "warn"
		summary = "Legacy awiki-agent-id-message credential layout detected"
	} else if credentialsExists || dataExists {
		status = "warn"
		summary = "Legacy awiki-agent-id-message paths detected"
	}
	return Check{
		Name:    "legacy_paths",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"legacy_credentials_dir": resolved.Paths.LegacyCredentialsDir,
			"credentials_exists":     credentialsExists,
			"legacy_data_dir":        resolved.Paths.LegacyDataDir,
			"data_exists":            dataExists,
			"legacy_scan":            scan,
			"scan_error":             errorText(scanErr),
		},
	}
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
