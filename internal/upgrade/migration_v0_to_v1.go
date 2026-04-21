package upgrade

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
)

type workspaceV0ToV1Migration struct{}
type workspaceV1ToV2Migration struct{}
type workspaceV2ToV3Migration struct{}

type legacySettingsFile struct {
	UserServiceURL   string `json:"user_service_url"`
	MoltMessageURL   string `json:"molt_message_url"`
	DidDomain        string `json:"did_domain"`
	MessageTransport struct {
		ReceiveMode string `json:"receive_mode"`
	} `json:"message_transport"`
}

const (
	legacySkillInstallDirName     = "awiki-agent-id-message"
	legacyHeartbeatSectionStart   = "<!-- awiki-heartbeat-start -->"
	legacyHeartbeatSectionEnd     = "<!-- awiki-heartbeat-end -->"
	legacyMacOSListenerLabel      = "com.awiki.ws-listener"
	legacyLinuxListenerUnit       = "awiki-ws-listener.service"
	legacyWindowsListenerTaskName = "awiki-ws-listener"
)

var (
	legacyCleanupGOOS       = runtime.GOOS
	legacyCleanupUserHome   = os.UserHomeDir
	legacyCleanupRunCommand = runLegacyCleanupCommand
)

func newWorkspaceV0ToV1Migration() Migration {
	return workspaceV0ToV1Migration{}
}

func newWorkspaceV1ToV2Migration() Migration {
	return workspaceV1ToV2Migration{}
}

func newWorkspaceV2ToV3Migration() Migration {
	return workspaceV2ToV3Migration{}
}

func (workspaceV0ToV1Migration) From() int { return 0 }

func (workspaceV0ToV1Migration) To() int { return 1 }

func (workspaceV0ToV1Migration) Name() string {
	return "workspace_0_to_1_bootstrap_local_state_upgrade"
}

func (workspaceV0ToV1Migration) IsDone(ctx context.Context, uc *Context) (bool, error) {
	meta, err := LoadMeta(uc.Paths.MetaPath)
	if err != nil {
		return false, err
	}
	return meta != nil && meta.WorkspaceSchemaVersion >= 1, nil
}

func (workspaceV0ToV1Migration) Apply(ctx context.Context, uc *Context) error {
	if uc == nil || uc.Resolved == nil {
		return fmt.Errorf("workspace upgrade requires a resolved config")
	}
	detection := uc.Inspection.Detection
	var importedLegacy *identity.ImportResult
	if detection.ConfigExists {
		if err := appconfig.EnsureConfigSchemaVersion(uc.Paths.ConfigFile); err != nil {
			return err
		}
	} else if detection.LegacyConfigExists {
		legacyFileConfig, _, err := appconfig.ReadFileConfig(uc.Paths.LegacyConfigFile)
		if err != nil {
			return err
		}
		legacyFileConfig.SchemaVersion = appconfig.ConfigSchemaVersion
		if err := appconfig.WriteFileConfig(uc.Paths.ConfigFile, legacyFileConfig); err != nil {
			return err
		}
		if err := os.Remove(uc.Paths.LegacyConfigFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove legacy config: %w", err)
		}
	} else if !detection.HasWorkspace && detection.LegacySettingsExists {
		legacyConfig, err := loadLegacySettings(uc.Paths.LegacySettingsPath)
		if err != nil {
			return err
		}
		fileConfig := appconfig.FileConfig{}
		fileConfig.SchemaVersion = appconfig.ConfigSchemaVersion
		fileConfig.Runtime.Mode = legacyConfig.RuntimeMode
		fileConfig.Services.ServiceBaseURL = legacyConfig.ServiceBaseURL
		fileConfig.Services.DIDDomain = legacyConfig.DidDomain
		if err := appconfig.WriteFileConfig(uc.Paths.ConfigFile, fileConfig); err != nil {
			return err
		}
	}

	if !detection.HasWorkspace && detection.HasLegacy {
		manager := identity.NewManager(uc.Resolved.Paths)
		legacyScan, err := manager.ScanLegacy()
		if err != nil {
			return err
		}
		if legacyScan != nil && legacyScan.HasLegacy {
			importedLegacy, err = manager.ImportAllLegacy()
			if err != nil {
				return err
			}
		}

		legacyDB, err := store.ScanLegacyDatabase(ctx, uc.Resolved.Paths)
		if err != nil {
			return err
		}
		if legacyDB != nil && legacyDB.Exists {
			db, err := store.Open(uc.Resolved.Paths)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := store.EnsureSchema(ctx, db); err != nil {
				return err
			}
			if _, err := store.ImportLegacyDatabase(ctx, db, uc.Resolved.Paths, manager); err != nil {
				return err
			}
		}
	}

	if fileExists(uc.Paths.DatabaseFile) {
		if err := ensureTargetStoreSchema(ctx, uc.Resolved.Paths); err != nil {
			return err
		}
	}

	if importedLegacy != nil && len(importedLegacy.Imported) > 0 {
		refreshed, err := refreshResolvedConfig(uc.Resolved)
		if err != nil {
			return err
		}
		uc.Resolved = refreshed
		uc.Paths = ResolvePaths(refreshed)
		uc.Warnings = append(uc.Warnings, replaceImportedLegacyK1DIDs(ctx, refreshed, importedLegacy.Imported)...)
	}
	return nil
}

