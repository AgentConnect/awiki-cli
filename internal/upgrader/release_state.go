package upgrader

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/update"
)

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func LoadReleaseState(path string) (*ReleaseState, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read release state: %w", err)
	}
	var state ReleaseState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("parse release state: %w", err)
	}
	return &state, nil
}

func SaveReleaseState(path string, state ReleaseState) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("release state path is required")
	}
	state.SchemaVersion = releaseStateSchemaVersion
	state.UpdatedAt = nowUTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal release state: %w", err)
	}
	return writeAtomicFile(path, raw, 0o600)
}

func RecordCheck(path string, decision update.Decision) (*ReleaseState, error) {
	state, err := LoadReleaseState(path)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &ReleaseState{SchemaVersion: releaseStateSchemaVersion}
	}
	state.Channel = decision.Channel
	state.LastCheckedAt = nowUTC().Format(time.RFC3339)
	state.LastCheckSource = normalizeCheckSource(decision.Source)
	if shouldTrackPending(decision) {
		state.PendingUpdate = &PendingUpdate{
			Version:        decision.LatestVersion,
			ArtifactURL:    decision.ArtifactURL,
			ArtifactSHA256: decision.ArtifactSHA256,
			RecordedAt:     state.LastCheckedAt,
		}
	} else {
		state.PendingUpdate = nil
	}
	if err := SaveReleaseState(path, *state); err != nil {
		return nil, err
	}
	return state, nil
}

func RecordWSPush(path string, decision update.Decision) (*ReleaseState, error) {
	return recordWSPush(path, decision, true)
}

func RecordWSPushObservation(path string, decision update.Decision) (*ReleaseState, error) {
	return recordWSPush(path, decision, false)
}

func recordWSPush(path string, decision update.Decision, trackPending bool) (*ReleaseState, error) {
	state, err := LoadReleaseState(path)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &ReleaseState{SchemaVersion: releaseStateSchemaVersion}
	}
	now := nowUTC().Format(time.RFC3339)
	state.Channel = decision.Channel
	state.LastWSUpgradeEventAt = now
	state.LastCheckSource = normalizeCheckSource("ws_push")
	if trackPending && shouldTrackPending(decision) {
		state.PendingUpdate = &PendingUpdate{
			Version:        decision.LatestVersion,
			ArtifactURL:    decision.ArtifactURL,
			ArtifactSHA256: decision.ArtifactSHA256,
			RecordedAt:     now,
		}
	}
	if err := SaveReleaseState(path, *state); err != nil {
		return nil, err
	}
	return state, nil
}

func RecordPredownload(path string, decision update.Decision) (*ReleaseState, error) {
	state, err := LoadReleaseState(path)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &ReleaseState{SchemaVersion: releaseStateSchemaVersion}
	}
	now := nowUTC().Format(time.RFC3339)
	state.Channel = decision.Channel
	state.LastWSUpgradeEventAt = now
	state.LastCheckSource = normalizeCheckSource("ws_push")
	state.PendingUpdate = &PendingUpdate{
		Version:        decision.LatestVersion,
		ArtifactURL:    decision.ArtifactURL,
		ArtifactSHA256: decision.ArtifactSHA256,
		RecordedAt:     now,
	}
	if err := SaveReleaseState(path, *state); err != nil {
		return nil, err
	}
	return state, nil
}

func RecordFailure(path string, version string, stage string, message string) (*ReleaseState, error) {
	state, err := LoadReleaseState(path)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &ReleaseState{SchemaVersion: releaseStateSchemaVersion}
	}
	state.LastFailedUpdate = &FailedUpdate{
		Version:  strings.TrimSpace(version),
		Stage:    strings.TrimSpace(stage),
		Message:  strings.TrimSpace(message),
		FailedAt: nowUTC().Format(time.RFC3339),
	}
	if err := SaveReleaseState(path, *state); err != nil {
		return nil, err
	}
	return state, nil
}

func RecordSuccess(path string, decision update.Decision) (*ReleaseState, error) {
	state, err := LoadReleaseState(path)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &ReleaseState{SchemaVersion: releaseStateSchemaVersion}
	}
	state.Channel = decision.Channel
	state.CurrentVersion = strings.TrimSpace(decision.LatestVersion)
	state.CurrentArtifactSHA256 = strings.TrimSpace(decision.ArtifactSHA256)
	state.LastAppliedVersion = strings.TrimSpace(decision.LatestVersion)
	state.PendingUpdate = nil
	state.LastFailedUpdate = nil
	if state.LastCheckedAt == "" {
		state.LastCheckedAt = nowUTC().Format(time.RFC3339)
	}
	if state.LastCheckSource == "" {
		state.LastCheckSource = normalizeCheckSource(decision.Source)
	}
	if err := SaveReleaseState(path, *state); err != nil {
		return nil, err
	}
	return state, nil
}

func normalizeCheckSource(source string) string {
	switch strings.TrimSpace(source) {
	case "network":
		return "check_api"
	case "cache", "cache_stale", "ws_push":
		return strings.TrimSpace(source)
	default:
		return strings.TrimSpace(source)
	}
}

func shouldTrackPending(decision update.Decision) bool {
	if !decision.ArtifactAvailable {
		return false
	}
	if strings.TrimSpace(decision.LatestVersion) == "" {
		return false
	}
	return decision.HasNewerVersion || decision.Blocked
}

func writeAtomicFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".release-state-*.tmp")
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
