package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
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
		anpServiceCheck(resolved),
		runtimeCheck(resolved),
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

func runtimeCheck(resolved *config.Resolved) Check {
	status := "ok"
	summary := "Runtime mode resolved"
	if strings.TrimSpace(resolved.RuntimeMode) == "websocket" {
		summary = "Runtime mode is websocket"
	} else {
		summary = "Runtime mode is http"
	}
	return Check{
		Name:    "runtime",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"mode":        resolved.RuntimeMode,
			"socket_path": resolved.RuntimeSocketPath,
		},
	}
}

func anpServiceCheck(resolved *config.Resolved) Check {
	status := "ok"
	summary := "ANP service discovery fields are ready for DID generation"
	details := map[string]any{
		"anp_service_endpoint": resolved.ANPServiceEndpoint,
		"anp_service_did":      resolved.ANPServiceDID,
	}
	if err := identity.ValidateANPServiceEndpoint(resolved.ANPServiceEndpoint); err != nil {
		status = "error"
		summary = "ANP service endpoint is invalid for public DID discovery"
		details["endpoint_error"] = err.Error()
	}
	if err := identity.ValidateANPServiceDID(resolved.ANPServiceDID); err != nil {
		if status != "error" {
			status = "error"
			summary = "ANP service DID is invalid for public DID discovery"
		}
		details["service_did_error"] = err.Error()
	}
	return Check{
		Name:    "anp_service",
		Status:  status,
		Summary: summary,
		Details: details,
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
	} else if current != nil && !current.UserState.ReadyForMessaging {
		status = "warn"
		summary = "Default identity is local-only and cannot be used for messaging yet"
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
			"user_state":       defaultIdentityUserState(current),
			"index_error":      errorText(indexErr),
		},
	}
}

func defaultIdentityUserState(current *identity.IdentitySummary) any {
	if current == nil {
		return nil
	}
	return current.UserState
}

func sqliteCheck(resolved *config.Resolved) Check {
	databaseExists := pathExists(resolved.Paths.DatabaseFile)
	status := "info"
	summary := "SQLite target path resolved"
	if databaseExists {
		status = "ok"
		summary = "SQLite database file already exists"
	}
	schemaVersion := 0
	schemaError := ""
	if databaseExists {
		db, err := store.OpenReadOnly(resolved.Paths.DatabaseFile)
		if err != nil {
			status = "error"
			summary = "SQLite database file exists but cannot be opened"
			schemaError = err.Error()
		} else {
			defer db.Close()
			version, err := store.CurrentSchemaVersion(db)
			if err != nil {
				status = "error"
				summary = "SQLite database is readable but schema version could not be inspected"
				schemaError = err.Error()
			} else {
				schemaVersion = version
				if version != store.SchemaVersion {
					status = "warn"
					summary = "SQLite database exists but schema version is not current"
				}
			}
		}
	}
	return Check{
		Name:    "sqlite",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"database_file":         resolved.Paths.DatabaseFile,
			"exists":                databaseExists,
			"parent_dir":            filepath.Dir(resolved.Paths.DatabaseFile),
			"schema_version":        schemaVersion,
			"target_schema_version": store.SchemaVersion,
			"schema_error":          schemaError,
		},
	}
}

func legacyCheck(resolved *config.Resolved) Check {
	manager := identity.NewManager(resolved.Paths)
	scan, scanErr := manager.ScanLegacy()
	credentialsExists := pathExists(resolved.Paths.LegacyCredentialsDir)
	dataExists := pathExists(resolved.Paths.LegacyDataDir)
	legacyDB, dbErr := store.ScanLegacyDatabase(context.Background(), resolved.Paths)
	status := "info"
	summary := "No legacy v1 paths detected"
	if scanErr != nil {
		status = "error"
		summary = "Legacy credential scan failed"
	} else if scan != nil && scan.HasLegacy {
		status = "warn"
		summary = "Legacy awiki-agent-id-message credential layout detected"
	} else if (legacyDB != nil && legacyDB.Exists) || credentialsExists || dataExists {
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
			"legacy_database":        legacyDB,
			"legacy_database_error":  errorText(dbErr),
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
