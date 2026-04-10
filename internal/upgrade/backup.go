package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func CreateBackup(ctx context.Context, uc *Context, backupID string) (string, error) {
	if uc == nil {
		return "", fmt.Errorf("upgrade context is required")
	}
	if strings.TrimSpace(backupID) == "" {
		backupID = nowUTC().Format(timeLayout)
	}
	backupDir := filepath.Join(uc.Paths.BackupRoot, backupID)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	if fileExists(uc.Paths.ConfigFile) {
		if err := copyFile(uc.Paths.ConfigFile, filepath.Join(backupDir, "config.json.bak"), 0o600); err != nil {
			return "", err
		}
	}
	if dirExists(uc.Paths.IdentityDir) {
		if err := copyTree(uc.Paths.IdentityDir, filepath.Join(backupDir, "identities")); err != nil {
			return "", err
		}
	}
	if fileExists(uc.Paths.DatabaseFile) {
		if err := backupSQLiteDatabase(ctx, uc.Paths.DatabaseFile, filepath.Join(backupDir, "awiki-cli.db.bak")); err != nil {
			return "", err
		}
	}
	if fileExists(uc.Paths.MetaPath) {
		if err := copyFile(uc.Paths.MetaPath, filepath.Join(backupDir, "meta.json.bak"), 0o600); err != nil {
			return "", err
		}
	}
	if fileExists(uc.Paths.JournalPath) {
		if err := copyFile(uc.Paths.JournalPath, filepath.Join(backupDir, "upgrade_journal.json.bak"), 0o600); err != nil {
			return "", err
		}
	}
	return backupDir, syncDirectory(filepath.Dir(backupDir))
}

func backupSQLiteDatabase(ctx context.Context, sourcePath string, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
		return fmt.Errorf("create sqlite backup dir: %w", err)
	}
	if err := os.Remove(destinationPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing sqlite backup: %w", err)
	}
	db, err := store.Open(appconfig.Paths{DatabaseFile: sourcePath})
	if err != nil {
		return fmt.Errorf("open sqlite source for backup: %w", err)
	}
	defer db.Close()
	statement := fmt.Sprintf("VACUUM INTO '%s'", escapeSQLiteString(destinationPath))
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("vacuum sqlite into backup: %w", err)
	}
	return nil
}

func escapeSQLiteString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
