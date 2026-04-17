package skillbundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func SyncRootSkill(resolved *appconfig.Resolved, dir string, force bool, adopt bool) (*SyncResult, error) {
	bundle, err := Load()
	if err != nil {
		return nil, err
	}
	rootContent, rootSHA, err := RenderRootSkill(bundle)
	if err != nil {
		return nil, err
	}
	defaultDir := strings.TrimSpace(dir)
	if defaultDir == "" {
		defaultDir, err = DefaultSkillRootDir()
		if err != nil {
			return nil, err
		}
	}
	skillDir := filepath.Join(defaultDir, bundle.Manifest.RootSkill.Name)
	skillPath := filepath.Join(skillDir, "SKILL.md")
	managedPath := filepath.Join(skillDir, ".awiki-managed.json")
	result := &SyncResult{
		Dir:             defaultDir,
		SkillDir:        skillDir,
		SkillPath:       skillPath,
		ManagedPath:     managedPath,
		RootSkillSHA256: rootSHA,
	}
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		return nil, fmt.Errorf("create skill dir: %w", err)
	}
	managed, managedExists, err := loadManagedFile(managedPath)
	if err != nil {
		return nil, err
	}
	currentExists := fileExists(skillPath)
	currentHash := ""
	unmodifiedManaged := false
	if currentExists {
		raw, err := os.ReadFile(skillPath)
		if err != nil {
			return nil, fmt.Errorf("read current skill file: %w", err)
		}
		currentHash = checksum(raw)
		if managedExists && managed.RootSkillSHA256 != "" {
			unmodifiedManaged = currentHash == managed.RootSkillSHA256
		}
	}
	if currentExists && !managedExists && !adopt && !force {
		result.Conflict = true
		result.ConflictReason = "target exists but is unmanaged"
		return result, nil
	}
	if managedExists && currentExists && !unmodifiedManaged && !force {
		result.Conflict = true
		result.ConflictReason = "managed root skill was modified locally"
		return result, nil
	}
	if currentExists && (force || adopt || (managedExists && !unmodifiedManaged)) {
		backupPath, err := backupSkillFile(skillPath)
		if err != nil {
			return nil, err
		}
		result.BackupPath = backupPath
	}
	if err := os.WriteFile(skillPath, []byte(rootContent), 0o600); err != nil {
		return nil, fmt.Errorf("write root skill: %w", err)
	}
	managedPayload := ManagedFile{
		SchemaVersion:   1,
		Type:            "root-skill",
		SkillName:       bundle.Manifest.RootSkill.Name,
		CLIVersion:      bundle.Manifest.CLIVersion,
		BundleVersion:   bundle.Manifest.BundleVersion,
		RootSkillSHA256: rootSHA,
		InstalledAt:     time.Now().UTC().Format(time.RFC3339),
		ManagedPaths:    []string{"SKILL.md"},
		Adopted:         adopt,
	}
	managedRaw, err := json.MarshalIndent(managedPayload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal managed skill metadata: %w", err)
	}
	if err := os.WriteFile(managedPath, managedRaw, 0o600); err != nil {
		return nil, fmt.Errorf("write managed skill metadata: %w", err)
	}
	paths, err := ResolvePaths(resolved)
	if err == nil {
		_, _ = RecordSyncState(paths.SkillStatePath, bundle, defaultDir, rootSHA, "ok", "")
	}
	result.WroteFiles = true
	result.Adopted = adopt
	return result, nil
}

func loadManagedFile(path string) (*ManagedFile, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read managed skill metadata: %w", err)
	}
	var managed ManagedFile
	if err := json.Unmarshal(raw, &managed); err != nil {
		return nil, false, fmt.Errorf("parse managed skill metadata: %w", err)
	}
	return &managed, true, nil
}

func backupSkillFile(path string) (string, error) {
	if !fileExists(path) {
		return "", nil
	}
	backupDir := filepath.Join(filepath.Dir(path), ".backup")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", fmt.Errorf("create skill backup dir: %w", err)
	}
	backupPath := filepath.Join(backupDir, filepath.Base(path)+"."+time.Now().UTC().Format("20060102T150405Z")+".bak")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read skill file for backup: %w", err)
	}
	if err := os.WriteFile(backupPath, raw, 0o600); err != nil {
		return "", fmt.Errorf("write skill backup: %w", err)
	}
	return backupPath, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
