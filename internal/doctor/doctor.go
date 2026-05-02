package doctor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/message"
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
		anpMLSCheck(resolved),
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

func anpMLSCheck(resolved *config.Resolved) Check {
	provider := message.NewDefaultMLSExecProvider(resolved)
	binary, resolveErr := provider.ResolveBinaryPath()
	state := inspectMLSState(resolved, provider.DataDir)
	details := map[string]any{
		"binary":            binary,
		"data_dir":          provider.DataDir,
		"env_override":      message.ANPMLSBinaryEnv,
		"plain_unaffected":  true,
		"resolve_error":     errorString(resolveErr),
		"remediation":       anpMLSRemediation(resolveErr, nil, nil, state),
		"data_dir_status":   state.DataDirStatus,
		"data_dir_exists":   state.DataDirExists,
		"data_dir_error":    state.DataDirError,
		"state_db":          state.StateDBPath,
		"state_db_status":   state.StateDBStatus,
		"state_db_error":    state.StateDBError,
		"state_lock":        state.StateLockPath,
		"state_lock_status": state.StateLockStatus,
		"state_lock_error":  state.StateLockError,
		"e2ee_group_count":  state.E2EEGroupCount,
	}
	status := "ok"
	summary := "anp-mls binary and compatibility probe are ready for group E2EE operations"
	if resolveErr != nil {
		status = "info"
		summary = "anp-mls binary not found; plain messaging is unaffected, but group E2EE commands will fail"
		details["remediation"] = anpMLSRemediation(resolveErr, nil, nil, state)
		return Check{Name: "anp_mls", Status: status, Summary: summary, Details: details}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	versionInfo, probeErr := provider.ProbeVersion(ctx)
	details["version"] = versionInfo
	details["probe_error"] = errorString(probeErr)
	compatErr := anpMLSCompatibilityError(versionInfo)
	details["compatibility_error"] = errorString(compatErr)
	details["remediation"] = anpMLSRemediation(resolveErr, probeErr, compatErr, state)
	if probeErr != nil {
		status = "warn"
		summary = "anp-mls binary is present but the version compatibility probe failed"
	} else if compatErr != nil {
		status = "warn"
		summary = "anp-mls binary version is not compatible with this awiki-cli build"
	} else if state.HasWarning() {
		status = "warn"
		summary = "anp-mls binary is compatible but MLS state needs attention"
	}
	return Check{Name: "anp_mls", Status: status, Summary: summary, Details: details}
}

type mlsStateInspection struct {
	DataDirExists   bool
	DataDirStatus   string
	DataDirError    string
	StateDBPath     string
	StateDBStatus   string
	StateDBError    string
	StateLockPath   string
	StateLockStatus string
	StateLockError  string
	E2EEGroupCount  int
}

func (s mlsStateInspection) HasWarning() bool {
	return strings.HasPrefix(s.DataDirStatus, "warn") || strings.HasPrefix(s.StateDBStatus, "warn") || strings.HasPrefix(s.StateLockStatus, "warn")
}

func inspectMLSState(resolved *config.Resolved, dataDir string) mlsStateInspection {
	state := mlsStateInspection{
		DataDirStatus:   "missing",
		StateDBPath:     filepath.Join(dataDir, "state.db"),
		StateDBStatus:   "missing",
		StateLockPath:   filepath.Join(dataDir, "state.lock"),
		StateLockStatus: "missing",
	}
	if strings.TrimSpace(dataDir) == "" {
		state.DataDirStatus = "not_configured"
		state.StateDBPath = ""
		state.StateLockPath = ""
		return state
	}
	state.E2EEGroupCount = cachedGroupE2EECount(resolved)
	if info, err := os.Stat(dataDir); err != nil {
		if os.IsNotExist(err) {
			if state.E2EEGroupCount > 0 {
				state.DataDirStatus = "warn_missing_with_cached_groups"
			} else {
				state.DataDirStatus = "missing"
			}
		} else {
			state.DataDirStatus = "warn_stat_failed"
			state.DataDirError = err.Error()
		}
		return state
	} else if !info.IsDir() {
		state.DataDirStatus = "warn_not_directory"
		return state
	}
	state.DataDirExists = true
	state.DataDirStatus = "ok"
	if err := canReadDir(dataDir); err != nil {
		state.DataDirStatus = "warn_not_readable"
		state.DataDirError = err.Error()
	} else if err := canWriteDir(dataDir); err != nil {
		state.DataDirStatus = "warn_not_writable"
		state.DataDirError = err.Error()
	}

	if info, err := os.Stat(state.StateDBPath); err != nil {
		if os.IsNotExist(err) {
			if state.E2EEGroupCount > 0 {
				state.StateDBStatus = "warn_missing_with_cached_groups"
			} else {
				state.StateDBStatus = "missing"
			}
		} else {
			state.StateDBStatus = "warn_stat_failed"
			state.StateDBError = err.Error()
		}
	} else if info.IsDir() {
		state.StateDBStatus = "warn_not_file"
	} else if err := canReadFile(state.StateDBPath); err != nil {
		state.StateDBStatus = "warn_not_readable"
		state.StateDBError = err.Error()
	} else {
		state.StateDBStatus = "ok"
	}

	if info, err := os.Stat(state.StateLockPath); err != nil {
		if os.IsNotExist(err) {
			state.StateLockStatus = "missing"
		} else {
			state.StateLockStatus = "warn_stat_failed"
			state.StateLockError = err.Error()
		}
	} else if info.IsDir() {
		state.StateLockStatus = "warn_not_file"
	} else if err := canReadFile(state.StateLockPath); err != nil {
		state.StateLockStatus = "warn_not_readable"
		state.StateLockError = err.Error()
	} else if time.Since(info.ModTime()) > 15*time.Minute {
		state.StateLockStatus = "warn_stale_candidate"
	} else {
		state.StateLockStatus = "present_active_or_recent"
	}
	return state
}

func canReadDir(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	_ = entries
	return nil
}

func canWriteDir(path string) error {
	probe, err := os.CreateTemp(path, ".awiki-cli-doctor-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func canReadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func cachedGroupE2EECount(resolved *config.Resolved) int {
	if resolved == nil || strings.TrimSpace(resolved.Paths.DatabaseFile) == "" || !pathExists(resolved.Paths.DatabaseFile) {
		return 0
	}
	db, err := store.OpenReadOnly(resolved.Paths.DatabaseFile)
	if err != nil {
		return 0
	}
	defer db.Close()
	var tableCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'groups'`).Scan(&tableCount); err != nil || tableCount == 0 {
		return 0
	}
	var count sql.NullInt64
	if err := db.QueryRow(`SELECT COUNT(*) FROM groups WHERE metadata LIKE ?`, "%group-e2ee%").Scan(&count); err != nil {
		return 0
	}
	if !count.Valid {
		return 0
	}
	return int(count.Int64)
}

func anpMLSCompatibilityError(info *message.MLSVersionInfo) error {
	if info == nil {
		return fmt.Errorf("missing version info")
	}
	if info.APIVersion != "anp-mls/v1" {
		return fmt.Errorf("api_version %q is not supported; want anp-mls/v1", info.APIVersion)
	}
	if info.BinaryName != "anp-mls" {
		return fmt.Errorf("binary_name %q is not supported; want anp-mls", info.BinaryName)
	}
	if !containsString(info.SupportedCommands, "system version") {
		return fmt.Errorf("supported_commands does not include system version")
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func anpMLSRemediation(resolveErr error, probeErr error, compatErr error, state mlsStateInspection) string {
	switch {
	case resolveErr != nil:
		return "Build anp-mls from ../anp/anp/rust, put it next to release artifacts or on PATH, or set AWIKI_ANP_MLS_BINARY to the absolute binary path. Plain messaging does not require anp-mls."
	case probeErr != nil:
		return "Install a current anp-mls build that supports `anp-mls system version --json-in -`; rebuild from ../anp/anp/rust if this probe fails."
	case compatErr != nil:
		return "Replace anp-mls with a build that reports api_version anp-mls/v1, binary_name anp-mls, and supported command `system version`."
	case state.DataDirStatus == "warn_not_writable" || state.DataDirStatus == "warn_not_readable":
		return "Fix permissions on the MLS data directory or move the workspace with AWIKI_CLI_WORKSPACE_HOME_DIR."
	case state.StateDBStatus == "warn_missing_with_cached_groups":
		return "The business database has cached group-e2ee groups but MLS state.db is missing; restore the MLS data directory from backup before sending encrypted group messages."
	case strings.HasPrefix(state.StateLockStatus, "warn"):
		return "If no anp-mls process is running, remove stale state.lock after backing up the MLS data directory."
	default:
		return "No action required."
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
