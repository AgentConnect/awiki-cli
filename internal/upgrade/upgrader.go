package upgrade

import (
	"context"
	"fmt"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func NewDefaultUpgrader() *Upgrader {
	migration := newWorkspaceV0ToV1Migration()
	return &Upgrader{
		latestVersion: LatestWorkspaceSchemaVersion,
		migrations: map[int]Migration{
			migration.From(): migration,
		},
	}
}

func UpgradeIfNeeded(ctx context.Context, resolved *appconfig.Resolved, appVersion string) error {
	return NewDefaultUpgrader().UpgradeIfNeeded(ctx, NewContext(resolved, appVersion))
}

func (u *Upgrader) UpgradeIfNeeded(ctx context.Context, uc *Context) error {
	if uc == nil {
		return fmt.Errorf("upgrade context is required")
	}
	inspection, err := Inspect(ctx, uc.Resolved, uc.AppVersion)
	if err != nil {
		return err
	}
	uc.Inspection = inspection
	uc.CurrentMeta = inspection.Meta
	currentVersion := inspection.Detection.CurrentVersion
	if currentVersion > u.latestVersion {
		return fmt.Errorf("workspace schema version %d is newer than supported %d", currentVersion, u.latestVersion)
	}
	if inspection.Detection.Empty || currentVersion == u.latestVersion {
		if inspection.Journal != nil {
			return ClearJournal(uc.Paths.JournalPath)
		}
		return nil
	}

	unlock, err := AcquireFileLock(uc.Paths.LockPath, uc.AppVersion)
	if err != nil {
		return err
	}
	defer func() {
		_ = unlock()
	}()

	inspection, err = Inspect(ctx, uc.Resolved, uc.AppVersion)
	if err != nil {
		return err
	}
	uc.Inspection = inspection
	uc.CurrentMeta = inspection.Meta
	currentVersion = inspection.Detection.CurrentVersion
	if inspection.Detection.Empty || currentVersion == u.latestVersion {
		if inspection.Journal != nil {
			return ClearJournal(uc.Paths.JournalPath)
		}
		return nil
	}

	journal, err := LoadJournal(uc.Paths.JournalPath)
	if err != nil {
		return err
	}
	backupDir := ""
	if journal != nil {
		backupDir = journal.BackupDir
	}
	if backupDir == "" {
		backupID := nowUTC().Format(timeLayout)
		backupDir, err = CreateBackup(ctx, uc, backupID)
		if err != nil {
			return err
		}
	}
	uc.BackupDir = backupDir

	plan, err := u.Plan(currentVersion, u.latestVersion)
	if err != nil {
		return err
	}
	if len(plan) == 0 {
		return ClearJournal(uc.Paths.JournalPath)
	}

	upgradeID := nowUTC().Format(timeLayout)
	if journal != nil && journal.UpgradeID != "" {
		upgradeID = journal.UpgradeID
	}
	for _, migration := range plan {
		journal = &Journal{
			UpgradeID:   upgradeID,
			FromVersion: migration.From(),
			ToVersion:   migration.To(),
			CurrentStep: migration.Name(),
			Phase:       "checking",
			BackupDir:   backupDir,
			StartedAt:   nowUTC().Format(time.RFC3339),
			AppVersion:  uc.AppVersion,
		}
		if err := SaveJournal(uc.Paths.JournalPath, *journal); err != nil {
			return err
		}
		done, err := migration.IsDone(ctx, uc)
		if err != nil {
			return err
		}
		if !done {
			journal.Phase = "applying"
			if err := SaveJournal(uc.Paths.JournalPath, *journal); err != nil {
				return err
			}
			if err := migration.Apply(ctx, uc); err != nil {
				return err
			}
		}
		journal.Phase = "validating"
		if err := SaveJournal(uc.Paths.JournalPath, *journal); err != nil {
			return err
		}
		if err := migration.Validate(ctx, uc); err != nil {
			return err
		}
		meta := Meta{
			WorkspaceSchemaVersion: migration.To(),
			AppVersion:             uc.AppVersion,
			UpdatedAt:              nowUTC().Format(time.RFC3339),
			LastUpgradeID:          upgradeID,
			LastBackupDir:          backupDir,
		}
		if err := SaveMeta(uc.Paths.MetaPath, meta); err != nil {
			return err
		}
		uc.CurrentMeta = &meta
	}

	if err := ClearJournal(uc.Paths.JournalPath); err != nil {
		return err
	}
	return nil
}