func (workspaceV0ToV1Migration) Validate(ctx context.Context, uc *Context) error {
	if uc == nil || uc.Resolved == nil {
		return fmt.Errorf("workspace upgrade requires a resolved config")
	}
	if fileExists(uc.Paths.ConfigFile) {
		fileConfig, _, err := appconfig.ReadFileConfig(uc.Paths.ConfigFile)
		if err != nil {
			return err
		}
		if fileConfig.SchemaVersion != appconfig.ConfigSchemaVersion {
			return fmt.Errorf("config schema version = %d, want %d", fileConfig.SchemaVersion, appconfig.ConfigSchemaVersion)
		}
	}
	if fileExists(uc.Paths.DatabaseFile) {
		db, err := store.OpenReadOnly(uc.Paths.DatabaseFile)
		if err != nil {
			return err
		}
		defer db.Close()
		version, err := store.CurrentSchemaVersion(db)
		if err != nil {
			return err
		}
		if version != store.SchemaVersion {
			return fmt.Errorf("sqlite schema version = %d, want %d", version, store.SchemaVersion)
		}
		if err := validateSQLiteHealth(ctx, db); err != nil {
			return err
		}
	}
	if uc.Inspection != nil && !uc.Inspection.Detection.HasWorkspace && uc.Inspection.Detection.LegacyIdentityExists {
		manager := identity.NewManager(uc.Resolved.Paths)
		identities, err := manager.List()
		if err != nil {
			return err
		}
		if len(identities) == 0 {
			return fmt.Errorf("expected at least one imported identity after legacy upgrade")
		}
	}
	return nil
}

func (workspaceV1ToV2Migration) From() int { return 1 }

func (workspaceV1ToV2Migration) To() int { return 2 }

func (workspaceV1ToV2Migration) Name() string {
	return "workspace_1_to_2_remove_legacy_skill_and_listener"
}

func (workspaceV1ToV2Migration) IsDone(ctx context.Context, uc *Context) (bool, error) {
	meta, err := LoadMeta(uc.Paths.MetaPath)
	if err != nil {
		return false, err
	}
	return meta != nil && meta.WorkspaceSchemaVersion >= 2, nil
}

func (workspaceV1ToV2Migration) Apply(ctx context.Context, uc *Context) error {
	if uc == nil {
		return fmt.Errorf("workspace upgrade context is required")
	}
	uc.Warnings = append(uc.Warnings, cleanupLegacySkillArtifacts(ctx)...)
	return nil
}

func (workspaceV1ToV2Migration) Validate(ctx context.Context, uc *Context) error {
	return nil
}

func (workspaceV2ToV3Migration) From() int { return 2 }

func (workspaceV2ToV3Migration) To() int { return 3 }

func (workspaceV2ToV3Migration) Name() string {
	return "workspace_2_to_3_replace_existing_k1_handle_dids"
}

func (workspaceV2ToV3Migration) IsDone(ctx context.Context, uc *Context) (bool, error) {
	meta, err := LoadMeta(uc.Paths.MetaPath)
	if err != nil {
		return false, err
	}
	return meta != nil && meta.WorkspaceSchemaVersion >= 3, nil
}

func (workspaceV2ToV3Migration) Apply(ctx context.Context, uc *Context) error {
	if uc == nil || uc.Resolved == nil {
		return fmt.Errorf("workspace upgrade requires a resolved config")
	}
	uc.Warnings = append(uc.Warnings, replaceExistingWorkspaceK1DIDs(ctx, uc.Resolved)...)
	return nil
}

func (workspaceV2ToV3Migration) Validate(ctx context.Context, uc *Context) error {
	return nil
}

type normalizedLegacySettings struct {
	ServiceBaseURL string
	DidDomain      string
	RuntimeMode    string
}

