package skillbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func LoadSkillState(path string) (*SkillState, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skill state: %w", err)
	}
	var state SkillState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("parse skill state: %w", err)
	}
	return &state, nil
}

func SaveSkillState(path string, state SkillState) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("skill state path is required")
	}
	state.SchemaVersion = skillStateSchemaVersion
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skill state: %w", err)
	}
	return writeAtomicFile(path, raw, 0o600)
}

func RecordSyncState(path string, bundle *Bundle, dir string, rootSHA string, status string, lastError string) (*SkillState, error) {
	state, err := LoadSkillState(path)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &SkillState{}
	}
	state.CurrentCLIVersion = bundle.Manifest.CLIVersion
	state.CurrentBundleVersion = bundle.Manifest.BundleVersion
	state.CurrentBundleSHA256 = bundle.Manifest.BundleSHA256
	state.RootSkillSync = RootSkillSyncState{
		Dir:             dir,
		RootSkillSHA256: rootSHA,
		LastSyncedAt:    nowUTC().Format(time.RFC3339),
		LastSyncStatus:  status,
		LastError:       strings.TrimSpace(lastError),
	}
	state.CachePolicy = CachePolicy{KeepVersions: []string{bundle.Manifest.CLIVersion}, TTLDays: 30}
	if err := SaveSkillState(path, *state); err != nil {
		return nil, err
	}
	return state, nil
}

func writeAtomicFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".skill-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	cleanup = false
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open dir: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync dir: %w", err)
	}
	return nil
}
