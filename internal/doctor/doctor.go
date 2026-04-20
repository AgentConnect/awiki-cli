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
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	listenerrt "github.com/agentconnect/awiki-cli/internal/runtime/listener"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/upgrade"
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
		upgradeStateCheck(resolved),
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

func upgradeStateCheck(resolved *config.Resolved) Check {
	inspection, err := upgrade.Inspect(context.Background(), resolved, buildinfo.Version)
	if err != nil {
		paths := upgrade.ResolvePaths(resolved)
		return Check{
			Name:    "workspace_upgrade",
			Status:  "error",
			Summary: "Workspace upgrade state inspection failed",
			Details: map[string]any{
				"meta_path":    paths.MetaPath,
				"journal_path": paths.JournalPath,
				"error":        err.Error(),
			},
		}
	}
	status := "ok"
	summary := "Workspace upgrade metadata is up to date"
	if inspection.Journal != nil {
		status = "warn"
		summary = "Workspace upgrade journal indicates an interrupted upgrade"
	} else if inspection.Meta != nil && len(inspection.Meta.Warnings) > 0 {
		status = "warn"
		summary = "Workspace upgrade completed with migration warnings"
	} else if inspection.Detection.CurrentVersion < inspection.Detection.LatestVersion {
		status = "warn"
		summary = "Workspace data still needs to be upgraded"
	} else if inspection.Detection.CurrentVersionSource == "legacy_detector" {
		status = "warn"
		summary = "Workspace upgrade metadata has not been initialized yet"
	}
	return Check{
		Name:    "workspace_upgrade",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"meta":      inspection.Meta,
			"journal":   inspection.Journal,
			"detection": inspection.Detection,
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
	if resolved.ConfigExists && resolved.ConfigSchemaVersion < config.ConfigSchemaVersion {
		status = "warn"
		summary = "Config file exists but schema version is not current"
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
			"path":           resolved.Paths.ConfigFile,
			"exists":         resolved.ConfigExists,
			"schema_version": resolved.ConfigSchemaVersion,
			"error":          resolved.ConfigError,
		},
	}
}

func envCheck(resolved *config.Resolved) Check {
	status := "info"
	summary := "No workspace environment override detected"
	if len(resolved.EnvHits) > 0 {
		status = "ok"
		summary = "Workspace environment override detected"
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
	listenerStatus, listenerErr := listenerrt.StatusFor(resolved)
	runtimeResolved := runtimecfg.Resolve(resolved)
	if runtimeResolved.Mode == runtimecfg.ModeWebSocket {
		summary = "Runtime mode is websocket"
		if !runtimeResolved.Listener.Enabled {
			status = "warn"
			summary = "Runtime mode is websocket but listener is disabled"
		} else if listenerErr != nil {
			status = "warn"
			summary = "Runtime mode is websocket but listener status is unavailable"
		} else if !listenerStatus.Running {
			status = "warn"
			summary = "Runtime mode is websocket but listener service is not running"
		}
	} else {
		summary = "Runtime mode is http"
	}
	details := map[string]any{
		"mode":                  resolved.RuntimeMode,
		"socket_path":           resolved.RuntimeSocketPath,
		"listener_enabled":      resolved.RuntimeListenerEnabled,
		"listener_auto_install": resolved.RuntimeListenerAutoInstall,
		"listener_auto_start":   resolved.RuntimeListenerAutoStart,
	}
	if listenerErr != nil {
		details["listener_status_error"] = listenerErr.Error()
	} else {
		details["listener_status"] = listenerStatus
	}
	return Check{
		Name:    "runtime",
		Status:  status,
		Summary: summary,
		Details: details,
	}
}

func anpServiceCheck(resolved *config.Resolved) Check {
	status := "ok"
	summary := "ANP service discovery fields are ready for DID generation"
	details := map[string]any{
		"service_base_url":     resolved.ServiceBaseURL,
		"did_domain":           resolved.DIDDomain,
		"anp_service_endpoint": resolved.ANPServiceEndpoint,
		"anp_service_did":      resolved.ANPServiceDID,
	}
	services := config.ServicesConfig{
		ServiceBaseURL:     resolved.ServiceBaseURL,
		DIDDomain:          resolved.DIDDomain,
		ANPServiceEndpoint: resolved.ANPServiceEndpoint,
		ANPServiceDID:      resolved.ANPServiceDID,
	}
	diagnostics := config.ValidateServices(services)
	details["diagnostics"] = diagnostics
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case config.ServiceSeverityError:
			status = "error"
			summary = "ANP service discovery fields have blocking issues"
		case config.ServiceSeverityWarn:
			if status != "error" {
				status = "warn"
				summary = "ANP service discovery fields need review"
			}
		}
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
	identities, listErr := manager.List()
	legacyK1DIDs := make([]string, 0)
	if listErr == nil {
		for _, summaryItem := range identities {
			if identity.IsK1DID(summaryItem.DID) {
				legacyK1DIDs = append(legacyK1DIDs, summaryItem.DID)
			}
		}
	}
	if currentErr != nil && !errors.Is(currentErr, identity.ErrNoDefaultIdentity) && len(index.Credentials) > 0 {
		status = "error"
		summary = "Identity index is missing a valid default identity"
	} else if len(legacyK1DIDs) > 0 {
		status = "warn"
		summary = "Identity store still contains legacy k1 DID material"
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
			"list_error":       errorText(listErr),
			"legacy_k1_dids":   legacyK1DIDs,
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
	handleBindingsExists := false
	handleBindingsCount := 0
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
			var bindingTableCount int
			if rowsErr := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'contact_handle_bindings'`).Scan(&bindingTableCount); rowsErr == nil {
				handleBindingsExists = bindingTableCount > 0
				if handleBindingsExists {
					_ = db.QueryRow(`SELECT COUNT(*) FROM contact_handle_bindings`).Scan(&handleBindingsCount)
				}
			}
		}
	}
	return Check{
		Name:    "sqlite",
		Status:  status,
		Summary: summary,
		Details: map[string]any{
			"database_file":                  resolved.Paths.DatabaseFile,
			"exists":                         databaseExists,
			"parent_dir":                     filepath.Dir(resolved.Paths.DatabaseFile),
			"schema_version":                 schemaVersion,
			"target_schema_version":          store.SchemaVersion,
			"contact_handle_bindings_exists": handleBindingsExists,
			"contact_handle_bindings_count":  handleBindingsCount,
			"schema_error":                   schemaError,
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