func loadLegacySettings(path string) (*normalizedLegacySettings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read legacy settings: %w", err)
	}
	var legacy legacySettingsFile
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("parse legacy settings: %w", err)
	}
	mode := runtimecfg.ModeHTTP
	if strings.EqualFold(strings.TrimSpace(legacy.MessageTransport.ReceiveMode), "websocket") {
		mode = runtimecfg.ModeWebSocket
	}
	userServiceURL := appconfig.NormalizeBaseURL(strings.TrimSpace(legacy.UserServiceURL))
	moltMessageURL := appconfig.NormalizeBaseURL(strings.TrimSpace(legacy.MoltMessageURL))
	if userServiceURL != "" && moltMessageURL != "" && userServiceURL != moltMessageURL {
		return nil, fmt.Errorf(
			"legacy settings use different user_service_url (%s) and molt_message_url (%s); automatic migration to one service_base_url is not supported",
			userServiceURL,
			moltMessageURL,
		)
	}
	serviceBaseURL := userServiceURL
	if serviceBaseURL == "" {
		serviceBaseURL = moltMessageURL
	}
	return &normalizedLegacySettings{
		ServiceBaseURL: serviceBaseURL,
		DidDomain:      legacy.DidDomain,
		RuntimeMode:    mode,
	}, nil
}

func refreshResolvedConfig(current *appconfig.Resolved) (*appconfig.Resolved, error) {
	if current == nil {
		return nil, fmt.Errorf("resolved config is required")
	}
	refreshed := *current
	fileConfig, exists, err := appconfig.ReadFileConfig(current.Paths.ConfigFile)
	if err != nil {
		return nil, err
	}
	refreshed.ConfigExists = exists
	refreshed.ConfigSchemaVersion = fileConfig.SchemaVersion
	if mode := strings.TrimSpace(fileConfig.Runtime.Mode); mode != "" {
		refreshed.RuntimeMode = mode
	}
	if socketPath := strings.TrimSpace(fileConfig.Runtime.SocketPath); socketPath != "" {
		refreshed.RuntimeSocketPath = socketPath
	}
	if format := strings.TrimSpace(fileConfig.Output.Format); format != "" {
		refreshed.OutputFormat = format
	}
	if fileConfig.Output.NoColor != nil {
		refreshed.NoColor = *fileConfig.Output.NoColor
	}
	if baseURL := strings.TrimSpace(fileConfig.Services.ServiceBaseURL); baseURL != "" {
		refreshed.ServiceBaseURL = appconfig.NormalizeBaseURL(baseURL)
	}
	if didDomain := strings.TrimSpace(fileConfig.Services.DIDDomain); didDomain != "" {
		refreshed.DIDDomain = didDomain
	}
	if endpoint := strings.TrimSpace(fileConfig.Services.ANPServiceEndpoint); endpoint != "" {
		refreshed.ANPServiceEndpoint = endpoint
	} else if strings.TrimSpace(refreshed.ANPServiceEndpoint) == "" {
		refreshed.ANPServiceEndpoint = appconfig.DeriveANPServiceEndpoint(refreshed.ServiceBaseURL)
	}
	if serviceDID := strings.TrimSpace(fileConfig.Services.ANPServiceDID); serviceDID != "" {
		refreshed.ANPServiceDID = serviceDID
	} else if strings.TrimSpace(refreshed.ANPServiceDID) == "" {
		refreshed.ANPServiceDID = appconfig.DeriveANPServiceDID(refreshed.ServiceBaseURL)
	}
	// Keep mail_service_url in sync with config, defaulting to service_base_url when unset.
	if mailURL := strings.TrimSpace(fileConfig.Services.MailServiceURL); mailURL != "" {
		refreshed.MailServiceURL = appconfig.NormalizeBaseURL(mailURL)
	} else if strings.TrimSpace(refreshed.MailServiceURL) == "" {
		refreshed.MailServiceURL = refreshed.ServiceBaseURL
	}
	if caBundle := strings.TrimSpace(fileConfig.Services.CABundle); caBundle != "" {
		refreshed.CABundle = caBundle
	}
	return &refreshed, nil
}

func replaceImportedLegacyK1DIDs(ctx context.Context, resolved *appconfig.Resolved, imported []identity.IdentitySummary) []string {
	return replaceK1DIDsForSummaries(ctx, resolved, imported)
}

func replaceExistingWorkspaceK1DIDs(ctx context.Context, resolved *appconfig.Resolved) []string {
	if resolved == nil {
		return nil
	}
	manager := identity.NewManager(resolved.Paths)
	identities, err := manager.List()
	if err != nil {
		return []string{fmt.Sprintf("Automatic existing k1 to e1 DID replacement was skipped: %v", err)}
	}
	return replaceK1DIDsForSummaries(ctx, resolved, identities)
}

