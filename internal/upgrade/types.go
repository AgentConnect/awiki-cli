package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const (
	LatestWorkspaceSchemaVersion = 3
	backupDirName                = "backups"
	timeLayout                   = "20060102T150405Z"
)

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

type Paths struct {
	ConfigFile           string `json:"config_file"`
	LegacyConfigFile     string `json:"legacy_config_file"`
	IdentityDir          string `json:"identity_dir"`
	DatabaseFile         string `json:"database_file"`
	LegacyCredentialsDir string `json:"legacy_credentials_dir"`
	LegacyDataDir        string `json:"legacy_data_dir"`
	LegacySettingsPath   string `json:"legacy_settings_path"`
	MetaPath             string `json:"meta_path"`
	JournalPath          string `json:"journal_path"`
	LockPath             string `json:"lock_path"`
	BackupRoot           string `json:"backup_root"`
}

type Context struct {
	Resolved    *appconfig.Resolved
	Paths       Paths
	AppVersion  string
	Inspection  *Inspection
	BackupDir   string
	CurrentMeta *Meta
	Warnings    []string
}

type Inspection struct {
	Paths     Paths     `json:"paths"`
	Meta      *Meta     `json:"meta,omitempty"`
	Journal   *Journal  `json:"journal,omitempty"`
	Detection Detection `json:"detection"`
}

type Detection struct {
	CurrentVersion             int    `json:"current_version"`
	LatestVersion              int    `json:"latest_version"`
	CurrentVersionSource       string `json:"current_version_source"`
	Empty                      bool   `json:"empty"`
	HasWorkspace               bool   `json:"has_workspace"`
	HasLegacy                  bool   `json:"has_legacy"`
	ConfigExists               bool   `json:"config_exists"`
	LegacyConfigExists         bool   `json:"legacy_config_exists"`
	ConfigSchemaVersion        int    `json:"config_schema_version"`
	ConfigError                string `json:"config_error,omitempty"`
	IdentityIndexExists        bool   `json:"identity_index_exists"`
	IdentityIndexSchemaVersion int    `json:"identity_index_schema_version"`
	IdentityIndexError         string `json:"identity_index_error,omitempty"`
	DatabaseExists             bool   `json:"database_exists"`
	DatabaseSchemaVersion      int    `json:"database_schema_version"`
	DatabaseError              string `json:"database_error,omitempty"`
	LegacyIdentityExists       bool   `json:"legacy_identity_exists"`
	LegacyIdentityError        string `json:"legacy_identity_error,omitempty"`
	LegacyDatabaseExists       bool   `json:"legacy_database_exists"`
	LegacyDatabaseError        string `json:"legacy_database_error,omitempty"`
	LegacySettingsExists       bool   `json:"legacy_settings_exists"`
}

type Meta struct {
	WorkspaceSchemaVersion int      `json:"workspace_schema_version"`
	AppVersion             string   `json:"app_version,omitempty"`
	UpdatedAt              string   `json:"updated_at"`
	LastUpgradeID          string   `json:"last_upgrade_id,omitempty"`
	LastBackupDir          string   `json:"last_backup_dir,omitempty"`
	Warnings               []string `json:"warnings,omitempty"`
}

type Journal struct {
	UpgradeID   string `json:"upgrade_id"`
	FromVersion int    `json:"from_version"`
	ToVersion   int    `json:"to_version"`
	CurrentStep string `json:"current_step"`
	Phase       string `json:"phase"`
	BackupDir   string `json:"backup_dir,omitempty"`
	StartedAt   string `json:"started_at"`
	AppVersion  string `json:"app_version,omitempty"`
}

type lockMetadata struct {
	LockScheme string `json:"lock_scheme,omitempty"`
	PID        int    `json:"pid"`
	AppVersion string `json:"app_version,omitempty"`
	StartedAt  string `json:"started_at"`
	Hostname   string `json:"hostname,omitempty"`
	Executable string `json:"executable,omitempty"`
}

type Migration interface {
	From() int
	To() int
	Name() string
	IsDone(ctx context.Context, uc *Context) (bool, error)
	Apply(ctx context.Context, uc *Context) error
	Validate(ctx context.Context, uc *Context) error
}

type Upgrader struct {
	latestVersion int
	migrations    map[int]Migration
}

func ResolvePaths(resolved *appconfig.Resolved) Paths {
	if resolved == nil {
		return Paths{}
	}
	workspaceHomeDir := resolved.Paths.WorkspaceHomeDir
	if workspaceHomeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			workspaceHomeDir = filepath.Join(home, "."+"awiki-cli")
		}
	}
	return Paths{
		ConfigFile:           resolved.Paths.ConfigFile,
		LegacyConfigFile:     appconfig.LegacyConfigPath(resolved.Paths),
		IdentityDir:          resolved.Paths.IdentityDir,
		DatabaseFile:         resolved.Paths.DatabaseFile,
		LegacyCredentialsDir: resolved.Paths.LegacyCredentialsDir,
		LegacyDataDir:        resolved.Paths.LegacyDataDir,
		LegacySettingsPath:   filepath.Join(resolved.Paths.LegacyDataDir, "config", "settings.json"),
		MetaPath:             filepath.Join(workspaceHomeDir, "upgrade", "meta.json"),
		JournalPath:          filepath.Join(workspaceHomeDir, "upgrade", "upgrade_journal.json"),
		LockPath:             filepath.Join(workspaceHomeDir, "upgrade", "upgrade.lock"),
		BackupRoot:           filepath.Join(workspaceHomeDir, "upgrade", backupDirName),
	}
}

func NewContext(resolved *appconfig.Resolved, appVersion string) *Context {
	return &Context{
		Resolved:   resolved,
		Paths:      ResolvePaths(resolved),
		AppVersion: appVersion,
	}
}

func (u *Upgrader) Plan(fromVersion, toVersion int) ([]Migration, error) {
	if fromVersion > toVersion {
		return nil, fmt.Errorf("workspace schema version %d is newer than target %d", fromVersion, toVersion)
	}
	plan := make([]Migration, 0, toVersion-fromVersion)
	for version := fromVersion; version < toVersion; version++ {
		migration, ok := u.migrations[version]
		if !ok {
			return nil, fmt.Errorf("missing workspace migration %d -> %d", version, version+1)
		}
		if migration.To() != version+1 {
			return nil, fmt.Errorf("workspace migration %d has unexpected target %d", version, migration.To())
		}
		plan = append(plan, migration)
	}
	return plan, nil
}
