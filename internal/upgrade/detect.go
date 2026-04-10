package upgrade

import (
	"context"
	"path/filepath"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func Inspect(ctx context.Context, resolved *appconfig.Resolved, appVersion string) (*Inspection, error) {
	uc := NewContext(resolved, appVersion)
	meta, err := LoadMeta(uc.Paths.MetaPath)
	if err != nil {
		return nil, err
	}
	journal, err := LoadJournal(uc.Paths.JournalPath)
	if err != nil {
		return nil, err
	}
	detection := Detect(ctx, resolved, meta)
	inspection := &Inspection{
		Paths:     uc.Paths,
		Meta:      meta,
		Journal:   journal,
		Detection: detection,
	}
	return inspection, nil
}

func Detect(ctx context.Context, resolved *appconfig.Resolved, meta *Meta) Detection {
	detection := Detection{LatestVersion: LatestWorkspaceSchemaVersion}
	if resolved == nil {
		detection.CurrentVersion = LatestWorkspaceSchemaVersion
		detection.CurrentVersionSource = "default_empty"
		detection.Empty = true
		return detection
	}

	paths := ResolvePaths(resolved)
	detection.ConfigExists = fileExists(paths.ConfigFile)
	if detection.ConfigExists {
		fileConfig, _, err := appconfig.ReadFileConfig(paths.ConfigFile)
		if err != nil {
			detection.ConfigError = err.Error()
		} else {
			detection.ConfigSchemaVersion = fileConfig.SchemaVersion
		}
	}

	identityIndexPath := filepath.Join(paths.IdentityDir, identity.IndexFileName)
	detection.IdentityIndexExists = fileExists(identityIndexPath)
	if detection.IdentityIndexExists {
		manager := identity.NewManager(resolved.Paths)
		index, err := manager.LoadIndex()
		if err != nil {
			detection.IdentityIndexError = err.Error()
		} else {
			detection.IdentityIndexSchemaVersion = index.SchemaVersion
		}
	}

	detection.DatabaseExists = fileExists(paths.DatabaseFile)
	if detection.DatabaseExists {
		db, err := store.OpenReadOnly(paths.DatabaseFile)
		if err != nil {
			detection.DatabaseError = err.Error()
		} else {
			defer db.Close()
			version, versionErr := store.CurrentSchemaVersion(db)
			if versionErr != nil {
				detection.DatabaseError = versionErr.Error()
			} else {
				detection.DatabaseSchemaVersion = version
			}
		}
	}

	manager := identity.NewManager(resolved.Paths)
	legacyScan, legacyScanErr := manager.ScanLegacy()
	if legacyScanErr != nil {
		detection.LegacyIdentityError = legacyScanErr.Error()
	} else if legacyScan != nil {
		detection.LegacyIdentityExists = legacyScan.HasLegacy
	}

	legacyDB, legacyDBErr := store.ScanLegacyDatabase(ctx, resolved.Paths)
	if legacyDBErr != nil {
		detection.LegacyDatabaseError = legacyDBErr.Error()
	} else if legacyDB != nil {
		detection.LegacyDatabaseExists = legacyDB.Exists
	}
	if fileExists(paths.LegacySettingsPath) {
		detection.LegacySettingsExists = true
	}

	detection.HasWorkspace = detection.ConfigExists || detection.IdentityIndexExists || detection.DatabaseExists
	detection.HasLegacy = detection.LegacyIdentityExists || detection.LegacyDatabaseExists || detection.LegacySettingsExists
	detection.Empty = meta == nil && !detection.HasWorkspace && !detection.HasLegacy

	switch {
	case meta != nil:
		detection.CurrentVersion = meta.WorkspaceSchemaVersion
		detection.CurrentVersionSource = "meta"
	case detection.Empty:
		detection.CurrentVersion = LatestWorkspaceSchemaVersion
		detection.CurrentVersionSource = "default_empty"
	default:
		detection.CurrentVersion = 0
		detection.CurrentVersionSource = "legacy_detector"
	}
	return detection
}