func replaceK1DIDsForSummaries(ctx context.Context, resolved *appconfig.Resolved, identities []identity.IdentitySummary) []string {
	if resolved == nil || len(identities) == 0 {
		return nil
	}
	service, err := identity.NewService(resolved)
	if err != nil {
		return []string{fmt.Sprintf("Automatic k1 to e1 DID replacement was skipped: %v", err)}
	}

	manager := identity.NewManager(resolved.Paths)
	warnings := make([]string, 0)
	for _, summary := range identities {
		if !identity.IsK1DID(summary.DID) {
			continue
		}

		record, err := manager.Load(summary.IdentityName)
		if err != nil {
			warnings = append(
				warnings,
				fmt.Sprintf(
					"Automatic DID replacement skipped for identity %s (%s): %v",
					summary.IdentityName,
					summary.DID,
					err,
				),
			)
			continue
		}
		if !identity.IsK1DID(record.DID) {
			continue
		}

		if _, _, err := identity.HandlePathPrefixFromDID(record.DID); err != nil {
			warnings = append(
				warnings,
				fmt.Sprintf(
					"Automatic DID replacement skipped for identity %s (%s): %v",
					summary.IdentityName,
					record.DID,
					err,
				),
			)
			continue
		}
		result, err := service.ReplaceDID(ctx, identity.ReplaceDIDParams{IdentityName: summary.IdentityName})
		if err != nil {
			warnings = append(
				warnings,
				fmt.Sprintf(
					"Automatic DID replacement failed for identity %s (%s): %v",
					summary.IdentityName,
					record.DID,
					err,
				),
			)
			continue
		}
		oldDID, _ := result.Data["old_did"].(string)
		newDID, _ := result.Data["did"].(string)
		if _, _, err := store.RebindLocalIdentityState(ctx, resolved.Paths, oldDID, newDID); err != nil {
			warnings = append(
				warnings,
				fmt.Sprintf(
					"Automatic DID replacement completed but local SQLite rebinding failed for identity %s: %v",
					summary.IdentityName,
					err,
				),
			)
		}
		for _, warning := range result.Warnings {
			warnings = append(
				warnings,
				fmt.Sprintf(
					"Automatic DID replacement completed with warning for identity %s: %s",
					summary.IdentityName,
					warning,
				),
			)
		}
	}
	return warnings
}

func ensureTargetStoreSchema(ctx context.Context, paths appconfig.Paths) error {
	db, err := store.Open(paths)
	if err != nil {
		return err
	}
	defer db.Close()
	return store.EnsureSchema(ctx, db)
}

func validateSQLiteHealth(ctx context.Context, db *sql.DB) error {
	if err := expectSingleSQLiteOK(ctx, db, "PRAGMA integrity_check"); err != nil {
		return err
	}
	if err := expectSQLiteNoRows(ctx, db, "PRAGMA foreign_key_check"); err != nil {
		return err
	}
	return nil
}

func expectSingleSQLiteOK(ctx context.Context, db *sql.DB, query string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("execute %s: %w", query, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return fmt.Errorf("%s returned no rows", query)
	}
	var result string
	if err := rows.Scan(&result); err != nil {
		return fmt.Errorf("scan %s result: %w", query, err)
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") && strings.TrimSpace(result) != "" {
		return fmt.Errorf("%s failed: %s", query, result)
	}
	return nil
}

func expectSQLiteNoRows(ctx context.Context, db *sql.DB, query string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("execute %s: %w", query, err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("%s returned foreign key violations", query)
	}
	return nil
}

func cleanupLegacySkillArtifacts(ctx context.Context) []string {
	homeDir, err := legacyCleanupUserHome()
	if err != nil {
		return []string{fmt.Sprintf("Legacy awiki skill cleanup skipped: resolve home directory failed: %v", err)}
	}

	warnings := make([]string, 0)
	warnings = append(warnings, uninstallLegacyListenerService(ctx, homeDir)...)

	for _, path := range legacySkillInstallDirs(homeDir) {
		if !pathExists(path) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			warnings = append(
				warnings,
				fmt.Sprintf("Failed to remove legacy awiki skill path %s: %v", path, err),
			)
		}
	}

	if err := removeLegacyHeartbeatSection(homeDir); err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to remove legacy awiki heartbeat section: %v", err),
		)
	}
	return warnings
}

func legacySkillInstallDirs(homeDir string) []string {
	return []string{
		filepath.Join(homeDir, ".openclaw", "skills", legacySkillInstallDirName),
		filepath.Join(homeDir, ".openclaw", "workspace", "skills", legacySkillInstallDirName),
	}
}

func uninstallLegacyListenerService(ctx context.Context, homeDir string) []string {
	switch legacyCleanupGOOS {
	case "darwin":
		return uninstallLegacyListenerServiceDarwin(ctx, homeDir)
	case "linux":
		return uninstallLegacyListenerServiceLinux(ctx, homeDir)
	case "windows":
		return uninstallLegacyListenerServiceWindows(ctx, homeDir)
	default:
		return nil
	}
}

func uninstallLegacyListenerServiceDarwin(ctx context.Context, homeDir string) []string {
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", legacyMacOSListenerLabel+".plist")
	if !fileExists(plistPath) {
		return nil
	}

	warnings := make([]string, 0)
	if err := legacyCleanupRunCommand(ctx, "launchctl", "unload", plistPath); err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to stop legacy awiki listener LaunchAgent: %v", err),
		)
	}
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to remove legacy awiki listener LaunchAgent plist %s: %v", plistPath, err),
		)
	}
	return warnings
}

func uninstallLegacyListenerServiceLinux(ctx context.Context, homeDir string) []string {
	unitPath := filepath.Join(legacyXDGConfigHome(homeDir), "systemd", "user", legacyLinuxListenerUnit)
	if !fileExists(unitPath) {
		return nil
	}

	warnings := make([]string, 0)
	if err := legacyCleanupRunCommand(ctx, "systemctl", "--user", "disable", "--now", legacyLinuxListenerUnit); err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to stop legacy awiki listener systemd user service: %v", err),
		)
	}
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to remove legacy awiki listener systemd unit %s: %v", unitPath, err),
		)
	}
	if err := legacyCleanupRunCommand(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to reload systemd user units after legacy awiki listener cleanup: %v", err),
		)
	}
	return warnings
}

func uninstallLegacyListenerServiceWindows(ctx context.Context, homeDir string) []string {
	appDir := filepath.Join(legacyLocalAppData(homeDir), legacyWindowsListenerTaskName)
	if !pathExists(appDir) {
		return nil
	}

	warnings := make([]string, 0)
	if err := legacyCleanupRunCommand(ctx, "schtasks", "/End", "/TN", legacyWindowsListenerTaskName); err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to stop legacy awiki listener scheduled task: %v", err),
		)
	}
	if err := legacyCleanupRunCommand(ctx, "schtasks", "/Delete", "/TN", legacyWindowsListenerTaskName, "/F"); err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to remove legacy awiki listener scheduled task: %v", err),
		)
	}
	if err := os.RemoveAll(appDir); err != nil {
		warnings = append(
			warnings,
			fmt.Sprintf("Failed to remove legacy awiki listener app directory %s: %v", appDir, err),
		)
	}
	return warnings
}

func removeLegacyHeartbeatSection(homeDir string) error {
	workspaceDir := strings.TrimSpace(os.Getenv("OPENCLAW_WORKSPACE"))
	if workspaceDir == "" {
		workspaceDir = filepath.Join(homeDir, ".openclaw", "workspace")
	}
	heartbeatPath := filepath.Join(workspaceDir, "HEARTBEAT.md")
	if !fileExists(heartbeatPath) {
		return nil
	}

	contentBytes, err := os.ReadFile(heartbeatPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", heartbeatPath, err)
	}
	content := string(contentBytes)
	startIndex := strings.Index(content, legacyHeartbeatSectionStart)
	endIndex := strings.Index(content, legacyHeartbeatSectionEnd)
	if startIndex < 0 || endIndex < startIndex {
		return nil
	}
	endIndex += len(legacyHeartbeatSectionEnd)
	section := content[startIndex:endIndex]
	if !strings.Contains(section, legacySkillInstallDirName) {
		return nil
	}

	updated := content[:startIndex] + content[endIndex:]
	updated = collapseExtraBlankLines(strings.TrimLeft(updated, "\n"))

	info, statErr := os.Stat(heartbeatPath)
	if statErr != nil {
		return fmt.Errorf("stat %s: %w", heartbeatPath, statErr)
	}
	return os.WriteFile(heartbeatPath, []byte(updated), info.Mode())
}

func collapseExtraBlankLines(content string) string {
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}
	return content
}

func legacyXDGConfigHome(homeDir string) string {
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return value
	}
	return filepath.Join(homeDir, ".config")
}

func legacyLocalAppData(homeDir string) string {
	if value := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); value != "" {
		return value
	}
	return filepath.Join(homeDir, "AppData", "Local")
}

func runLegacyCleanupCommand(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}
